package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

type RepositoryInfo struct {
	Root      string `json:"root"`
	CommonDir string `json:"common_dir"`
	BaseSHA   string `json:"base_sha"`
}

func gitEnvironment() []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
}

func gitCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	base := []string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "core.fsmonitor=false", "-C", root}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = gitEnvironment()
	// These bounded local Git operations must not outlive a killed scheduler.
	// Native runtimes use the separately persisted process-group handshake.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	return cmd
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := gitCommand(ctx, root, args...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %.2048s", args[0], err, strings.TrimSpace(string(raw)))
	}
	return strings.TrimSpace(string(raw)), nil
}

// Git filter drivers can execute arbitrary programs during checkout or add.
// A short-lived Git process does not supervise their descendants, so reject
// executable filters before creating a worktree or staging files.
func rejectExecutableGitFilters(ctx context.Context, root string) error {
	cmd := gitCommand(ctx, root, "config", "--includes", "--name-only", "--get-regexp", "^filter\\..*\\.(process|clean|smudge)$")
	_, err := cmd.Output()
	if err == nil {
		return fmt.Errorf("repository defines executable Git filters; remove them or use an independently supervised checkout")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("inspect executable Git filters: %w", err)
}

// InspectRepository resolves the actual common directory so aliases, linked
// worktrees and separate eng data directories cannot bypass the repository lock.
func InspectRepository(ctx context.Context, root, base string) (RepositoryInfo, error) {
	var result RepositoryInfo
	abs, err := filepath.Abs(root)
	if err != nil {
		return result, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return result, err
	}
	top, err := gitOutput(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return result, err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil {
		return result, err
	}
	if err := rejectExecutableGitFilters(ctx, top); err != nil {
		return result, err
	}
	common, err := gitOutput(ctx, abs, "rev-parse", "--git-common-dir")
	if err != nil {
		return result, err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(abs, common)
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return result, err
	}
	if base == "" {
		base = "HEAD"
	}
	sha, err := gitOutput(ctx, abs, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return result, err
	}
	if len(sha) != 40 && len(sha) != 64 {
		return result, fmt.Errorf("unexpected Git object identity")
	}
	return RepositoryInfo{Root: top, CommonDir: common, BaseSHA: sha}, nil
}

func AcquireRepository(ctx context.Context, root string) (*Lock, RepositoryInfo, error) {
	info, err := InspectRepository(ctx, root, "HEAD")
	if err != nil {
		return nil, info, err
	}
	lock, err := acquireFileLock(filepath.Join(info.CommonDir, "eng-execution.lock"))
	if err != nil {
		return nil, info, err
	}
	// A native CLI may close inherited descriptors. The shared repository record
	// therefore survives a dead scheduler even when another data directory asks
	// for this same Git common directory.
	raw, err := io.ReadAll(io.LimitReader(lock.file, 4097))
	if err != nil || len(raw) > 4096 {
		lock.Close()
		return nil, info, fmt.Errorf("cannot read repository process record")
	}
	if len(strings.TrimSpace(string(raw))) != 0 && strings.TrimSpace(string(raw)) != "null" {
		var id ProcessIdentity
		if json.Unmarshal(raw, &id) != nil {
			lock.Close()
			return nil, info, fmt.Errorf("repository process identity is unreadable; inspect the existing task")
		}
		alive, observeErr := ProcessGroupAlive(id)
		if observeErr != nil || alive {
			lock.Close()
			return nil, info, fmt.Errorf("%w: repository retains a prior process group: %v", ErrProcessUnresolved, observeErr)
		}
	}
	return lock, info, nil
}

var taskPathID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func taskDirectory(dataDir, taskID string) (string, error) {
	if !taskPathID.MatchString(taskID) {
		return "", fmt.Errorf("invalid task identity")
	}
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "tasks", taskID), nil
}

func prepareWorkspace(ctx context.Context, store *Store, task *Task) error {
	dir, err := taskDirectory(store.DataDir(), task.ID)
	if err != nil {
		return err
	}
	worktree := filepath.Join(dir, "worktree")
	branch := "eng/" + task.ID
	if task.Worktree != "" && (task.Worktree != worktree || task.Branch != branch) {
		return fmt.Errorf("stored task workspace identity changed")
	}
	info, err := InspectRepository(ctx, task.Config.Project.Root, task.Config.Project.Base)
	if err != nil {
		return err
	}
	if task.BaseSHA == "" {
		task.BaseSHA = info.BaseSHA
	}
	task.Worktree, task.Branch = worktree, branch
	// Persist locators before Git creates either branch or working directory.
	if err := store.UpdateTask(ctx, *task); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if stat, err := os.Lstat(worktree); err == nil {
		if !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("task workspace is not an owned directory")
		}
		return verifyWorkspace(ctx, *task)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// If a crash created the task branch but not its worktree, an explicit resume
	// can attach only the exact intended base. It never resets an existing branch.
	ref, refErr := gitOutput(ctx, info.Root, "rev-parse", "--verify", "refs/heads/"+branch)
	if refErr == nil {
		if ref != task.BaseSHA {
			return fmt.Errorf("existing task branch differs from recorded base; inspect before recovery")
		}
		_, err = gitOutput(ctx, info.Root, "worktree", "add", "--", worktree, branch)
	} else {
		_, err = gitOutput(ctx, info.Root, "worktree", "add", "-b", branch, "--", worktree, task.BaseSHA)
	}
	if err != nil {
		return err
	}
	return verifyWorkspace(ctx, *task)
}

func verifyWorkspace(ctx context.Context, task Task) error {
	expected, err := InspectRepository(ctx, task.Config.Project.Root, task.BaseSHA)
	if err != nil {
		return err
	}
	actual, err := InspectRepository(ctx, task.Worktree, "HEAD")
	if err != nil {
		return err
	}
	if actual.Root != task.Worktree || actual.CommonDir != expected.CommonDir {
		return fmt.Errorf("worktree no longer belongs to the frozen repository")
	}
	branch, err := gitOutput(ctx, task.Worktree, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch != task.Branch {
		return fmt.Errorf("runtime changed task branch; manual inspection required")
	}
	if err := gitCommand(ctx, task.Worktree, "merge-base", "--is-ancestor", task.BaseSHA, "HEAD").Run(); err != nil {
		return fmt.Errorf("task result no longer descends from its frozen baseline")
	}
	return nil
}

func snapshotWorkspace(ctx context.Context, task Task, message string) (string, error) {
	if err := verifyWorkspace(ctx, task); err != nil {
		return "", err
	}
	if _, err := gitOutput(ctx, task.Worktree, "add", "-u", "--", "."); err != nil {
		return "", err
	}
	// Generated, untracked artifacts have their own hashed copy. Do not silently
	// turn a build result from the previous attempt into source on resume.
	untracked, err := untrackedSourcePaths(ctx, task)
	if err != nil {
		return "", err
	}
	for _, path := range untracked {
		if _, err := gitOutput(ctx, task.Worktree, "add", "--", path); err != nil {
			return "", err
		}
	}
	cmd := gitCommand(ctx, task.Worktree, "diff", "--cached", "--quiet", "--exit-code")
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", fmt.Errorf("inspect staged changes: %w", err)
		}
		if _, err := gitOutput(ctx, task.Worktree, "-c", "user.name=eng task", "-c", "user.email=eng@localhost", "commit", "--no-verify", "-m", message); err != nil {
			return "", err
		}
	}
	return gitOutput(ctx, task.Worktree, "rev-parse", "HEAD")
}

func writeWorkspaceDiff(ctx context.Context, task Task, resultCommit, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd := gitCommand(ctx, task.Worktree, "diff", "--binary", "--no-ext-diff", task.BaseSHA, resultCommit, "--")
	cmd.Stdout = file
	var errorsOut strings.Builder
	cmd.Stderr = &errorsOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("capture diff: %w: %.2048s", err, errorsOut.String())
	}
	return file.Sync()
}

func trackedWorkspaceChanged(ctx context.Context, task Task, expected string) (bool, error) {
	if err := verifyWorkspace(ctx, task); err != nil {
		return true, err
	}
	head, err := gitOutput(ctx, task.Worktree, "rev-parse", "HEAD")
	if err != nil {
		return true, err
	}
	if head != expected {
		return true, nil
	}
	cmd := gitCommand(ctx, task.Worktree, "diff", "--quiet", "--exit-code", "HEAD", "--")
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return true, nil
		}
		return true, err
	}
	untracked, err := untrackedSourcePaths(ctx, task)
	return len(untracked) != 0, err
}

func untrackedSourcePaths(ctx context.Context, task Task) ([]string, error) {
	raw, err := gitCommand(ctx, task.Worktree, "ls-files", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect untracked source: %w", err)
	}
	paths := []string{}
	for _, path := range strings.Split(string(raw), "\x00") {
		if path == "" {
			continue
		}
		artifact := false
		for _, pattern := range task.Config.Project.Artifacts {
			if artifactMatch(pattern, path) {
				artifact = true
				break
			}
		}
		if !artifact {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// artifactMatch gives ** a directory-recursive meaning; ordinary segments use
// filepath.Match. No external globbing package or shell expansion is needed.
func artifactMatch(pattern, name string) bool {
	parts, path := strings.Split(filepath.ToSlash(pattern), "/"), strings.Split(filepath.ToSlash(name), "/")
	var match func(int, int) bool
	match = func(i, j int) bool {
		if i == len(parts) {
			return j == len(path)
		}
		if parts[i] == "**" {
			if match(i+1, j) {
				return true
			}
			return j < len(path) && match(i, j+1)
		}
		if j == len(path) {
			return false
		}
		ok, err := filepath.Match(parts[i], path[j])
		return err == nil && ok && match(i+1, j+1)
	}
	return match(0, 0)
}

func captureArtifacts(ctx context.Context, task Task, destination string) ([]Artifact, error) {
	artifacts := []Artifact{}
	patterns := task.Config.Project.Artifacts
	if len(patterns) == 0 {
		return artifacts, nil
	}
	hits := make([]int, len(patterns))
	var total int64
	err := filepath.WalkDir(task.Worktree, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(task.Worktree, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		selected := false
		for i, pattern := range patterns {
			if artifactMatch(pattern, rel) {
				hits[i]++
				selected = true
			}
		}
		if !selected {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact %s is not a regular file", rel)
		}
		if len(artifacts) >= 1024 || info.Size() > 128<<20 || total+info.Size() > 512<<20 {
			return fmt.Errorf("artifact capture exceeds 1024 files, 128 MiB per file or 512 MiB total")
		}
		target := filepath.Join(destination, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		actual, err := input.Stat()
		if err != nil || !os.SameFile(info, actual) {
			return fmt.Errorf("artifact %s changed before capture", rel)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		hash := sha256.New()
		copied, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, (128<<20)+1))
		syncErr, closeErr := output.Sync(), output.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil {
			return errors.Join(copyErr, syncErr, closeErr)
		}
		if copied != actual.Size() {
			return fmt.Errorf("artifact %s changed size during capture", rel)
		}
		total += copied
		artifacts = append(artifacts, Artifact{Path: target, Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), Size: copied})
		return nil
	})
	if err != nil {
		return artifacts, err
	}
	for i, hits := range hits {
		if hits == 0 {
			return artifacts, fmt.Errorf("required artifact pattern has no regular file: %s", patterns[i])
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}
