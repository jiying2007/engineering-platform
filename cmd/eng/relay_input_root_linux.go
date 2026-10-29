//go:build linux

package main

import (
	"os"
	"syscall"
)

// Open relative to a pinned os.Root, without following a final symlink or
// waiting on a FIFO substituted for a checked regular file.
func openRelayRootInput(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
}
