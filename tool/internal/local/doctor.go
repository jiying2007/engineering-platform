package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type doctorCounts struct {
	Runtimes      int `json:"runtimes"`
	Projects      int `json:"projects"`
	ProjectChecks int `json:"project_checks"`
	Passed        int `json:"passed"`
	Failed        int `json:"failed"`
	Skipped       int `json:"skipped"`
}

type doctorDisk struct {
	BlockSize      uint64 `json:"block_size"`
	TotalBytes     uint64 `json:"total_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

type doctorReport struct {
	Ready             bool          `json:"ready"`
	InstallationReady bool          `json:"installation_ready"`
	Status            string        `json:"status"`
	DataDir           string        `json:"data_dir"`
	Checks            []doctorCheck `json:"checks"`
	Counts            doctorCounts  `json:"counts"`
	Disk              *doctorDisk   `json:"disk,omitempty"`
	ModelInvocations  int           `json:"model_invocations"`
	Credentials       string        `json:"credentials"`
	RuntimeVersions   string        `json:"runtime_versions"`
}

// doctor inspects local availability. It does not run registered commands,
// probe their versions, inspect credentials, or establish project correctness.
// All local Git inspection shares one deadline, including multiple projects.
func doctor(dataDir string) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return inspectDoctor(ctx, dataDir), nil
}

func inspectDoctor(ctx context.Context, dataDir string) doctorReport {
	report := doctorReport{DataDir: dataDir, Checks: []doctorCheck{}, Credentials: "not inspected", RuntimeVersions: "not probed"}
	add := func(name, detail string, err error) {
		item := doctorCheck{Name: name, Status: "passed", OK: true, Detail: detail}
		if err != nil {
			item.Status, item.OK, item.Detail = "failed", false, err.Error()
			report.Counts.Failed++
		} else {
			report.Counts.Passed++
		}
		report.Checks = append(report.Checks, item)
	}
	skip := func(name, detail string) {
		report.Checks = append(report.Checks, doctorCheck{Name: name, Status: "skipped", Detail: detail})
		report.Counts.Skipped++
	}
	gitPath, gitErr := doctorExecutable("git", "")
	add("git:executable", gitPath, gitErr)
	directoryErr := doctorDirectory(dataDir)
	add("data_dir:permissions", "real directory owned by this user with mode 0700", directoryErr)
	if directoryErr == nil {
		add("data_dir:write_sync", "temporary file written, fsynced and removed", doctorWriteProbe(dataDir))
	} else {
		skip("data_dir:write_sync", "directory permissions need attention before a write probe")
	}
	var disk syscall.Statfs_t
	diskErr := syscall.Statfs(dataDir, &disk)
	if diskErr == nil && disk.Bsize <= 0 {
		diskErr = errors.New("filesystem returned an invalid block size")
	}
	if diskErr == nil {
		blockSize := uint64(disk.Bsize)
		report.Disk = &doctorDisk{BlockSize: blockSize, TotalBytes: disk.Blocks * blockSize, FreeBytes: disk.Bfree * blockSize, AvailableBytes: disk.Bavail * blockSize}
	}
	add("data_dir:disk", "filesystem capacity observed; task space requirements are project-specific", diskErr)
	report.InstallationReady = report.Counts.Failed == 0 && report.Counts.Skipped == 0

	runtimeNames, err := registrations(dataDir, "runtimes")
	add("runtimes:registrations", fmt.Sprintf("%d registered", len(runtimeNames)), err)
	projectNames, err := registrations(dataDir, "projects")
	add("projects:registrations", fmt.Sprintf("%d registered", len(projectNames)), err)
	report.Counts.Runtimes, report.Counts.Projects = len(runtimeNames), len(projectNames)
	runtimes := map[string]Runtime{}
	projects := map[string]Project{}
	for _, name := range runtimeNames {
		runtime, err := LoadRuntime(filepath.Join(dataDir, "runtimes", name+".yaml"), name)
		add("runtime:"+name+":config", "configuration is valid; no runtime command was invoked", err)
		if err == nil {
			runtimes[name] = runtime
		}
	}
	for _, name := range projectNames {
		project, err := LoadProject(filepath.Join(dataDir, "projects", name+".yaml"), name)
		add("project:"+name+":config", "configuration is valid", err)
		if err == nil {
			projects[name] = project
			report.Counts.ProjectChecks += len(project.Checks)
		}
	}
	for _, name := range runtimeNames {
		runtime, ok := runtimes[name]
		if !ok {
			continue
		}
		command := runtime.Argv[0]
		if !filepath.IsAbs(command) && strings.ContainsRune(command, '/') {
			// Such a command runs relative to a project's worktree. Inspect the
			// registered checkout, rather than accidentally checking the CLI cwd.
			linked := false
			for _, projectName := range projectNames {
				project, valid := projects[projectName]
				if !valid || project.Runtime != name {
					continue
				}
				linked = true
				path, err := doctorExecutable(command, project.Root)
				add("runtime:"+name+":project:"+projectName+":executable", path, err)
			}
			if !linked {
				skip("runtime:"+name+":executable", "relative executable has no valid registered project; use an absolute path or register its project")
			}
		} else {
			path, err := doctorExecutable(command, "")
			add("runtime:"+name+":executable", path, err)
		}
	}
	for _, name := range projectNames {
		project, ok := projects[name]
		if !ok {
			continue
		}
		if _, ok := runtimes[project.Runtime]; ok {
			add("project:"+name+":runtime", project.Runtime, nil)
		} else {
			add("project:"+name+":runtime", "", errors.New("referenced runtime is missing or invalid"))
		}
		if gitErr != nil {
			skip("project:"+name+":git", "Git executable is unavailable")
		} else if ctx.Err() != nil {
			add("project:"+name+":git", "", ctx.Err())
		} else {
			repository, err := InspectRepository(ctx, project.Root, project.Base)
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			add("project:"+name+":git", fmt.Sprintf("common_dir=%s base_commit=%s", repository.CommonDir, repository.BaseSHA), err)
		}
		for _, check := range project.Checks {
			path, err := doctorExecutable(check.Argv[0], project.Root)
			add("project:"+name+":check:"+check.Name+":executable", path, err)
		}
	}
	report.Ready = report.InstallationReady && report.Counts.Projects > 0 && report.Counts.Runtimes > 0 && report.Counts.Failed == 0 && report.Counts.Skipped == 0
	switch {
	case report.Counts.Failed > 0:
		report.Status = "needs_attention"
	case report.Ready:
		report.Status = "configured_prerequisites_checked"
	default:
		report.Status = "installation_checked"
	}
	return report
}

func doctorExecutable(command, root string) (string, error) {
	if strings.ContainsRune(command, '/') && !filepath.IsAbs(command) {
		if root == "" {
			return "", errors.New("relative executable needs a project directory")
		}
		command = filepath.Join(root, command)
	}
	return exec.LookPath(command)
}

func doctorDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return errors.New("data directory must be a real directory with mode 0700")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("data directory must be owned by the executing user")
	}
	return nil
}

func doctorWriteProbe(path string) (err error) {
	file, err := os.CreateTemp(path, ".doctor-write-*")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close(), os.Remove(file.Name())) }()
	if _, err = file.WriteString("eng doctor local write check\n"); err != nil {
		return err
	}
	return file.Sync()
}
