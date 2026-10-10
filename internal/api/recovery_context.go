package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/recovery"
)

// recoveryContextStore applies caller cancellation to the original Core
// Recovery epoch/audit/proof authority. This does not grant external replay.
type recoveryContextStore interface {
	GetRecoveryContext(context.Context) (recovery.Manager, error)
	BeginRecoveryContext(context.Context, uint64) (recovery.Manager, error)
	CompleteRecoveryContext(context.Context, uint64, bool) (recovery.Manager, error)
}

func (s *Server) getRecoveryForRequest(ctx context.Context) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	if typed, ok := s.store.(recoveryContextStore); ok {
		return typed.GetRecoveryContext(ctx)
	}
	return s.store.GetRecovery()
}

func (s *Server) beginRecoveryForRequest(ctx context.Context, epoch uint64) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	if typed, ok := s.store.(recoveryContextStore); ok {
		return typed.BeginRecoveryContext(ctx, epoch)
	}
	return s.store.BeginRecovery(epoch)
}

func (s *Server) completeRecoveryForRequest(ctx context.Context, epoch uint64, reconciled bool) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	if typed, ok := s.store.(recoveryContextStore); ok {
		return typed.CompleteRecoveryContext(ctx, epoch, reconciled)
	}
	return s.store.CompleteRecovery(epoch, reconciled)
}
