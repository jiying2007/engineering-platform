package codexapp

import (
	"bytes"
	"sync"
)

// os/exec writes stderr on a separate goroutine. Startup/error diagnostics can
// be read before Wait, so a bare bytes.Buffer would race with that writer.
type diagnosticBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *diagnosticBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }
