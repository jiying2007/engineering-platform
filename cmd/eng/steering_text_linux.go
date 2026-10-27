//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// The final component must not be a symlink and must not wait for a FIFO
// writer if an operator-controlled regular file was replaced after Lstat.
// Descriptor and original path identity are checked by the shared reader.
func openSteeringFile(path string) (*os.File, error) {
	var fd int
	var err error
	for {
		fd, err = syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
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
		return nil, fmt.Errorf("invalid steering text descriptor")
	}
	return file, nil
}
