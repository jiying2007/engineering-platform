package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinalizeCreatesIndependentResultCommitAndBundle(t *testing.T) {
	ctx := context.Background()
	repo, base := initRepository(t)
	root := t.TempDir()
	manager, err := New(filepath.Join(root, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := manager.Create(ctx, Spec{ID: "finalize", Repository: repo, BaseCommit: base})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.WorktreePath, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.Mkdir(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}
	finalized, err := manager.Finalize(ctx, w, artifacts, "result")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Facts.Recipe != FinalizeRecipe ||
		finalized.Facts.BaseCommit != strings.ToLower(base) ||
		finalized.Facts.ResultCommit == finalized.Facts.BaseCommit ||
		finalized.Facts.ResultTree == finalized.Facts.BaseTree ||
		finalized.Facts.ResultSourceDigest == finalized.Facts.BaseSourceDigest ||
		finalized.Facts.BundleDigest == "" || finalized.Facts.BundleSize <= 0 {
		t.Fatalf("unexpected finalization: %#v", finalized)
	}
	if _, err := os.Stat(finalized.BundlePath); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(repo, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(source) != "base\n" {
		t.Fatalf("finalization mutated source repository: %q", source)
	}
}

func TestFinalizeRejectsNoChangeAndUnsafeArtifactRoot(t *testing.T) {
	ctx := context.Background()
	repo, base := initRepository(t)
	root := t.TempDir()
	manager, err := New(filepath.Join(root, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := manager.Create(ctx, Spec{ID: "finalize-noop", Repository: repo, BaseCommit: base})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.Mkdir(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Finalize(ctx, w, artifacts, "noop"); err == nil {
		t.Fatal("clean workspace finalized")
	}
	if err := os.WriteFile(filepath.Join(w.WorktreePath, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Finalize(ctx, w, filepath.Join(root, "managed"), "unsafe"); err == nil {
		t.Fatal("managed workspace accepted as artifact root")
	}
}
