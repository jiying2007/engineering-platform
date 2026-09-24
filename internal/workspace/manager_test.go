package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceCreateIsExactCleanAndIsolated(t *testing.T) {
	ctx := context.Background()
	repo, commit := initRepository(t)
	root := t.TempDir()

	manager, err := New(filepath.Join(root, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := manager.Create(ctx, Spec{
		ID:         "run-001",
		Repository: repo,
		BaseCommit: commit,
	})
	if err != nil {
		t.Fatal(err)
	}

	head, err := manager.Head(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if head != strings.ToLower(commit) {
		t.Fatalf("unexpected workspace HEAD %s want %s", head, commit)
	}
	clean, err := manager.IsClean(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Fatal("new workspace must be clean")
	}
	if !pathExists(workspace.HomePath) {
		t.Fatal("isolated HOME was not created")
	}
	if strings.HasPrefix(workspace.HomePath, workspace.WorktreePath+string(os.PathSeparator)) {
		t.Fatal("isolated HOME must live outside the source worktree")
	}

	if err := os.WriteFile(filepath.Join(workspace.WorktreePath, "hello.txt"), []byte("workspace change
"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainContent, err := os.ReadFile(filepath.Join(repo, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mainContent) != "base
" {
		t.Fatalf("workspace mutation leaked into source checkout: %q", mainContent)
	}
	clean, err = manager.IsClean(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if clean {
		t.Fatal("modified workspace should be dirty")
	}

	if err := manager.Cleanup(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	if pathExists(workspace.WorktreePath) || pathExists(workspace.HomePath) {
		t.Fatalf("workspace cleanup incomplete: worktree=%v home=%v", pathExists(workspace.WorktreePath), pathExists(workspace.HomePath))
	}
}

func TestWorkspaceRejectsUnsafeIdentityAndNonExactCommit(t *testing.T) {
	ctx := context.Background()
	repo, commit := initRepository(t)
	manager, err := New(filepath.Join(t.TempDir(), "managed"))
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"../escape", "/absolute", "a/b", "..", ""} {
		_, err := manager.Create(ctx, Spec{ID: id, Repository: repo, BaseCommit: commit})
		if !errors.Is(err, ErrInvalidWorkspaceID) {
			t.Fatalf("id %q: expected ErrInvalidWorkspaceID, got %v", id, err)
		}
	}
	_, err = manager.Create(ctx, Spec{ID: "run-short", Repository: repo, BaseCommit: commit[:12]})
	if !errors.Is(err, ErrInvalidCommit) {
		t.Fatalf("short SHA must fail closed, got %v", err)
	}
	_, err = manager.Create(ctx, Spec{ID: "run-nonhex", Repository: repo, BaseCommit: strings.Repeat("z", 40)})
	if !errors.Is(err, ErrInvalidCommit) {
		t.Fatalf("non-hex SHA must fail closed, got %v", err)
	}
}

func TestCleanupRejectsUnmanagedPaths(t *testing.T) {
	ctx := context.Background()
	repo, commit := initRepository(t)
	manager, err := New(filepath.Join(t.TempDir(), "managed"))
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := manager.Create(ctx, Spec{ID: "run-safe", Repository: repo, BaseCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Cleanup(context.Background(), workspace) }()

	tampered := workspace
	tampered.HomePath = t.TempDir()
	if err := manager.Cleanup(ctx, tampered); !errors.Is(err, ErrUnmanagedWorkspace) {
		t.Fatalf("expected unmanaged path rejection, got %v", err)
	}
}

func initRepository(t *testing.T) (string, string) {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Engineering Platform Test")
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("base
"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "hello.txt")
	runGit(t, repo, "commit", "-m", "base")
	commit := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	if len(commit) != 40 {
		t.Fatalf("expected full SHA, got %q", commit)
	}
	return repo, commit
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}
