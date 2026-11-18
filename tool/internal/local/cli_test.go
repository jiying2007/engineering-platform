package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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
