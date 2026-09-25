package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	ErrUnmanagedWorkspace = errors.New("workspace ownership or anchored path mismatch")
	ErrDirtyWorkspace     = errors.New("workspace differs from its prepared snapshot")
)
var workspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var fullCommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

const Recipe = "independent-git-snapshot-v1"

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
	TreeCommit     string    `json:"tree_commit"`
	SourceDigest   string    `json:"source_digest"`
	ConfigDigest   string    `json:"git_config_digest"`
	Ownership      string    `json:"ownership"`
	CreatedAt      time.Time `json:"created_at"`
}
type Manager struct {
	root     string
	git      string
	identity os.FileInfo
}

// New resolves the host's Git once. Services should use NewWithGit with an
// operator-selected absolute executable. No child inherits the parent environment.
func New(root string) (*Manager, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		return nil, err
	}
	return NewWithGit(root, git)
}
func NewWithGit(root, git string) (*Manager, error) {
	if root == "" || !filepath.IsAbs(git) {
		return nil, ErrUnmanagedWorkspace
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	if root == string(filepath.Separator) {
		return nil, ErrUnmanagedWorkspace
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil || parent != filepath.Dir(root) {
		return nil, ErrUnmanagedWorkspace
	}
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := canonicalDirectory(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(git)
	if err != nil || resolved != git {
		return nil, ErrUnmanagedWorkspace
	}
	executable, err := os.Stat(git)
	if err != nil || !executable.Mode().IsRegular() || executable.Mode().Perm()&0o022 != 0 || executable.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("trusted regular Git executable required")
	}
	return &Manager{root: root, git: git, identity: info}, nil
}
func canonicalDirectory(path string) (os.FileInfo, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return nil, ErrUnmanagedWorkspace
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return nil, ErrUnmanagedWorkspace
	}
	return info, nil
}
func (m *Manager) openRoot() (*os.Root, error) {
	if m == nil {
		return nil, ErrUnmanagedWorkspace
	}
	info, err := canonicalDirectory(m.root)
	if err != nil || !os.SameFile(info, m.identity) {
		return nil, ErrUnmanagedWorkspace
	}
	root, err := os.OpenRoot(m.root)
	if err != nil {
		return nil, err
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		_ = root.Close()
		return nil, ErrUnmanagedWorkspace
	}
	return root, nil
}
func validID(id string) bool { return workspaceIDPattern.MatchString(id) && id != "." && id != ".." }
func (m *Manager) paths(id string) (string, string) {
	return filepath.Join(m.root, id, "source"), filepath.Join(m.root, id, "home")
}

// Create fetches only the exact local commit into a fresh, independent object
// database. No linked worktree, hardlinks, alternates, source hooks/config, remote
// credentials, checkout filters or submodule initialization are carried over.
func (m *Manager) Create(ctx context.Context, spec Spec) (result Workspace, err error) {
	if !validID(spec.ID) {
		return Workspace{}, ErrInvalidWorkspaceID
	}
	if !fullCommitPattern.MatchString(spec.BaseCommit) {
		return Workspace{}, ErrInvalidCommit
	}
	if !filepath.IsAbs(spec.Repository) {
		return Workspace{}, ErrUnmanagedWorkspace
	}
	repository := filepath.Clean(spec.Repository)
	if _, err := canonicalDirectory(repository); err != nil {
		return Workspace{}, err
	}
	root, err := m.openRoot()
	if err != nil {
		return Workspace{}, err
	}
	defer root.Close()
	if overlaps(m.root, repository) {
		return Workspace{}, ErrUnmanagedWorkspace
	}
	if err := root.Mkdir(spec.ID, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Workspace{}, ErrWorkspaceExists
		}
		return Workspace{}, err
	}
	complete := false
	defer func() {
		if !complete {
			err = errors.Join(err, root.RemoveAll(spec.ID))
		}
	}()
	for _, dir := range []string{"source", "home", "git-home", "template"} {
		if err := root.Mkdir(spec.ID+"/"+dir, 0o700); err != nil {
			return Workspace{}, err
		}
	}
	source, home := m.paths(spec.ID)
	host := filepath.Join(m.root, spec.ID, "git-home")
	template := filepath.Join(m.root, spec.ID, "template")
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if _, err := m.gitOutput(ctx, source, host, "init", "--template="+template, "--object-format=sha1"); err != nil {
		return Workspace{}, err
	}
	if _, err := m.gitOutput(ctx, source, host, "fetch", "--no-tags", "--no-recurse-submodules", "--depth=1", "--no-write-fetch-head", "--", repository, strings.ToLower(spec.BaseCommit)); err != nil {
		return Workspace{}, err
	}
	head, err := m.gitOutput(ctx, source, host, "rev-parse", "--verify", strings.ToLower(spec.BaseCommit)+"^{commit}")
	if err != nil || strings.TrimSpace(head) != strings.ToLower(spec.BaseCommit) {
		return Workspace{}, ErrInvalidCommit
	}
	tree, err := m.gitOutput(ctx, source, host, "rev-parse", "--verify", strings.ToLower(spec.BaseCommit)+"^{tree}")
	if err != nil {
		return Workspace{}, err
	}
	entries, err := m.gitOutput(ctx, source, host, "ls-tree", "-r", strings.ToLower(spec.BaseCommit))
	if err != nil {
		return Workspace{}, err
	}
	for _, entry := range strings.Split(entries, "\n") {
		if strings.HasPrefix(entry, "160000 ") {
			return Workspace{}, fmt.Errorf("submodule content requires an explicit materialization policy")
		}
	}
	if _, err := m.gitOutput(ctx, source, host, "checkout", "--detach", strings.ToLower(spec.BaseCommit)); err != nil {
		return Workspace{}, err
	}
	// No subsequent status/diff command reads mutable checkout configuration.
	digest, err := snapshotDigest(ctx, source)
	if err != nil {
		return Workspace{}, err
	}
	config, err := boundedRootFile(root, spec.ID+"/source/.git/config", 64<<10)
	if err != nil {
		return Workspace{}, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Workspace{}, err
	}
	result = Workspace{ID: spec.ID, RepositoryRoot: repository, WorktreePath: source, HomePath: home, BaseCommit: strings.ToLower(spec.BaseCommit), TreeCommit: strings.TrimSpace(tree), SourceDigest: digest, ConfigDigest: rawDigest(config), Ownership: hex.EncodeToString(nonce[:]), CreatedAt: time.Now().UTC()}
	data, err := json.Marshal(result)
	if err != nil {
		return Workspace{}, err
	}
	if err := root.WriteFile(spec.ID+"/owner.json", data, 0o600); err != nil {
		return Workspace{}, err
	}
	complete = true
	return result, nil
}
func (m *Manager) validateManaged(w Workspace) (*os.Root, error) {
	if m == nil {
		return nil, ErrUnmanagedWorkspace
	}
	if !validID(w.ID) {
		return nil, ErrInvalidWorkspaceID
	}
	source, home := m.paths(w.ID)
	if w.WorktreePath != source || w.HomePath != home || len(w.Ownership) != 32 {
		return nil, ErrUnmanagedWorkspace
	}
	root, err := m.openRoot()
	if err != nil {
		return nil, err
	}
	fail := func() (*os.Root, error) { _ = root.Close(); return nil, ErrUnmanagedWorkspace }
	info, err := root.Lstat(w.ID)
	if err != nil || !info.IsDir() {
		return fail()
	}
	f, err := root.Open(w.ID + "/owner.json")
	if err != nil {
		return fail()
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 16<<10))
	_ = f.Close()
	var stored Workspace
	if readErr != nil || json.Unmarshal(data, &stored) != nil || stored != w {
		return fail()
	}
	return root, nil
}

// Cleanup only removes the exclusively created anchored slot after checking its
// random ownership record. It never executes Git in a caller-provided repository.
func (m *Manager) Cleanup(ctx context.Context, w Workspace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := m.validateManaged(w)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(w.ID)
}
func (m *Manager) Head(ctx context.Context, w Workspace) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := m.validateManaged(w)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(w.ID + "/source/.git")
	if err != nil || !info.IsDir() {
		return "", ErrUnmanagedWorkspace
	}
	data, err := boundedRootFile(root, w.ID+"/source/.git/HEAD", 64)
	if err != nil || string(data) != w.BaseCommit+"\n" {
		return "", ErrDirtyWorkspace
	}
	config, err := boundedRootFile(root, w.ID+"/source/.git/config", 64<<10)
	if err != nil || rawDigest(config) != w.ConfigDigest {
		return "", ErrDirtyWorkspace
	}
	for _, forbidden := range []string{"commondir", "objects/info/alternates", "info/attributes", "hooks"} {
		if _, err := root.Lstat(w.ID + "/source/.git/" + forbidden); !errors.Is(err, os.ErrNotExist) {
			return "", ErrDirtyWorkspace
		}
	}
	return w.BaseCommit, nil
}
func (m *Manager) IsClean(ctx context.Context, w Workspace) (bool, error) {
	if _, err := m.Head(ctx, w); err != nil {
		return false, err
	}
	digest, err := snapshotDigest(ctx, w.WorktreePath)
	return err == nil && digest == w.SourceDigest, err
}
func overlaps(a, b string) bool {
	inside := func(parent, path string) bool {
		rel, err := filepath.Rel(parent, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	}
	return inside(a, b) || inside(b, a)
}
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

type cappedOutput struct{ data []byte }

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 4<<20 {
		return 0, fmt.Errorf("Git output limit exceeded")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (m *Manager) gitOutput(ctx context.Context, dir, home string, args ...string) (string, error) {
	fixed := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.autocrlf=false", "-c", "core.attributesFile=/dev/null", "-c", "credential.helper=", "-c", "protocol.allow=never", "-c", "protocol.file.allow=always", "-c", "fetch.fsckObjects=true", "-c", "transfer.fsckObjects=true", "-c", "gc.auto=0", "-C", dir}
	cmd := exec.CommandContext(ctx, m.git, append(fixed, args...)...)
	cmd.Env = []string{"PATH=" + filepath.Dir(m.git) + ":/usr/bin:/bin", "HOME=" + home, "XDG_CONFIG_HOME=" + home, "LANG=C", "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_ATTR_NOSYSTEM=1", "GIT_ALLOW_PROTOCOL=file"}
	if err := configureGitProcess(cmd); err != nil {
		return "", err
	}
	cmd.WaitDelay = 2 * time.Second
	var out, diagnostic cappedOutput
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("workspace Git command failed: %w", err)
	}
	return string(out.data), nil
}

func boundedRootFile(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrUnmanagedWorkspace
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, ErrUnmanagedWorkspace
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(data) > int(limit) {
		return nil, ErrUnmanagedWorkspace
	}
	return data, err
}
func rawDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}
