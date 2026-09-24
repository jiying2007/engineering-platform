package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndependentWorkspaceCancellationReapsChildrenAndOwnedSlot(t *testing.T) {
	repo, commit := initRepository(t)
	base := t.TempDir()
	ready, escaped, release := filepath.Join(base, "ready"), filepath.Join(base, "escaped"), filepath.Join(base, "release")
	git := filepath.Join(base, "fixture-git")
	// A trusted test executable simulates a Git child which would outlive its
	// parent without group termination. It never reads repository data or keys.
	script := "#!/bin/sh\n(while [ ! -e '" + release + "' ]; do sleep 0.01; done; printf unsafe > '" + escaped + "') &\nprintf ready > '" + ready + "'\nwait\n"
	if err := os.WriteFile(git, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m, err := NewWithGit(filepath.Join(base, "managed"), git)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.Create(ctx, Spec{ID: "cancelled", Repository: repo, BaseCommit: commit}); done <- err }()
	deadline := time.After(5 * time.Second)
	for !pathExists(ready) {
		select {
		case err := <-done:
			t.Fatalf("Git fixture exited before cancellation: %v", err)
		case <-deadline:
			cancel()
			<-done
			t.Fatal("Git fixture did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled preparation succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled Git process did not join")
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if pathExists(escaped) || pathExists(filepath.Join(m.root, "cancelled")) {
		t.Fatal("child process or partial workspace survived cancellation")
	}
}
