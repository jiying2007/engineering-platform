package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const FinalizeRecipe = "independent-git-finalize-v2-self-contained"

var bundleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type ChangeFacts struct {
	Recipe             string `json:"recipe"`
	BaseCommit         string `json:"base_commit"`
	BaseTree           string `json:"base_tree"`
	BaseSourceDigest   string `json:"base_source_digest"`
	ResultCommit       string `json:"result_commit"`
	ResultTree         string `json:"result_tree"`
	ResultSourceDigest string `json:"result_source_digest"`
	BundleDigest       string `json:"bundle_digest"`
	BundleSize         int64  `json:"bundle_size"`
}

type Finalized struct {
	Facts      ChangeFacts `json:"facts"`
	BundlePath string      `json:"bundle_path"`
}

func (m *Manager) Finalize(ctx context.Context, w Workspace, artifactRoot, artifactID string) (Finalized, error) {
	var result Finalized
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !bundleIDPattern.MatchString(artifactID) || !filepath.IsAbs(artifactRoot) {
		return result, ErrUnmanagedWorkspace
	}
	root, err := m.validateManaged(w)
	if err != nil {
		return result, err
	}
	_ = root.Close()
	if overlaps(m.root, artifactRoot) || overlaps(w.RepositoryRoot, artifactRoot) {
		return result, ErrUnmanagedWorkspace
	}
	artifactInfo, err := canonicalDirectory(artifactRoot)
	if err != nil || artifactInfo.Mode().Perm()&0o077 != 0 {
		return result, ErrUnmanagedWorkspace
	}
	head, err := m.Head(ctx, w)
	if err != nil || head != w.BaseCommit {
		return result, ErrDirtyWorkspace
	}
	currentDigest, err := snapshotDigest(ctx, w.WorktreePath)
	if err != nil {
		return result, err
	}
	if currentDigest == w.SourceDigest {
		return result, fmt.Errorf("workspace has no source changes")
	}
	status, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(status) == "" {
		return result, fmt.Errorf("workspace bytes changed without Git-visible change")
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "add", "-A", "--", "."); err != nil {
		return result, err
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath,
		"-c", "user.name=Engineering Platform Worker",
		"-c", "user.email=engineering-platform@invalid",
		"-c", "commit.gpgSign=false",
		"commit", "--no-gpg-sign", "--no-verify", "-m", "engineering-platform: retained runtime result"); err != nil {
		return result, err
	}
	resultCommit, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, err
	}
	resultCommit = strings.TrimSpace(resultCommit)
	if !fullCommitPattern.MatchString(resultCommit) || resultCommit == w.BaseCommit {
		return result, ErrInvalidCommit
	}
	resultTree, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return result, err
	}
	resultTree = strings.TrimSpace(resultTree)
	if !fullCommitPattern.MatchString(resultTree) || resultTree == w.TreeCommit {
		return result, fmt.Errorf("result tree did not change")
	}
	entries, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "ls-tree", "-r", resultCommit)
	if err != nil {
		return result, err
	}
	for _, entry := range strings.Split(entries, "\n") {
		if strings.HasPrefix(entry, "160000 ") {
			return result, fmt.Errorf("result introduced submodule content")
		}
	}
	resultDigest, err := snapshotDigest(ctx, w.WorktreePath)
	if err != nil {
		return result, err
	}
	if resultDigest == w.SourceDigest {
		return result, fmt.Errorf("result source digest did not change")
	}
	clean, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || strings.TrimSpace(clean) != "" {
		return result, fmt.Errorf("result workspace is not clean after commit")
	}

	// Git cleanliness does not imply that the committed tree reproduces the
	// observed source. Ignored files, empty directories and attribute transforms
	// can disappear/change while status is clean. Read an independent checkout
	// before issuing a bundle; never force-add private files or drop source.
	if err := m.verifyResultReadback(ctx, w, artifactRoot, resultCommit, resultTree, resultDigest); err != nil {
		return result, err
	}

	bundlePath := filepath.Join(artifactRoot, artifactID+".bundle")
	if _, err := os.Lstat(bundlePath); err == nil || !os.IsNotExist(err) {
		return result, ErrWorkspaceExists
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "bundle", "create", bundlePath, "HEAD"); err != nil {
		return result, err
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "bundle", "verify", bundlePath); err != nil {
		_ = os.Remove(bundlePath)
		return result, err
	}
	if err := m.verifySelfContainedResultBundle(ctx, w, artifactRoot, bundlePath, resultCommit, resultTree, resultDigest); err != nil {
		_ = os.Remove(bundlePath)
		return result, err
	}
	info, err := os.Lstat(bundlePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Size() <= 0 || info.Size() > 1<<30 {
		_ = os.Remove(bundlePath)
		return result, ErrUnmanagedWorkspace
	}
	file, err := os.Open(bundlePath)
	if err != nil {
		_ = os.Remove(bundlePath)
		return result, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, io.LimitReader(file, (1<<30)+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n != info.Size() || n > 1<<30 {
		_ = os.Remove(bundlePath)
		return result, fmt.Errorf("bundle digest read failed")
	}
	result = Finalized{
		Facts: ChangeFacts{
			Recipe: FinalizeRecipe, BaseCommit: w.BaseCommit, BaseTree: w.TreeCommit,
			BaseSourceDigest: w.SourceDigest, ResultCommit: resultCommit, ResultTree: resultTree,
			ResultSourceDigest: resultDigest, BundleDigest: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
			BundleSize: n,
		},
		BundlePath: bundlePath,
	}
	return result, nil
}

// verifySelfContainedResultBundle imports only the retained bundle into a fresh
// empty object database. Successful fetch plus explicit base/result byte checks
// prove the bundle has no external Git-object prerequisite. It does not import
// source HOME/config/hooks/credentials and removes only its own private scratch.
func (m *Manager) verifySelfContainedResultBundle(ctx context.Context, source Workspace, artifactRoot, bundlePath, resultCommit, resultTree, resultDigest string) (err error) {
	dir, err := os.MkdirTemp(artifactRoot, ".bundle-readback-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	repo, home, template := filepath.Join(dir, "repo"), filepath.Join(dir, "home"), filepath.Join(dir, "template")
	for _, path := range []string{repo, home, template} {
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
	}
	if _, err := m.gitOutput(ctx, repo, home, "init", "--template="+template, "--object-format=sha1"); err != nil {
		return fmt.Errorf("initialize independent bundle readback: %w", err)
	}
	if _, err := m.gitOutput(ctx, repo, home, "fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", "--", bundlePath, "HEAD"); err != nil {
		return fmt.Errorf("self-contained result bundle import failed: %w", err)
	}
	fetched, err := m.gitOutput(ctx, repo, home, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil || strings.TrimSpace(fetched) != resultCommit {
		return fmt.Errorf("result bundle head identity mismatch")
	}
	parents, err := m.gitOutput(ctx, repo, home, "rev-list", "--parents", "-n", "1", resultCommit)
	if err != nil {
		return fmt.Errorf("result bundle parent identity unavailable")
	}
	fields := strings.Fields(parents)
	if len(fields) != 2 || fields[0] != resultCommit || fields[1] != source.BaseCommit {
		return fmt.Errorf("result bundle is not the exact direct child of retained base")
	}
	shallow, err := m.gitOutput(ctx, repo, home, "rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(shallow) != "false" {
		return fmt.Errorf("result bundle import retained an external shallow prerequisite")
	}
	for commit, tree := range map[string]string{source.BaseCommit: source.TreeCommit, resultCommit: resultTree} {
		actual, err := m.gitOutput(ctx, repo, home, "rev-parse", "--verify", commit+"^{tree}")
		if err != nil || strings.TrimSpace(actual) != tree {
			return fmt.Errorf("result bundle missing exact commit/tree identity")
		}
	}
	if _, err := m.gitOutput(ctx, repo, home, "checkout", "--detach", source.BaseCommit); err != nil {
		return fmt.Errorf("result bundle base checkout failed: %w", err)
	}
	baseDigest, err := snapshotDigest(ctx, repo)
	if err != nil || baseDigest != source.SourceDigest {
		return fmt.Errorf("result bundle base source bytes mismatch")
	}
	if _, err := m.gitOutput(ctx, repo, home, "checkout", "--detach", resultCommit); err != nil {
		return fmt.Errorf("result bundle result checkout failed: %w", err)
	}
	observedResult, err := snapshotDigest(ctx, repo)
	if err != nil || observedResult != resultDigest {
		return fmt.Errorf("result bundle result source bytes mismatch")
	}
	return nil
}

// verifyResultReadback uses the same trusted Git and snapshot recipe in a fresh
// private object database. It never executes source scripts or imports HOME,
// hooks/configuration/credentials, and only removes its own temporary directory.
func (m *Manager) verifyResultReadback(ctx context.Context, source Workspace, artifactRoot, commit, tree, digest string) (err error) {
	dir, err := os.MkdirTemp(artifactRoot, ".result-readback-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	verifier, err := NewWithGit(filepath.Join(dir, "managed"), m.git)
	if err != nil {
		return err
	}
	observed, err := verifier.Create(ctx, Spec{ID: "result", Repository: source.WorktreePath, BaseCommit: commit})
	if err != nil {
		return fmt.Errorf("independent result checkout failed: %w", err)
	}
	if observed.TreeCommit != tree || observed.SourceDigest != digest {
		return fmt.Errorf("committed result does not reproduce retained source bytes; resolve ignored files, empty directories or Git attribute transformations explicitly")
	}
	// Do not accept a successful readback if source or HEAD changed during it.
	after, err := snapshotDigest(ctx, source.WorktreePath)
	if err != nil || after != digest {
		return fmt.Errorf("source changed during result readback")
	}
	head, err := m.gitOutput(ctx, source.WorktreePath, source.HomePath, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != commit {
		return fmt.Errorf("result identity changed during readback")
	}
	return nil
}
