// Package servicelifecycle binds HTTP request work to its owning service.
// It adds no background task, deadline, recovery state or retry authority.
package servicelifecycle

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// BindHTTP must run before Serve. These command-owned servers do not support
// replacement context hooks: a hook returning Background would silently break
// the service-to-request cancellation chain. Reject rather than overwrite hooks
// or introduce asynchronous merging of unrelated context trees.
func BindHTTP(ctx context.Context, server *http.Server) error {
	if ctx == nil || server == nil || ctx.Done() == nil {
		return fmt.Errorf("cancellable service Context and HTTP server required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if server.BaseContext != nil || server.ConnContext != nil {
		return fmt.Errorf("service owns HTTP base and connection Context hooks")
	}
	server.BaseContext = func(net.Listener) context.Context { return ctx }
	return nil
}

// Normalize only error trees whose EVERY leaf is the exact expected sentinel.
// Opaque Is implementations, empty unwraps, cycles and oversized error trees
// remain failures; do not discard useful error details to simplify exit status.
func OnlyStopError(err, expected error) bool {
	budget := 128
	var visit func(error) bool
	visit = func(current error) bool {
		budget--
		if current == nil || budget < 0 {
			return false
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			children := wrapped.Unwrap()
			seen := false
			for _, child := range children {
				if child == nil {
					continue
				}
				seen = true
				if !visit(child) {
					return false
				}
			}
			return seen
		case interface{ Unwrap() error }:
			return visit(wrapped.Unwrap())
		default:
			return current == expected
		}
	}
	return visit(err)
}
