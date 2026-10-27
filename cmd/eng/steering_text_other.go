//go:build !linux

package main

import (
	"fmt"
	"os"
)

// There is not yet a separately qualified nonblocking, no-follow file opener
// on this platform. Do not recreate the Lstat/Open FIFO race as a fallback.
func openSteeringFile(string) (*os.File, error) {
	return nil, fmt.Errorf("safe steering text file opening is unsupported on this platform")
}
