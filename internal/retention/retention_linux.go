//go:build linux

// Package retention copies externally anchored private artifact-set archives
// into a second owner-private root. It never scans for work, deletes originals,
// contacts Core, runs retained bytes or grants execution authority.
package retention

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const (
	MaxItems      = 128
	MaxSweepBytes = int64(16 << 30)
)

var archiveLeaf = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type Item struct {
	Archive       string `json:"archive"`
	RunID         string `json:"run_id"`
	ArchiveDigest string `json:"archive_digest"`
}

type Config struct {
	Version       int    `json:"version"`
	PrimaryRoot   string `json:"primary_root"`
	ReplicaRoot   string `json:"replica_root"`
	MaxTotalBytes int64  `json:"max_total_bytes"`
	Items         []Item `json:"items"`
}

type Report struct {
	Version             int    `json:"version"`
	Status              string `json:"status"`
	ConfigDigest        string `json:"config_digest"`
	ItemCount           int    `json:"item_count"`
	CopiedCount         int    `json:"copied_count"`
	ExistingCount       int    `json:"existing_count"`
	VerifiedBytes       int64  `json:"verified_bytes"`
	DeletionPerformed   bool   `json:"deletion_performed"`
	ExecutionAuthorized bool   `json:"execution_authorized"`
	ProductionQualified bool   `json:"production_qualified"`
	SecondSiteQualified bool   `json:"second_site_qualified"`
}

type verified struct {
	item   Item
	report artifactset.Report
	exists bool
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func validateConfig(c Config) error {
	if c.Version != 1 || c.MaxTotalBytes < 1 || c.MaxTotalBytes > MaxSweepBytes ||
		len(c.Items) == 0 || len(c.Items) > MaxItems {
		return fmt.Errorf("retention config version/items/byte budget invalid")
	}
	if c.PrimaryRoot == c.ReplicaRoot || !canonicalDirectory(c.PrimaryRoot) || !canonicalDirectory(c.ReplicaRoot) {
		return fmt.Errorf("two distinct canonical private roots required")
	}
	for _, pair := range [][2]string{{c.PrimaryRoot, c.ReplicaRoot}, {c.ReplicaRoot, c.PrimaryRoot}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return fmt.Errorf("primary and replica roots must not overlap")
		}
	}
	last := ""
	for _, item := range c.Items {
		if !archiveLeaf.MatchString(item.Archive) || item.Archive <= last ||
			len(item.RunID) == 0 || len(item.RunID) > 256 || !canonical.ValidDigest(item.ArchiveDigest) {
			return fmt.Errorf("invalid, unordered or duplicate retention item")
		}
		for _, ch := range item.RunID {
			if ch < 0x21 || ch > 0x7e {
				return fmt.Errorf("invalid retention run identity")
			}
		}
		last = item.Archive
	}
	return nil
}

func canonicalDirectory(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

func privateDirectory(path string) (os.FileInfo, error) {
	if !canonicalDirectory(path) {
		return nil, fmt.Errorf("canonical private directory required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	info, statErr := os.Lstat(path)
	if err != nil || statErr != nil || resolved != path || !info.IsDir() || info.Mode().Perm()&0077 != 0 || info.Mode()&os.ModeSetgid != 0 {
		return nil, fmt.Errorf("unaliased owner-private directory required")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("private directory must be owned by retention identity")
	}
	return info, nil
}

func directoryUnchanged(path string, before os.FileInfo) error {
	now, err := privateDirectory(path)
	if err != nil || !os.SameFile(before, now) || now.Mode() != before.Mode() {
		return fmt.Errorf("private directory changed during retention")
	}
	return nil
}

func stableFile(a, b os.FileInfo) bool {
	if a == nil || b == nil || !os.SameFile(a, b) || a.Mode() != b.Mode() ||
		a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Uid == y.Uid && x.Nlink == y.Nlink && x.Ctim == y.Ctim
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func sameArchive(a, b artifactset.Report) bool {
	a.Status, b.Status = "", ""
	return a == b
}

func copyArchive(ctx context.Context, primary, replica string, v verified) error {
	source := filepath.Join(primary, v.item.Archive)
	before, err := os.Lstat(source)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 || before.Size() != v.report.ArchiveSize {
		return fmt.Errorf("retention source changed before copy")
	}
	s, ok := before.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 {
		return fmt.Errorf("retention source ownership/link identity changed")
	}
	src, err := os.OpenFile(source, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer src.Close()
	opened, err := src.Stat()
	if err != nil || !stableFile(before, opened) {
		return fmt.Errorf("retention source changed before read")
	}

	root, err := os.OpenRoot(replica)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat(v.item.Archive); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replica destination appeared before publication")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	tempName := ".retention-" + hex.EncodeToString(nonce)
	temp, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = temp.Close()
		_ = root.Remove(tempName)
	}()
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(contextReader{ctx, src}, v.report.ArchiveSize+1))
	if copyErr != nil || n != v.report.ArchiveSize || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != v.item.ArchiveDigest {
		return fmt.Errorf("retention copy byte identity mismatch")
	}
	afterOpen, err := src.Stat()
	afterPath, pathErr := os.Lstat(source)
	if err != nil || pathErr != nil || !stableFile(before, afterOpen) || !stableFile(before, afterPath) {
		return fmt.Errorf("retention source changed during copy")
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	tempPath := filepath.Join(replica, tempName)
	check, err := artifactset.Verify(ctx, tempPath, v.item.ArchiveDigest, v.item.RunID)
	if err != nil || !sameArchive(check, v.report) {
		return fmt.Errorf("replica temporary byte readback failed")
	}
	if err := root.Link(tempName, v.item.Archive); err != nil {
		return err
	}
	if err := root.Remove(tempName); err != nil {
		return err
	}
	if err := syncDirectory(replica); err != nil {
		return err
	}
	final, err := artifactset.Verify(ctx, filepath.Join(replica, v.item.Archive), v.item.ArchiveDigest, v.item.RunID)
	if err != nil || !sameArchive(final, v.report) {
		return fmt.Errorf("published replica byte readback failed")
	}
	again, err := artifactset.Verify(ctx, source, v.item.ArchiveDigest, v.item.RunID)
	if err != nil || !sameArchive(again, v.report) {
		return fmt.Errorf("primary changed after replica publication")
	}
	return nil
}

func Replicate(ctx context.Context, configPath, expectedConfigDigest string) (Report, error) {
	var zero Report
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if !canonical.ValidDigest(expectedConfigDigest) {
		return zero, fmt.Errorf("externally pinned retention config digest required")
	}
	raw, err := access.ReadConfiguration(configPath, false)
	if err != nil {
		return zero, err
	}
	if canonical.BytesDigest(raw) != expectedConfigDigest {
		return zero, fmt.Errorf("retention config digest mismatch")
	}
	var config Config
	if err := strictjson.Decode(raw, &config); err != nil {
		return zero, err
	}
	if err := validateConfig(config); err != nil {
		return zero, err
	}
	primaryIdentity, err := privateDirectory(config.PrimaryRoot)
	if err != nil {
		return zero, fmt.Errorf("primary root: %w", err)
	}
	replicaIdentity, err := privateDirectory(config.ReplicaRoot)
	if err != nil {
		return zero, fmt.Errorf("replica root: %w", err)
	}
	lock, err := os.Open(config.ReplicaRoot)
	if err != nil {
		return zero, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return zero, fmt.Errorf("retention sweep already active")
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	verifiedItems := make([]verified, 0, len(config.Items))
	var total int64
	existing := 0
	for _, item := range config.Items {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		primary, err := artifactset.Verify(ctx, filepath.Join(config.PrimaryRoot, item.Archive), item.ArchiveDigest, item.RunID)
		if err != nil {
			return zero, fmt.Errorf("primary archive %s: %w", item.Archive, err)
		}
		total += primary.ArchiveSize
		if total < 0 || total > config.MaxTotalBytes {
			return zero, fmt.Errorf("retention sweep exceeds explicit byte budget")
		}
		current := verified{item: item, report: primary}
		replicaPath := filepath.Join(config.ReplicaRoot, item.Archive)
		if _, err := os.Lstat(replicaPath); err == nil {
			replica, err := artifactset.Verify(ctx, replicaPath, item.ArchiveDigest, item.RunID)
			if err != nil || !sameArchive(replica, primary) {
				return zero, fmt.Errorf("existing replica differs from anchored primary")
			}
			current.exists = true
			existing++
		} else if !errors.Is(err, os.ErrNotExist) {
			return zero, fmt.Errorf("replica destination cannot be inspected")
		}
		verifiedItems = append(verifiedItems, current)
	}
	if err := directoryUnchanged(config.PrimaryRoot, primaryIdentity); err != nil {
		return zero, err
	}
	if err := directoryUnchanged(config.ReplicaRoot, replicaIdentity); err != nil {
		return zero, err
	}

	copied := 0
	for _, item := range verifiedItems {
		if item.exists {
			continue
		}
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if err := copyArchive(ctx, config.PrimaryRoot, config.ReplicaRoot, item); err != nil {
			return zero, err
		}
		copied++
	}
	if err := directoryUnchanged(config.PrimaryRoot, primaryIdentity); err != nil {
		return zero, err
	}
	if err := directoryUnchanged(config.ReplicaRoot, replicaIdentity); err != nil {
		return zero, err
	}
	return Report{
		Version: 1, Status: "RETENTION_REPLICA_BYTES_VERIFIED",
		ConfigDigest: expectedConfigDigest, ItemCount: len(config.Items),
		CopiedCount: copied, ExistingCount: existing, VerifiedBytes: total,
	}, nil
}
