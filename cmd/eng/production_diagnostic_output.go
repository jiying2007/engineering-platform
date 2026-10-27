package main

import (
	"context"
	"fmt"
	"io"
)

// writeProductionDiagnostic uses the existing isolated, deadline-aware
// Linux pipe writer for bounded read-only operational reports. It never
// changes the caller's original stdout descriptor flags. Partial output
// does not grant successful observation: the write AND the inherited
// invocation Context must complete. Devices/regular files retain their
// existing synchronous I/O semantics.
func writeProductionDiagnostic(ctx context.Context, dst io.Writer, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	output, closeOutput, err := prepareProductionMonitorOutput(ctx, dst)
	if err != nil {
		return err
	}
	defer closeOutput()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeJSON(output, value); err != nil {
		return fmt.Errorf("production diagnostic output failed: %w", err)
	}
	return ctx.Err()
}
