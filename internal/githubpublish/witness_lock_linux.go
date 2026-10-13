//go:build linux

package githubpublish

import (
	"errors"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"io"
	"os"
	"strings"
	"syscall"
)

// The lock is on the existing slot inode, not a replaceable lock pathname.
// Separate os.Open calls are separate open file descriptions, including within
// this process. NB is essential: Recovery must not wait for or kill a Publisher.
func lockWitnessSlot(root *os.Root) (*os.File, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.Join(ErrDispatchEvidenceUnavailable, err, f.Close())
	}
	return f, nil
}

// Restrict negative/pre-effect inference to this boot and original slot inode
// on an explicitly local filesystem. Positive completed evidence keeps its own
// historical rules. This is NOT protection against an administrator reverting
// an entire live VM/disk or forging Publisher-owned records/certificates.
func witnessIncarnation(root *os.Root) (string, error) {
	f, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer f.Close()
	var st syscall.Stat_t
	var fs syscall.Statfs_t
	if syscall.Fstat(int(f.Fd()), &st) != nil || syscall.Fstatfs(int(f.Fd()), &fs) != nil {
		return "", ErrDispatchEvidenceUnavailable
	}
	switch uint32(fs.Type) {
	case 0xef53, 0x58465342, 0x9123683e, 0x794c7630, 0x01021994: // ext, XFS, Btrfs, overlay, tmpfs
	default:
		return "", ErrDispatchEvidenceUnavailable // no network/unknown-FS inference
	}
	boot, err := os.Open("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	defer boot.Close()
	raw, err := io.ReadAll(io.LimitReader(boot, 128))
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if len(id) != 36 {
		return "", ErrDispatchEvidenceUnavailable
	}
	for n, ch := range id {
		if n == 8 || n == 13 || n == 18 || n == 23 {
			if ch != '-' {
				return "", ErrDispatchEvidenceUnavailable
			}
		} else if !strings.ContainsRune("0123456789abcdef", ch) {
			return "", ErrDispatchEvidenceUnavailable
		}
	}
	return canonical.Digest(struct {
		Boot   string
		Device uint64
		Inode  uint64
	}{id, uint64(st.Dev), st.Ino})
}
