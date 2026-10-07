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


func TestFinalizeBundleRemainsSelfContainedAfterSourceRemoval(t *testing.T) {
	ctx := context.Background()
	repo, base := initRepository(t)
	root := t.TempDir()
	manager, err := New(filepath.Join(root, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := manager.Create(ctx, Spec{ID: "self-contained", Repository: repo, BaseCommit: base})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.WorktreePath, "hello.txt"), []byte("self contained\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.Mkdir(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}
	finalized, err := manager.Finalize(ctx, w, artifacts, "self-contained")
	if err != nil {
		t.Fatal(err)
	}
	// Prove the bundle does not rely on either original object store.
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "managed")); err != nil {
		t.Fatal(err)
	}
	independent := filepath.Join(root, "independent")
	home := filepath.Join(root, "independent-home")
	template := filepath.Join(root, "independent-template")
	for _, path := range []string{independent, home, template} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.gitOutput(ctx, independent, home, "init", "--template="+template, "--object-format=sha1"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.gitOutput(ctx, independent, home, "fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", "--", finalized.BundlePath, "HEAD"); err != nil {
		t.Fatal(err)
	}
	for _, commit := range []string{finalized.Facts.BaseCommit, finalized.Facts.ResultCommit} {
		if _, err := manager.gitOutput(ctx, independent, home, "cat-file", "-e", commit+"^{commit}"); err != nil {
			t.Fatal("self-contained bundle omitted commit", commit, err)
		}
	}
	parents, err := manager.gitOutput(ctx, independent, home, "rev-list", "--parents", "-n", "1", finalized.Facts.ResultCommit)
	fields := strings.Fields(parents)
	if err != nil || len(fields) != 2 || fields[1] != finalized.Facts.BaseCommit {
		t.Fatal("retained bundle lost exact base parent", parents, err)
	}
	shallow, err := manager.gitOutput(ctx, independent, home, "rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(shallow) != "false" {
		t.Fatal("retained bundle still requires shallow/external objects", shallow, err)
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
