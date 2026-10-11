package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/session"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) getSteeringForRequest(ctx context.Context, id string) (session.SteeringCommand, error) {
	if err := ctx.Err(); err != nil {
		return session.SteeringCommand{}, err
	}
	return s.store.GetSteeringContext(ctx, id)
}
