//go:build linux

package production

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// os.Root.Open follows in-root symlinks. A flat Openat anchored to its directory
// descriptor preserves O_NOFOLLOW, while O_NONBLOCK prevents FIFO substitution
// from blocking before the regular-file check can run.
func openOperationalSLIFile(root *os.Root, name string) (*os.File, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return nil, fmt.Errorf("flat operational SLI source name required")
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	var fd int
	for {
		fd, err = syscall.Openat(int(dir.Fd()), name, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("invalid operational SLI descriptor")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("regular operational SLI source required")
	}
	return f, nil
}
