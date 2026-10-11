package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/action"
)

// actionContextReader is a read-only transport capability of the existing
// Action ledger; it does not authorize a model, CI, Git/PR or device effect.
type actionContextReader interface {
	GetContext(context.Context, string) (action.Operation, error)
}

func (s *Server) getActionForRequest(ctx context.Context, operationID string) (action.Operation, error) {
	if err := ctx.Err(); err != nil {
		return action.Operation{}, err
	}
	if scoped, ok := s.actions.(actionContextReader); ok {
		return scoped.GetContext(ctx, operationID)
	}
	return s.actions.Get(operationID)
}
