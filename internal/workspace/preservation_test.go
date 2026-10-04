package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreservationHeadNeverWeakensExecutionReopen(t *testing.T) {
	for _, mode := range []string{"base", "child", "grandchild", "symbolic-head", "foreign-parent", "changed-config", "grafts", "altered-shallow", "foreign-owner"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			repo, base := initRepository(t)
			m, err := New(filepath.Join(t.TempDir(), "owned"))
			if err != nil {
				t.Fatal(err)
			}
			w, err := m.Create(ctx, Spec{ID: "slot", Repository: repo, BaseCommit: base})
			if err != nil {
				t.Fatal(err)
			}
			write := func(name, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(w.WorktreePath, name), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			git := func(args ...string) string {
				t.Helper()
				v, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, args...)
				if err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(v)
			}
			commit := func(message string) {
				t.Helper()
				write("hello.txt", message)
				git("add", "-A")
				git("-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "-m", message)
			}
			switch mode {
			case "child":
				commit("one")
			case "grandchild":
				commit("one")
				commit("two")
			case "symbolic-head":
				write(".git/HEAD", "ref: refs/heads/main\n")
			case "foreign-parent":
				parent := git("-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit-tree", w.TreeCommit, "-m", "foreign root")
				write(".git/HEAD", parent+"\n")
			case "changed-config":
				write(".git/config", "[core]\nbare=true\n")
			case "grafts":
				if err := os.MkdirAll(filepath.Join(w.WorktreePath, ".git", "info"), 0700); err != nil {
					t.Fatal(err)
				}
				write(".git/info/grafts", base+"\n")
			case "altered-shallow":
				write(".git/shallow", strings.Repeat("0", 40)+"\n")
			case "foreign-owner":
				w.Ownership = strings.Repeat("0", 32)
			}
			got, err := m.PreservationHead(ctx, w)
			ok := mode == "base" || mode == "child"
			if (err == nil) != ok {
				t.Fatalf("preservation %s: %s %v", mode, got, err)
			}
			if mode == "child" {
				if _, err = m.Head(ctx, w); err == nil {
					t.Fatal("normal execution base fence weakened")
				}
			}
		})
	}
}
