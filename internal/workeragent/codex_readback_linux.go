//go:build linux

package workeragent

import (
	"os"
	"syscall"
)

// Nonblocking/no-follow also rejects a FIFO or alias raced after Lstat.
func openReadbackFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
