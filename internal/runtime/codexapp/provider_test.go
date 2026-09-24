package codexapp

import (
	"context"
	"path/filepath"
	"testing"

	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

func TestProviderBuildsAppServerCommandInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	home := filepath.Join(root, "home")

	provider := NewProvider("/opt/codex")
	cmd, err := provider.Command(context.Background(), runtimeprovider.LaunchSpec{
		Dir:  worktree,
		Env:  []string{"HOME=" + home, "CODEX_ACCESS_TOKEN=secret-reference-for-test"},
		Args: []string{"--some-future-compatible-flag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Name() != "codex-app-server" {
		t.Fatalf("unexpected provider name %q", provider.Name())
	}
	if got := cmd.Args; len(got) < 2 || got[0] != "/opt/codex" || got[1] != "app-server" {
		t.Fatalf("unexpected command args %#v", got)
	}
	if cmd.Dir != worktree {
		t.Fatalf("unexpected workspace dir %q", cmd.Dir)
	}
}

func TestProviderRequiresIsolatedHomeOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	provider := NewProvider("codex")

	if _, err := provider.Command(context.Background(), runtimeprovider.LaunchSpec{Dir: worktree}); err == nil {
		t.Fatal("expected missing HOME to fail")
	}
	if _, err := provider.Command(context.Background(), runtimeprovider.LaunchSpec{
		Dir: worktree,
		Env: []string{"HOME=" + filepath.Join(worktree, ".home")},
	}); err == nil {
		t.Fatal("expected HOME inside source worktree to fail")
	}
}
