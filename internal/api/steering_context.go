package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/session"
)

// steeringContextStore binds historical steering readback to the original
// caller without changing the Codex live control-delivery state machine.
type steeringContextStore interface {
	GetSteeringContext(context.Context, string) (session.SteeringCommand, error)
}

func (s *Server) getSteeringForRequest(ctx context.Context, id string) (session.SteeringCommand, error) {
	if err := ctx.Err(); err != nil {
		return session.SteeringCommand{}, err
	}
	if typed, ok := s.store.(steeringContextStore); ok {
		return typed.GetSteeringContext(ctx, id)
	}
	return s.store.GetSteering(id)
}
