package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCLIFailedExecutionReturnsEvidenceAndNonzero(t *testing.T) {
	store, root := executionStore(t), executionRepository(t)
	project := Project{Name: "failure", Root: root, Base: "main", Runtime: "failure"}
	runtime := Runtime{Name: "failure", Argv: []string{"/bin/sh", "-c", "printf actual-failure >&2; exit 7"}, Stdin: "task", Timeout: "5s", Model: false}
	if err := saveRegistration(store.DataDir(), "projects", project.Name, project, false); err != nil {
		t.Fatal(err)
	}
	if err := saveRegistration(store.DataDir(), "runtimes", runtime.Name, runtime, false); err != nil {
		t.Fatal(err)
	}
	instruction := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(instruction, []byte("A controlled failure must not look like a successful CLI invocation.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := CLI(context.Background(), []string{"--data-dir", store.DataDir(), "run", project.Name, "--task", instruction}, &stdout, &stderr)
	var result Task
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("missing structured task: %s %s %v", &stdout, &stderr, err)
	}
	if code == 0 || result.State != StateFailed || !strings.Contains(stderr.String(), "failed") {
		t.Fatalf("failed execution returned success: %d %+v %s", code, result, &stderr)
	}
	attempt := lastExecutionAttempt(t, store, result)
	if attempt.Result.RuntimeExitCode != 7 || !attempt.Result.LocalProcessStopped {
		t.Fatalf("failure evidence lost: %+v", attempt.Result)
	}
}

func TestCLIRejectsFIFOInstructionWithoutOpeningAWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstruction(path); err == nil {
		t.Fatal("FIFO accepted as task input")
	}
}

func TestCLIProjectAddUsesCheckedOutBranchAndHonorsExplicitBase(t *testing.T) {
	cases := []struct {
		name     string
		config   string
		base     string
		detached bool
		wantBase string
		wantFail bool
	}{
		{name: "no config defaults to master", wantBase: "master"},
		{name: "yaml without base defaults to master", config: "runtime: fixture\n", wantBase: "master"},
		{name: "explicit yaml base is not silently replaced", config: "runtime: fixture\nbase: main\n", wantFail: true},
		{name: "explicit cli base overrides yaml", config: "runtime: fixture\nbase: main\n", base: "master", wantBase: "master"},
		{name: "explicit missing branch fails closed", base: "missing-branch", wantFail: true},
		{name: "detached HEAD pins exact commit", detached: true, wantBase: "SHA"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			store, err := OpenStore(filepath.Join(t.TempDir(), "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			root := filepath.Join(t.TempDir(), "repo")
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", args...)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			git("init", "-b", "master", root)
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("test repo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			git("-C", root, "add", "README.md")
			git("-C", root, "-c", "user.name=eng test", "-c", "user.email=eng@localhost", "commit", "-m", "baseline")
			baseSHA := git("-C", root, "rev-parse", "HEAD")
			if test.detached {
				git("-C", root, "checkout", "--detach", "HEAD")
			}
			if test.config != "" {
				if err := os.WriteFile(filepath.Join(root, ".eng.yaml"), []byte(test.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			runtime := Runtime{Name: "fixture", Argv: []string{"/bin/true"}, Stdin: "none", Timeout: "5s", Model: false}
			if err := saveRegistration(store.DataDir(), "runtimes", runtime.Name, runtime, false); err != nil {
				t.Fatal(err)
			}
			args := []string{"--data-dir", store.DataDir(), "project", "add", "actual", root, "--runtime", "fixture"}
			if test.base != "" {
				args = append(args, "--base", test.base)
			}
			var stdout, stderr bytes.Buffer
			code := CLI(context.Background(), args, &stdout, &stderr)
			if test.wantFail {
				if code == 0 || !strings.Contains(stderr.String(), "project base") {
					t.Fatalf("must reject unavailable explicitly selected base: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
				return
			}
			if code != 0 {
				t.Fatalf("registration failed: %s %s", &stdout, &stderr)
			}
			var project Project
			if err := json.Unmarshal(stdout.Bytes(), &project); err != nil {
				t.Fatal(err)
			}
			expected := test.wantBase
			if expected == "SHA" {
				expected = baseSHA
			}
			if project.Base != expected || project.Root != root || project.Runtime != "fixture" {
				t.Fatalf("registered wrong base or project: %+v", project)
			}
			info, err := InspectRepository(context.Background(), project.Root, project.Base)
			if err != nil || info.BaseSHA != baseSHA {
				t.Fatalf("frozen target resolved incorrectly: %+v %v", info, err)
			}
		})
	}
}
