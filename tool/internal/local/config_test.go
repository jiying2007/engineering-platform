package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigForTest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigRejectsMisspelledLimitsAndMultipleDocuments(t *testing.T) {
	for _, body := range []string{
		"runtime: fixture\nchecks:\n  - name: test\n    argv: [true]\n    timeot: 5s\n",
		"runtime: fixture\nruntime: replacement\n",
		"runtime: fixture\n---\nruntime: replacement\n",
		"runtime: fixture\nchecks:\n  - name: test\n    argv: [echo, ok]\n    timeout: 0s\n",
		"runtime: fixture\nartifacts: ['../private-key']\n",
		"runtime: fixture\nartifacts: ['.git/config']\n",
	} {
		if _, err := LoadProject(writeConfigForTest(t, body), "demo"); err == nil {
			t.Fatalf("unsafe or ambiguous project config accepted: %s", body)
		}
	}
}

func TestConfigRuntimeRequiresExplicitModelDeclaration(t *testing.T) {
	path := writeConfigForTest(t, "argv: [true]\nstdin: none\ntimeout: 5s\n")
	if _, err := LoadRuntime(path, "fixture"); err == nil {
		t.Fatal("omitted model flag silently became a non-model fixture")
	}
	path = writeConfigForTest(t, "argv: [cat]\nstdin: task\ntimeout: 5s\nmodel: false\n")
	runtime, err := LoadRuntime(path, "fixture")
	if err != nil || runtime.Model || runtime.Name != "fixture" {
		t.Fatalf("explicit non-model command: %+v %v", runtime, err)
	}
}

func TestConfigRejectsRecordedCredentialsWithoutEchoingThem(t *testing.T) {
	for _, argv := range []string{
		"[runtime, --api-key, do-not-print-this-secret]",
		"[runtime, '--token=do-not-print-this-secret']",
		"[runtime, 'https://name:do-not-print-this-secret@example.test/path']",
	} {
		path := writeConfigForTest(t, "argv: "+argv+"\nmodel: true\ntimeout: 5s\n")
		_, err := LoadRuntime(path, "registered")
		if err == nil || strings.Contains(err.Error(), "do-not-print-this-secret") {
			t.Fatalf("credential was accepted or echoed: %v", err)
		}
	}
}

func TestConfigFreezesDeepCopyAndResolvesRelativeRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "source"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(path, []byte("root: source\nruntime: fixture\nchecks:\n  - name: test\n    argv: [echo, success]\nartifacts: ['out/**/*.elf']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject(path, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Name: "fixture", Argv: []string{"cat"}, Model: false}
	frozen, digest, err := Freeze(project, runtime)
	if err != nil {
		t.Fatal(err)
	}
	project.Checks[0].Argv[1] = "changed"
	runtime.Argv[0] = "other-executable"
	if frozen.Project.Root != filepath.Join(dir, "source") || frozen.Project.Base != "main" || frozen.Project.Checks[0].Timeout != "10m" || frozen.Project.Checks[0].Argv[1] != "success" || frozen.Runtime.Argv[0] != "cat" || frozen.Runtime.Timeout != "30m" {
		t.Fatalf("freeze changed with caller settings: %+v", frozen)
	}
	observed, _ := snapshotDigest(frozen)
	if digest != observed {
		t.Fatal("frozen configuration digest changed")
	}
}

func TestConfigRejectsDirectoryAndOversizedInput(t *testing.T) {
	if _, err := LoadRuntime(t.TempDir(), "fixture"); err == nil {
		t.Fatal("directory accepted as runtime config")
	}
	if _, err := LoadRuntime(writeConfigForTest(t, strings.Repeat("#", maxConfigBytes+1)), "fixture"); err == nil {
		t.Fatal("oversized config accepted")
	}
}

func TestConfigLocalOverridesPrecedeValidation(t *testing.T) {
	path := writeConfigForTest(t, "checks:\n  - name: build\n    argv: [echo, build]\n")
	root := t.TempDir()
	project, err := LoadProjectOverrides(path, "demo", root, "topic/fix", "fixture")
	if err != nil || project.Root != root || project.Runtime != "fixture" || project.Base != "topic/fix" {
		t.Fatalf("local registration could not complete shared checks config: %+v %v", project, err)
	}
	bad := writeConfigForTest(t, "runtime: ignored\ntimeot: 5s\n")
	if _, err := LoadProjectOverrides(bad, "demo", root, "main", "fixture"); err == nil {
		t.Fatal("overrides hid an unknown configuration field")
	}
}
