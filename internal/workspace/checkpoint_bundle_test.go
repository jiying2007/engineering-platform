package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainPreservationBundleCoversBaseAndDirectFinalizeChild(t *testing.T) {
	for _, finalized := range []bool{false, true} {
		t.Run(map[bool]string{false: "base", true: "finalized-child"}[finalized], func(t *testing.T) {
			ctx := context.Background()
			repo, base := initRepository(t)
			root := t.TempDir()
			manager, err := New(filepath.Join(root, "managed"))
			if err != nil {
				t.Fatal(err)
			}
			w, err := manager.Create(ctx, Spec{ID: "preserve", Repository: repo, BaseCommit: base})
			if err != nil {
				t.Fatal(err)
			}
			artifacts := filepath.Join(root, "artifacts")
			if err := os.Mkdir(artifacts, 0o700); err != nil {
				t.Fatal(err)
			}
			head := w.BaseCommit
			if finalized {
				if err := os.WriteFile(filepath.Join(w.WorktreePath, "hello.txt"), []byte("changed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				result, err := manager.Finalize(ctx, w, artifacts, "result")
				if err != nil {
					t.Fatal(err)
				}
				head = result.Facts.ResultCommit
			}
			retained, err := manager.RetainPreservationBundle(ctx, w, artifacts, "checkpoint", head)
			if err != nil {
				t.Fatal(err)
			}
			if retained.Head != head || retained.Size <= 0 || !strings.HasPrefix(retained.Digest, "sha256:") {
				t.Fatal("invalid retained bundle", retained)
			}
			if _, err := manager.RetainPreservationBundle(ctx, w, artifacts, "checkpoint", head); err == nil {
				t.Fatal("existing bundle overwritten")
			}
			if _, err := manager.RetainPreservationBundle(ctx, w, artifacts, "other", strings.Repeat("f", 40)); err == nil {
				t.Fatal("changed checkpoint head accepted")
			}
			if _, err := os.Stat(retained.Path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
