//go:build linux

package artifactset

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func deviceOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("Unix filesystem identity required")
	}
	return stat.Dev
}

func independentMirrorDir(t *testing.T, sourceRoot string) string {
	t.Helper()
	const shm = "/dev/shm"
	if _, err := os.Stat(shm); err != nil || deviceOf(t, shm) == deviceOf(t, sourceRoot) {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Fatal("GitHub Linux CI requires an independent /dev/shm filesystem")
		}
		t.Skip("independent /dev/shm filesystem unavailable")
	}
	root, err := os.MkdirTemp(shm, "engineering-platform-artifact-mirror-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func TestMirrorCopiesVerifiedArchiveToIndependentFilesystemOnly(t *testing.T) {
	p, plan, sum := fixture(t)
	sourceStore := t.TempDir()
	if err := os.Chmod(sourceStore, 0700); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(sourceStore, "source.tar")
	packed, err := Pack(context.Background(), plan, sum, archive)
	if err != nil {
		t.Fatal(err)
	}

	same := filepath.Join(sourceStore, "same-device.tar")
	if _, err := Mirror(context.Background(), archive, packed.ArchiveDigest, p.Subject.RunID, same); err == nil {
		t.Fatal("same-filesystem mirror accepted")
	}
	if _, err := os.Lstat(same); !os.IsNotExist(err) {
		t.Fatal("rejected same-filesystem mirror published output")
	}

	targetRoot := independentMirrorDir(t, sourceStore)
	target := filepath.Join(targetRoot, "mirror.tar")
	mirrored, err := Mirror(context.Background(), archive, packed.ArchiveDigest, p.Subject.RunID, target)
	if err != nil {
		t.Fatal(err)
	}
	if mirrored.Status != "ARTIFACT_SET_INDEPENDENT_FILESYSTEM_MIRROR_BYTES_VERIFIED" ||
		mirrored.ArchiveDigest != packed.ArchiveDigest || mirrored.ArchiveSize != packed.ArchiveSize ||
		mirrored.ManifestDigest != packed.ManifestDigest || mirrored.Subject != packed.Subject ||
		!mirrored.IndependentFilesystem || mirrored.SecondSiteQualified ||
		mirrored.ExecutionAuthorized || mirrored.ProducerSemanticsVerified || mirrored.ProductionQualified {
		t.Fatal("mirror identity/authority drift", mirrored)
	}
	sourceBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	mirrorBytes, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(sourceBytes, mirrorBytes) {
		t.Fatal("mirror bytes differ", err)
	}
	if deviceOf(t, archive) == deviceOf(t, target) {
		t.Fatal("mirror did not cross filesystems")
	}
	if _, err := Mirror(context.Background(), archive, packed.ArchiveDigest, p.Subject.RunID, target); err == nil {
		t.Fatal("existing mirror overwritten")
	}
	after, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(after, mirrorBytes) {
		t.Fatal("failed overwrite attempt changed mirror")
	}

	for name, identity := range map[string][2]string{
		"wrong-digest": {"sha256:" + strings.Repeat("0", 64), p.Subject.RunID},
		"wrong-run":    {packed.ArchiveDigest, "other-run"},
	} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(targetRoot, name+".tar")
			if _, err := Mirror(context.Background(), archive, identity[0], identity[1], out); err == nil {
				t.Fatal("invalid mirror anchor accepted")
			}
			if _, err := os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("failed mirror published output")
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := filepath.Join(targetRoot, "cancelled.tar")
	if _, err := Mirror(ctx, archive, packed.ArchiveDigest, p.Subject.RunID, cancelled); err == nil {
		t.Fatal("cancelled mirror succeeded")
	}
	if _, err := os.Lstat(cancelled); !os.IsNotExist(err) {
		t.Fatal("cancelled mirror published output")
	}

	// Original archive remains independently valid after all mirror operations.
	if _, err := Verify(context.Background(), archive, canonical.BytesDigest(sourceBytes), p.Subject.RunID); err != nil {
		t.Fatal("mirror mutated source archive", err)
	}
}
