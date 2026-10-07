package sourcecheckpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const MaxGitBundle int64 = 512 << 20

var gitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type GitBundle struct {
	Path   string
	Digest string
	Size   int64
	Head   string
}

func validateGitBundleFacts(base, head, digest string, size int64) error {
	if !gitCommitPattern.MatchString(base) || !gitCommitPattern.MatchString(head) ||
		!canonical.ValidDigest(digest) || size <= 0 || size > MaxGitBundle {
		return fmt.Errorf("invalid checkpoint Git bundle facts")
	}
	return nil
}

func (g GitBundle) Validate(base string) error {
	if !filepath.IsAbs(g.Path) || filepath.Clean(g.Path) != g.Path ||
		validateGitBundleFacts(base, g.Head, g.Digest, g.Size) != nil {
		return fmt.Errorf("canonical private checkpoint Git bundle required")
	}
	if err := privateDir(filepath.Dir(g.Path)); err != nil {
		return err
	}
	return nil
}

func copyGitBundle(ctx context.Context, g GitBundle, target io.Writer) error {
	if !filepath.IsAbs(g.Path) || filepath.Clean(g.Path) != g.Path ||
		!gitCommitPattern.MatchString(g.Head) || !canonical.ValidDigest(g.Digest) ||
		g.Size <= 0 || g.Size > MaxGitBundle {
		return fmt.Errorf("canonical bounded checkpoint Git bundle required")
	}
	root, err := os.OpenRoot(filepath.Dir(g.Path))
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(g.Path)
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 || before.Size() != g.Size {
		return fmt.Errorf("private exact Git bundle required")
	}
	stat, ok := before.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return fmt.Errorf("Git bundle ownership/link identity invalid")
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !stable(before, opened) {
		return fmt.Errorf("Git bundle changed before read")
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(target, hash), io.LimitReader(reader{ctx, file}, MaxGitBundle+1))
	after, statErr := root.Lstat(name)
	if err != nil || statErr != nil || n != g.Size || !stable(before, after) ||
		"sha256:"+hex.EncodeToString(hash.Sum(nil)) != g.Digest {
		return fmt.Errorf("Git bundle changed or mismatched during read")
	}
	return nil
}
