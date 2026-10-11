package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/action"
)

// The ActionGateway interface now requires a caller-bound GetContext.
// This read is observation-free and never dispatches or replays an external
// model, CI, Git/PR or device effect.
func (s *Server) getActionForRequest(ctx context.Context, operationID string) (action.Operation, error) {
	if err := ctx.Err(); err != nil {
		return action.Operation{}, err
	}
	return s.actions.GetContext(ctx, operationID)
}
