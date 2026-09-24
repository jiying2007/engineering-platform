package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

func TestSupervisorCapturesOutputAndInput(t *testing.T) {
	s := New(runtimeprovider.NewLocalProcessProvider("test"))
	events, err := s.Start(
		context.Background(),
		"run-1",
		"attempt-1",
		1,
		runtimeprovider.LaunchSpec{
			Executable: "/bin/sh",
			Args:       []string{"-c", "read line; echo out:$line; echo err:$line >&2"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Input("run-1", 1, "hello\n"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bool
	for event := range events {
		if event.Stream == Stdout && strings.Contains(event.Data, "out:hello") {
			stdout = true
		}
		if event.Stream == Stderr && strings.Contains(event.Data, "err:hello") {
			stderr = true
		}
	}
	result, err := s.Wait("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !stdout || !stderr {
		t.Fatalf("unexpected result=%#v stdout=%v stderr=%v", result, stdout, stderr)
	}
}

func TestSupervisorRejectsStaleEpochAndCanAbort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := New(runtimeprovider.NewLocalProcessProvider("test"))
	_, err := s.Start(ctx, "run-2", "attempt-1", 2, runtimeprovider.LaunchSpec{
		Executable: "/bin/sh",
		Args:       []string{"-c", "sleep 30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Pause("run-2", 1); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("expected stale epoch, got %v", err)
	}
	if err := s.Abort("run-2", 2); err != nil {
		t.Fatal(err)
	}
	result, err := s.Wait("run-2")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("aborted process should not exit 0: %#v", result)
	}
}

func TestSupervisorPauseResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := New(runtimeprovider.NewLocalProcessProvider("test"))
	_, err := s.Start(ctx, "run-3", "attempt-1", 1, runtimeprovider.LaunchSpec{
		Executable: "/bin/sh",
		Args:       []string{"-c", "sleep 30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Pause("run-3", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume("run-3", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Abort("run-3", 1); err != nil {
		t.Fatal(err)
	}
	_, _ = s.Wait("run-3")
}
