//go:build linux

package githubpublish

import (
	"os"
	"strings"
	"syscall"
)

func witnessPrivateDirectory(i os.FileInfo) bool {
	if i == nil || !i.IsDir() || i.Mode().Perm() != 0700 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid())
}
func witnessPrivateFile(i os.FileInfo) bool {
	if i == nil || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Size() <= 0 || i.Size() > 64<<10 {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid()) && s.Nlink == 1
}
func openWitnessRead(r *os.Root, name string) (*os.File, error) {
	// Root.OpenFile deliberately resolves in-root links. Use a held directory
	// descriptor and a SINGLE leaf component for strict no-follow semantics.
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return nil, ErrDispatchEvidenceUnavailable
	}
	dir, err := r.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	fd, err := syscall.Openat(int(dir.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
