package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestIndependentWorkspaceDoesNotInheritExecutionConfiguration(t *testing.T) {
	repo, _ := initRepository(t)
	sentinel := filepath.Join(t.TempDir(), "executed")
	hook := "#!/bin/sh\nprintf unsafe > '" + sentinel + "'\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("hello.txt filter=unsafe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitattributes")
	runGit(t, repo, "commit", "-m", "attributes")
	commit := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	runGit(t, repo, "config", "filter.unsafe.smudge", "sh -c \"printf unsafe > '"+sentinel+"'\"")
	runGit(t, repo, "config", "filter.unsafe.clean", "sh -c \"printf unsafe > '"+sentinel+"'\"")
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "post-checkout"), []byte(hook), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := New(filepath.Join(t.TempDir(), "managed"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", "/nonexistent-parent-git-dir")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", filepath.Join(repo, ".git", "hooks"))
	t.Setenv("GIT_CONFIG_PARAMETERS", "malformed-parent-config")
	w, err := manager.Create(context.Background(), Spec{ID: "safe", Repository: repo, BaseCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(context.Background(), w)
	if pathExists(sentinel) {
		t.Fatal("source/ambient hooks or filter executed")
	}
	if pathExists(filepath.Join(repo, ".git", "worktrees")) {
		t.Fatal("source acquired shared worktree metadata")
	}
	if pathExists(filepath.Join(w.WorktreePath, ".git", "objects", "info", "alternates")) {
		t.Fatal("borrowed source objects")
	}
	data, err := os.ReadFile(filepath.Join(w.WorktreePath, "hello.txt"))
	if err != nil || string(data) != "base\n" {
		t.Fatalf("checkout changed: %q %v", data, err)
	}
	config, err := os.ReadFile(filepath.Join(w.WorktreePath, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config), "filter") || strings.Contains(string(config), repo) {
		t.Fatal("source configuration or locator copied")
	}
	// Even malicious configuration injected after preparation is never executed by
	// the cleanup/clean-check APIs. It is detected without running Git status.
	if err := os.WriteFile(filepath.Join(w.WorktreePath, ".git", "config"), []byte("[core]\nfsmonitor = "+sentinel+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if clean, err := manager.IsClean(context.Background(), w); clean || !errors.Is(err, ErrDirtyWorkspace) {
		t.Fatalf("tampered config accepted: %v %v", clean, err)
	}
	if pathExists(sentinel) {
		t.Fatal("clean check executed repository configuration")
	}
}
func TestIndependentWorkspaceSurvivesSourceRemovalAndChecksUntrackedBytes(t *testing.T) {
	repo, commit := initRepository(t)
	m, err := New(filepath.Join(t.TempDir(), "managed"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := m.Create(context.Background(), Spec{ID: "independent", Repository: repo, BaseCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if clean, err := m.IsClean(context.Background(), w); err != nil || !clean {
		t.Fatalf("still depends on source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.WorktreePath, "untracked"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if clean, err := m.IsClean(context.Background(), w); err != nil || clean {
		t.Fatalf("untracked bytes not detected: %v", err)
	}
	if err := m.Cleanup(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}
func TestIndependentWorkspaceExclusiveSlotsAndOwnership(t *testing.T) {
	repo, commit := initRepository(t)
	m, err := New(filepath.Join(t.TempDir(), "managed"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan Workspace, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := m.Create(context.Background(), Spec{ID: "one", Repository: repo, BaseCommit: commit})
			results <- w
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var winner Workspace
	count := 0
	for w := range results {
		if w.ID != "" {
			count++
			winner = w
		}
	}
	for err := range errs {
		if err != nil && !errors.Is(err, ErrWorkspaceExists) {
			t.Fatal(err)
		}
	}
	if count != 1 {
		t.Fatalf("slot owners=%d", count)
	}
	forged := winner
	forged.Ownership = strings.Repeat("0", 32)
	if err := m.Cleanup(context.Background(), forged); !errors.Is(err, ErrUnmanagedWorkspace) {
		t.Fatalf("forged cleanup accepted: %v", err)
	}
	if err := m.Cleanup(context.Background(), winner); err != nil {
		t.Fatal(err)
	}
}
func TestIndependentWorkspaceRejectsSymlinkRootAndCleansFailure(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	alias := filepath.Join(base, "alias")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(alias, "managed")); !errors.Is(err, ErrUnmanagedWorkspace) {
		t.Fatalf("symlink parent accepted: %v", err)
	}
	repo, _ := initRepository(t)
	m, err := New(filepath.Join(real, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), Spec{ID: "missing", Repository: repo, BaseCommit: strings.Repeat("f", 40)}); err == nil {
		t.Fatal("missing commit accepted")
	}
	if pathExists(filepath.Join(m.root, "missing")) {
		t.Fatal("partial slot survived failure")
	}
}
