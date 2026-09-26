package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const FinalizeRecipe = "independent-git-finalize-v1"

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

	bundlePath := filepath.Join(artifactRoot, artifactID+".bundle")
	if _, err := os.Lstat(bundlePath); err == nil || !os.IsNotExist(err) {
		return result, ErrWorkspaceExists
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "bundle", "create", bundlePath, "HEAD", "^"+w.BaseCommit); err != nil {
		return result, err
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "bundle", "verify", bundlePath); err != nil {
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
