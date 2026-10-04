package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizeRejectsSourceMissingFromResultCommit(t *testing.T) {
	for _, mode := range []string{"ignored-file", "empty-directory", "attribute-normalization"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			repo, base := initRepository(t)
			root := t.TempDir()
			manager, err := New(filepath.Join(root, "managed"))
			if err != nil {
				t.Fatal(err)
			}
			w, err := manager.Create(ctx, Spec{ID: "lost-source", Repository: repo, BaseCommit: base})
			if err != nil {
				t.Fatal(err)
			}
			write := func(name string, raw []byte) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(w.WorktreePath, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("hello.txt", []byte("changed source\n"))
			switch mode {
			case "ignored-file":
				write(".gitignore", []byte("private.bin\n"))
				write("private.bin", []byte{0, 1, 2, 255})
			case "empty-directory":
				if err := os.Mkdir(filepath.Join(w.WorktreePath, "empty"), 0700); err != nil {
					t.Fatal(err)
				}
			case "attribute-normalization":
				write(".gitattributes", []byte("hello.txt text eol=lf\n"))
				write("hello.txt", []byte("changed source\r\n"))
			}
			artifacts := filepath.Join(root, "artifacts")
			if err := os.Mkdir(artifacts, 0700); err != nil {
				t.Fatal(err)
			}
			result, err := manager.Finalize(ctx, w, artifacts, "result")
			if err == nil {
				t.Fatalf("undeliverable source accepted: result_source_digest=%s commit=%s", result.Facts.ResultSourceDigest, result.Facts.ResultCommit)
			}
			if result.BundlePath != "" {
				t.Fatal("failed finalization returned a deliverable bundle")
			}
			if _, err := os.Stat(filepath.Join(artifacts, "result.bundle")); !os.IsNotExist(err) {
				t.Fatal("bundle created before source readback")
			}
			if _, err := os.Stat(filepath.Join(w.WorktreePath, "hello.txt")); err != nil {
				t.Fatal("failure destroyed retained work")
			}
			if mode == "ignored-file" {
				if _, err := os.Stat(filepath.Join(w.WorktreePath, "private.bin")); err != nil {
					t.Fatal("ignored private source was deleted")
				}
			}
			entries, err := os.ReadDir(artifacts)
			if err != nil || len(entries) != 0 {
				t.Fatal("temporary verification state leaked", entries, err)
			}
		})
	}
}
func TestFinalizeReadbackPreservesRepresentableFullSource(t *testing.T) {
	ctx := context.Background()
	repo, base := initRepository(t)
	root := t.TempDir()
	manager, err := New(filepath.Join(root, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := manager.Create(ctx, Spec{ID: "complete-source", Repository: repo, BaseCommit: base})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"hello.txt": []byte("完整差异\n"), "empty.txt": {}, ".gitignore": []byte("tracked.bin\n"), "tracked.bin": {0, 1, 2, 255}} {
		if err := os.WriteFile(filepath.Join(w.WorktreePath, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("hello.txt", filepath.Join(w.WorktreePath, "relative-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(w.WorktreePath, "hello.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	// An explicitly staged file is representable even if its path matches ignore
	// rules. The implementation must never itself force-add ignored private files.
	if _, err := manager.gitOutput(ctx, w.WorktreePath, w.HomePath, "add", "-f", "--", "tracked.bin"); err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.Mkdir(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Finalize(ctx, w, artifacts, "result")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := New(filepath.Join(root, "independent"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := verifier.Create(ctx, Spec{ID: "readback", Repository: w.WorktreePath, BaseCommit: result.Facts.ResultCommit})
	if err != nil {
		t.Fatal(err)
	}
	if actual.SourceDigest != result.Facts.ResultSourceDigest || actual.TreeCommit != result.Facts.ResultTree {
		t.Fatal("receipt does not describe delivered source")
	}
	entries, err := os.ReadDir(artifacts)
	if err != nil || len(entries) != 1 || entries[0].Name() != "result.bundle" {
		t.Fatal("unexpected retained members", entries, err)
	}
}
