//go:build !linux

package main

import (
	"context"
	"io"
)

// Other platforms retain synchronous output. Do not detach writes or alter
// caller-owned descriptor flags to simulate an unsupported cancellation bound.
func prepareProductionMonitorOutput(_ context.Context, writer io.Writer) (io.Writer, func(), error) {
	return writer, func() {}, nil
}
