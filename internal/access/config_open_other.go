//go:build !linux

package access

import "os"

// Preserve the preexisting platform opening semantics outside Linux.
// The Linux FIFO nonblocking guarantee is not established on this platform.
func openConfigurationFile(path string) (*os.File, error) {
	return os.Open(path)
}
