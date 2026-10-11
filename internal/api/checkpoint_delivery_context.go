package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/session"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) createCheckpointForRequest(ctx context.Context, item session.Checkpoint) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.store.CreateCheckpointContext(ctx, item)
}

func (s *Server) getCheckpointForRequest(ctx context.Context, id string) (session.Checkpoint, string, error) {
	if err := ctx.Err(); err != nil {
		return session.Checkpoint{}, "", err
	}
	return s.store.GetCheckpointContext(ctx, id)
}

func (s *Server) createDeliveryForRequest(ctx context.Context, item core.DeliveryReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateDeliveryContext(ctx, item)
}

func (s *Server) getDeliveryForRequest(ctx context.Context, id string) (core.DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return core.DeliveryReceipt{}, err
	}
	return s.store.GetDeliveryContext(ctx, id)
}
