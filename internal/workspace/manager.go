package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidWorkspaceID = errors.New("invalid workspace id")
	ErrInvalidCommit      = errors.New("base commit must be a full 40-character hexadecimal SHA")
	ErrWorkspaceExists    = errors.New("workspace path already exists")
	ErrUnmanagedWorkspace = errors.New("workspace paths are outside the managed root")
	ErrDirtyWorkspace     = errors.New("new workspace is not clean")
)

var (
	workspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	fullCommitPattern  = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
)

type Spec struct {
	ID         string `json:"workspace_id"`
	Repository string `json:"repository"`
	BaseCommit string `json:"base_commit"`
}

type Workspace struct {
	ID             string    `json:"workspace_id"`
	RepositoryRoot string    `json:"repository_root"`
	WorktreePath   string    `json:"worktree_path"`
	HomePath       string    `json:"home_path"`
	BaseCommit     string    `json:"base_commit"`
	CreatedAt      time.Time `json:"created_at"`
}

type Manager struct {
	root string
	git  string
	now  func() time.Time
}

func New(root string) (*Manager, error) {
	if root == "" {
		return nil, fmt.Errorf("workspace root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	return &Manager{
		root: filepath.Clean(absolute),
		git:  "git",
		now:  func() time.Time { return time.Now().UTC() },
	}, nil
}

func (m *Manager) Create(ctx context.Context, spec Spec) (Workspace, error) {
	if m == nil {
		return Workspace{}, fmt.Errorf("workspace manager is nil")
	}
	if !workspaceIDPattern.MatchString(spec.ID) || spec.ID == "." || spec.ID == ".." {
		return Workspace{}, ErrInvalidWorkspaceID
	}
	if !fullCommitPattern.MatchString(spec.BaseCommit) {
		return Workspace{}, ErrInvalidCommit
	}
	if spec.Repository == "" {
		return Workspace{}, fmt.Errorf("repository path is required")
	}

	repositoryRoot, err := m.repositoryRoot(ctx, spec.Repository)
	if err != nil {
		return Workspace{}, err
	}
	resolvedCommit, err := m.gitOutput(ctx, repositoryRoot, "rev-parse", "--verify", spec.BaseCommit+"^{commit}")
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve base commit: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(resolvedCommit), spec.BaseCommit) {
		return Workspace{}, fmt.Errorf("%w: requested=%s resolved=%s", ErrInvalidCommit, spec.BaseCommit, strings.TrimSpace(resolvedCommit))
	}
	baseCommit := strings.ToLower(strings.TrimSpace(resolvedCommit))

	worktreePath, homePath := m.paths(spec.ID)
	if pathExists(worktreePath) || pathExists(homePath) {
		return Workspace{}, ErrWorkspaceExists
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create worktree parent: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create home parent: %w", err)
	}
	if err := os.Mkdir(homePath, 0o700); err != nil {
		return Workspace{}, fmt.Errorf("create isolated HOME: %w", err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = m.removeWorktree(context.Background(), repositoryRoot, worktreePath)
			_ = os.RemoveAll(homePath)
		}
	}()

	if _, err := m.gitOutput(ctx, repositoryRoot, "worktree", "add", "--detach", worktreePath, baseCommit); err != nil {
		return Workspace{}, fmt.Errorf("create detached worktree: %w", err)
	}
	head, err := m.gitOutput(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return Workspace{}, fmt.Errorf("verify workspace HEAD: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(head), baseCommit) {
		return Workspace{}, fmt.Errorf("workspace HEAD mismatch: got=%s want=%s", strings.TrimSpace(head), baseCommit)
	}
	status, err := m.gitOutput(ctx, worktreePath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return Workspace{}, fmt.Errorf("verify workspace status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return Workspace{}, fmt.Errorf("%w: %s", ErrDirtyWorkspace, strings.TrimSpace(status))
	}

	workspace := Workspace{
		ID:             spec.ID,
		RepositoryRoot: repositoryRoot,
		WorktreePath:   worktreePath,
		HomePath:       homePath,
		BaseCommit:     baseCommit,
		CreatedAt:      m.now(),
	}
	cleanup = false
	return workspace, nil
}

func (m *Manager) Cleanup(ctx context.Context, workspace Workspace) error {
	if m == nil {
		return fmt.Errorf("workspace manager is nil")
	}
	if !workspaceIDPattern.MatchString(workspace.ID) {
		return ErrInvalidWorkspaceID
	}
	expectedWorktree, expectedHome := m.paths(workspace.ID)
	if filepath.Clean(workspace.WorktreePath) != expectedWorktree || filepath.Clean(workspace.HomePath) != expectedHome {
		return ErrUnmanagedWorkspace
	}
	repositoryRoot, err := m.repositoryRoot(ctx, workspace.RepositoryRoot)
	if err != nil {
		return err
	}
	if err := m.removeWorktree(ctx, repositoryRoot, expectedWorktree); err != nil {
		return err
	}
	if err := os.RemoveAll(expectedHome); err != nil {
		return fmt.Errorf("remove isolated HOME: %w", err)
	}
	_, _ = m.gitOutput(ctx, repositoryRoot, "worktree", "prune")
	return nil
}

func (m *Manager) Head(ctx context.Context, workspace Workspace) (string, error) {
	if err := m.validateManaged(workspace); err != nil {
		return "", err
	}
	head, err := m.gitOutput(ctx, workspace.WorktreePath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(head)), nil
}

func (m *Manager) IsClean(ctx context.Context, workspace Workspace) (bool, error) {
	if err := m.validateManaged(workspace); err != nil {
		return false, err
	}
	status, err := m.gitOutput(ctx, workspace.WorktreePath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status) == "", nil
}

func (m *Manager) validateManaged(workspace Workspace) error {
	if !workspaceIDPattern.MatchString(workspace.ID) {
		return ErrInvalidWorkspaceID
	}
	expectedWorktree, expectedHome := m.paths(workspace.ID)
	if filepath.Clean(workspace.WorktreePath) != expectedWorktree || filepath.Clean(workspace.HomePath) != expectedHome {
		return ErrUnmanagedWorkspace
	}
	return nil
}

func (m *Manager) paths(id string) (string, string) {
	return filepath.Join(m.root, "worktrees", id), filepath.Join(m.root, "homes", id)
}

func (m *Manager) repositoryRoot(ctx context.Context, path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository path: %w", err)
	}
	root, err := m.gitOutput(ctx, absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	root = strings.TrimSpace(root)
	resolved, err := filepath.EvalSymlinks(root)
	if err == nil {
		root = resolved
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("normalize repository root: %w", err)
	}
	return filepath.Clean(root), nil
}

func (m *Manager) removeWorktree(ctx context.Context, repositoryRoot, worktreePath string) error {
	if !pathExists(worktreePath) {
		return nil
	}
	if _, err := m.gitOutput(ctx, repositoryRoot, "worktree", "remove", "--force", worktreePath); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}
	return nil
}

func (m *Manager) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, m.git, append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
