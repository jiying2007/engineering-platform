package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/recovery"
)

// Core Store requires request-bound Recovery read, Begin and Complete
// at compile time. An old unscoped adapter cannot be admitted by
// NewServer/NewAuthenticatedHandler, nor silently survive client cancellation.
func (s *Server) getRecoveryForRequest(ctx context.Context) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return s.store.GetRecoveryContext(ctx)
}

func (s *Server) beginRecoveryForRequest(ctx context.Context, epoch uint64) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return s.store.BeginRecoveryContext(ctx, epoch)
}

func (s *Server) completeRecoveryForRequest(ctx context.Context, epoch uint64, reconciled bool) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return s.store.CompleteRecoveryContext(ctx, epoch, reconciled)
}
