package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func doctorDataDir(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func doctorResult(t *testing.T, dataDir string) doctorReport {
	t.Helper()
	value, err := doctor(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return value.(doctorReport)
}

func doctorCheckStatus(t *testing.T, report doctorReport, name string) string {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Status
		}
	}
	t.Fatalf("missing diagnostic %q: %+v", name, report)
	return ""
}

func TestDoctorUnconfiguredInstallationDoesNotClaimProjectReadiness(t *testing.T) {
	dataDir := doctorDataDir(t)
	report := doctorResult(t, dataDir)
	if report.Ready || !report.InstallationReady || report.Status != "installation_checked" || report.Counts.Projects != 0 || report.Counts.Runtimes != 0 || report.ModelInvocations != 0 {
		t.Fatalf("installation-only scope was lost: %+v", report)
	}
	if report.Disk == nil || report.Disk.TotalBytes == 0 || report.Disk.BlockSize == 0 || report.Disk.AvailableBytes > report.Disk.FreeBytes {
		t.Fatalf("missing or inconsistent filesystem capacity: %+v", report.Disk)
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("write probe left files behind: %+v, %v", entries, err)
	}
}

func TestDoctorInspectsConfiguredCommandsWithoutExecutingThem(t *testing.T) {
	dataDir, root := doctorDataDir(t), executionRepository(t)
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	runtimePath := filepath.Join(t.TempDir(), "runtime")
	checkPath := filepath.Join(root, "project-check")
	for _, path := range []string{runtimePath, checkPath} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf invoked > '"+marker+"'\nexit 97\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	runtime := Runtime{Name: "fixture", Argv: []string{runtimePath, "--version"}, Stdin: "task", Timeout: "1m", Model: true}
	project := Project{Name: "example", Root: root, Base: "main", Runtime: "fixture", Checks: []Check{{Name: "local", Argv: []string{"./project-check"}, Timeout: "1m"}}}
	if err := saveRegistration(dataDir, "runtimes", runtime.Name, runtime, false); err != nil {
		t.Fatal(err)
	}
	if err := saveRegistration(dataDir, "projects", project.Name, project, false); err != nil {
		t.Fatal(err)
	}
	report := doctorResult(t, dataDir)
	if !report.Ready || report.Status != "configured_prerequisites_checked" || report.Counts.Projects != 1 || report.Counts.Runtimes != 1 || report.Counts.ProjectChecks != 1 || report.ModelInvocations != 0 {
		t.Fatalf("registered local prerequisites: %+v", report)
	}
	if doctorCheckStatus(t, report, "project:example:check:local:executable") != "passed" {
		t.Fatal("relative check executable was not resolved at its project")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("doctor invoked a configured command: %v", err)
	}
	if report.RuntimeVersions != "not probed" || report.Credentials != "not inspected" {
		t.Fatalf("unexpected inspection scope: %+v", report)
	}
}

func TestDoctorReportsMissingExecutablesAndGitBase(t *testing.T) {
	dataDir, root := doctorDataDir(t), executionRepository(t)
	runtime := Runtime{Name: "missing", Argv: []string{filepath.Join(t.TempDir(), "absent-runtime")}, Stdin: "none", Timeout: "1m"}
	project := Project{Name: "example", Root: root, Base: "nonexistent-base", Runtime: runtime.Name, Checks: []Check{{Name: "missing", Argv: []string{"./absent-check"}, Timeout: "1m"}}}
	if err := saveRegistration(dataDir, "runtimes", runtime.Name, runtime, false); err != nil {
		t.Fatal(err)
	}
	if err := saveRegistration(dataDir, "projects", project.Name, project, false); err != nil {
		t.Fatal(err)
	}
	report := doctorResult(t, dataDir)
	if report.Ready || report.Status != "needs_attention" || report.ModelInvocations != 0 {
		t.Fatalf("broken project was ready: %+v", report)
	}
	for _, name := range []string{"runtime:missing:executable", "project:example:git", "project:example:check:missing:executable"} {
		if doctorCheckStatus(t, report, name) != "failed" {
			t.Fatalf("missing failure for %s", name)
		}
	}
}

func TestDoctorRejectsSharedOrSymlinkDataDirectory(t *testing.T) {
	dataDir := doctorDataDir(t)
	if err := os.Chmod(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "state-link")
	if err := os.Symlink(dataDir, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dataDir, link} {
		report := doctorResult(t, path)
		if report.InstallationReady || doctorCheckStatus(t, report, "data_dir:permissions") != "failed" || doctorCheckStatus(t, report, "data_dir:write_sync") != "skipped" {
			t.Fatalf("unsuitable directory accepted: %+v", report)
		}
	}
}

func TestDoctorSharesDeadlineAcrossLocalGitInspection(t *testing.T) {
	dataDir, root := doctorDataDir(t), executionRepository(t)
	runtime := Runtime{Name: "fixture", Argv: []string{"/bin/true"}, Stdin: "none", Timeout: "1m"}
	project := Project{Name: "example", Root: root, Base: "main", Runtime: runtime.Name}
	if err := saveRegistration(dataDir, "runtimes", runtime.Name, runtime, false); err != nil {
		t.Fatal(err)
	}
	if err := saveRegistration(dataDir, "projects", project.Name, project, false); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec /bin/sleep 5\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	report := inspectDoctor(ctx, dataDir)
	if time.Since(started) > time.Second || report.Ready || doctorCheckStatus(t, report, "project:example:git") != "failed" {
		t.Fatalf("local Git inspection did not honor its deadline: %+v", report)
	}
	for _, check := range report.Checks {
		if check.Name == "project:example:git" && !strings.Contains(check.Detail, "deadline") {
			t.Fatalf("deadline reason was not retained: %+v", check)
		}
	}
}
