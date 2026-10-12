package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type cleanupStepContext struct {
	context.Context
	calls  int
	stopAt int
	cancel context.CancelFunc
}

func (c *cleanupStepContext) Err() error {
	c.calls++
	if c.calls == c.stopAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCleanupStopsMidTraversalAndPreservesExactOwner(t *testing.T) {
	repo, commit := initRepository(t)
	m, err := New(filepath.Join(t.TempDir(), "slots"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := m.Create(context.Background(), Spec{ID: "cancel-walk", Repository: repo, BaseCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 260; i++ {
		if err := os.WriteFile(filepath.Join(w.HomePath, fmt.Sprintf("f%03d", i)), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "keep")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(w.HomePath, "outside")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Join(m.root, w.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before, err := root.ReadFile("owner.json")
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cleanupStepContext{Context: parent, stopAt: 30, cancel: cancel}
	budget := cleanupMaxEntries
	if err := cleanupContents(ctx, root, 0, &budget, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("partial traversal claimed success: %v", err)
	}
	after, err := root.ReadFile("owner.json")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("partial traversal lost owner")
	}
	if err := m.Cleanup(parent, w); !errors.Is(err, ErrCleanupIncomplete) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cleanup lost classification: %v", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "untouched" {
		t.Fatal("outside link was followed")
	}
	// No detached cleanup is running. A new explicit call uses the same owner;
	// it does not reconstruct a Runtime, replay work, or delete another slot.
	if err := m.Cleanup(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(m.root, w.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("normal cleanup not complete", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "untouched" {
		t.Fatal("cleanup touched outside target")
	}
}

func TestCleanupEntryAndDepthBudgetsFailClosed(t *testing.T) {
	path := t.TempDir()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"owner.json", "a", "b", "c"} {
		if err := root.WriteFile(name, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	budget := 1
	if err := cleanupContents(context.Background(), root, 0, &budget, true); err == nil {
		t.Fatal("entry budget ignored")
	}
	if _, err := root.ReadFile("owner.json"); err != nil {
		t.Fatal("budget removed ownership")
	}
	budget = 10
	if err := cleanupContents(context.Background(), root, cleanupMaxDepth+1, &budget, true); err == nil {
		t.Fatal("depth budget ignored")
	}
}

func TestCleanupAfterFailureOwnsIndependentLifecycle(t *testing.T) {
	repo, commit := initRepository(t)
	m, err := New(filepath.Join(t.TempDir(), "slots"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := m.Create(context.Background(), Spec{ID: "cleanup-failed-preparation", Repository: repo, BaseCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cancel()
	if err := m.CleanupAfterFailure(ctx, w); err != nil {
		t.Fatal("caller cancellation erased cleanup lifecycle", err)
	}
	if _, err := os.Lstat(filepath.Join(m.root, w.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed preparation not removed", err)
	}
}

func TestCleanupRejectsSymlinkOwnershipRecord(t *testing.T) {
	repo, commit := initRepository(t)
	m, err := New(filepath.Join(t.TempDir(), "slots"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := m.Create(context.Background(), Spec{ID: "bad-owner", Repository: repo, BaseCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(m.root, w.ID, "owner.json")
	if err := os.Rename(owner, owner+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("owner.json.original", owner); err != nil {
		t.Fatal(err)
	}
	if err := m.Cleanup(context.Background(), w); !errors.Is(err, ErrCleanupIncomplete) || !errors.Is(err, ErrUnmanagedWorkspace) {
		t.Fatalf("symlink owner accepted: %v", err)
	}
	if _, err := os.Stat(w.WorktreePath); err != nil {
		t.Fatal("invalid owner allowed deletion")
	}
}

func TestCleanupOwnerRestorationDoesNotOverwriteReplacement(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("owner.json", []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreCleanupOwner(root, []byte("original")); !errors.Is(err, os.ErrExist) {
		t.Fatal("replacement owner was overwritten", err)
	}
	raw, err := root.ReadFile("owner.json")
	if err != nil || string(raw) != "replacement" {
		t.Fatal("replacement changed")
	}
}
