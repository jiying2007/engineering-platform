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
	"strings"
	"syscall"
)

const MaxPreservationBundle int64 = 512 << 20

type PreservationBundle struct {
	Path   string
	Digest string
	Size   int64
	Head   string
}

// RetainPreservationBundle records the exact stopped Git graph needed to
// reconstruct the frozen base even if source capture occurs before Finalize.
// expectedHead comes from the already validated checkpoint boundary and prevents
// a new commit appearing between source authorization and bundle creation.
func (m *Manager) RetainPreservationBundle(ctx context.Context, w Workspace, artifactRoot, artifactID, expectedHead string) (result PreservationBundle, err error) {
	if !bundleIDPattern.MatchString(artifactID) || !filepath.IsAbs(artifactRoot) || !fullCommitPattern.MatchString(expectedHead) {
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
	head, err := m.PreservationHead(ctx, w)
	if err != nil || head != expectedHead {
		return result, ErrDirtyWorkspace
	}
	bundlePath := filepath.Join(artifactRoot, artifactID+".checkpoint-git.bundle")
	if _, err := os.Lstat(bundlePath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return result, ErrWorkspaceExists
	}
	complete := false
	defer func() {
		if !complete {
			err = errors.Join(err, os.Remove(bundlePath))
		}
	}()
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "bundle", "create", bundlePath, "HEAD"); err != nil {
		return result, err
	}
	if err := os.Chmod(bundlePath, 0o600); err != nil {
		return result, err
	}
	if _, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "bundle", "verify", bundlePath); err != nil {
		return result, err
	}
	if err := m.verifyPreservationBundle(ctx, w, artifactRoot, bundlePath, expectedHead); err != nil {
		return result, err
	}
	info, err := os.Lstat(bundlePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > MaxPreservationBundle {
		return result, ErrUnmanagedWorkspace
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return result, ErrUnmanagedWorkspace
	}
	file, err := os.Open(bundlePath)
	if err != nil {
		return result, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, io.LimitReader(file, MaxPreservationBundle+1))
	opened, statErr := file.Stat()
	closeErr := file.Close()
	after, lstatErr := os.Lstat(bundlePath)
	if copyErr != nil || statErr != nil || closeErr != nil || lstatErr != nil ||
		n != info.Size() || n > MaxPreservationBundle || !os.SameFile(info, opened) || !os.SameFile(info, after) ||
		info.Mode() != after.Mode() || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return result, fmt.Errorf("preservation bundle changed during digest read")
	}
	result = PreservationBundle{Path: bundlePath, Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), Size: n, Head: expectedHead}
	complete = true
	return result, nil
}

func (m *Manager) verifyPreservationBundle(ctx context.Context, source Workspace, artifactRoot, bundlePath, expectedHead string) (err error) {
	dir, err := os.MkdirTemp(artifactRoot, ".checkpoint-bundle-readback-")
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
		return err
	}
	if _, err := m.gitOutput(ctx, repo, home, "fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", "--", bundlePath, "HEAD:refs/heads/checkpoint"); err != nil {
		return fmt.Errorf("checkpoint bundle import failed: %w", err)
	}
	fetched, err := m.gitOutput(ctx, repo, home, "rev-parse", "--verify", "refs/heads/checkpoint^{commit}")
	if err != nil || strings.TrimSpace(fetched) != expectedHead {
		return fmt.Errorf("checkpoint bundle head mismatch")
	}
	shallow, err := m.gitOutput(ctx, repo, home, "rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(shallow) != "false" {
		return fmt.Errorf("checkpoint bundle depends on external shallow objects")
	}
	baseTree, err := m.gitOutput(ctx, repo, home, "rev-parse", "--verify", source.BaseCommit+"^{tree}")
	if err != nil || strings.TrimSpace(baseTree) != source.TreeCommit {
		return fmt.Errorf("checkpoint bundle missing frozen base tree")
	}
	if expectedHead != source.BaseCommit {
		parents, err := m.gitOutput(ctx, repo, home, "rev-list", "--parents", "-n", "1", expectedHead)
		fields := strings.Fields(parents)
		if err != nil || len(fields) != 2 || fields[1] != source.BaseCommit {
			return fmt.Errorf("checkpoint bundle head is not one direct Finalize child")
		}
	}
	if _, err := m.gitOutput(ctx, repo, home, "checkout", "--detach", source.BaseCommit); err != nil {
		return err
	}
	digest, err := snapshotDigest(ctx, repo)
	if err != nil || digest != source.SourceDigest {
		return fmt.Errorf("checkpoint bundle base source mismatch")
	}
	return nil
}
