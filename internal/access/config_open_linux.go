//go:build linux

package access

import (
	"fmt"
	"os"
	"syscall"
)

// The path and its ancestors remain host-owned configuration authority.
// Keep the existing symlink-to-regular-file behavior; the opened descriptor
// is checked by ReadConfiguration. O_NONBLOCK prevents an accidentally or
// concurrently substituted FIFO from blocking before that type check.
func openConfigurationFile(path string) (*os.File, error) {
	var fd int
	var err error
	for {
		fd, err = syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("invalid configuration descriptor")
	}
	return file, nil
}
