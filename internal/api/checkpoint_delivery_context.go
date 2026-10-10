package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/session"
)

// checkpointDeliveryContextStore exposes caller cancellation on the existing
// Core PostgreSQL ledger. The in-memory Store continues to support the same
// historical API without inventing a second checkpoint or delivery authority.
type checkpointDeliveryContextStore interface {
	CreateCheckpointContext(context.Context, session.Checkpoint) (string, error)
	GetCheckpointContext(context.Context, string) (session.Checkpoint, string, error)
	CreateDeliveryContext(context.Context, core.DeliveryReceipt) error
	GetDeliveryContext(context.Context, string) (core.DeliveryReceipt, error)
}

func (s *Server) createCheckpointForRequest(ctx context.Context, item session.Checkpoint) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if typed, ok := s.store.(checkpointDeliveryContextStore); ok {
		return typed.CreateCheckpointContext(ctx, item)
	}
	return s.store.CreateCheckpoint(item)
}

func (s *Server) getCheckpointForRequest(ctx context.Context, id string) (session.Checkpoint, string, error) {
	if err := ctx.Err(); err != nil {
		return session.Checkpoint{}, "", err
	}
	if typed, ok := s.store.(checkpointDeliveryContextStore); ok {
		return typed.GetCheckpointContext(ctx, id)
	}
	return s.store.GetCheckpoint(id)
}

func (s *Server) createDeliveryForRequest(ctx context.Context, item core.DeliveryReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(checkpointDeliveryContextStore); ok {
		return typed.CreateDeliveryContext(ctx, item)
	}
	return s.store.CreateDelivery(item)
}

func (s *Server) getDeliveryForRequest(ctx context.Context, id string) (core.DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return core.DeliveryReceipt{}, err
	}
	if typed, ok := s.store.(checkpointDeliveryContextStore); ok {
		return typed.GetDeliveryContext(ctx, id)
	}
	return s.store.GetDelivery(id)
}
