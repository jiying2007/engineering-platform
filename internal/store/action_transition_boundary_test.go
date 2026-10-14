package store

import (
	"context"
	"errors"
	"github.com/jiying2007/engineering-platform/internal/action"
	"testing"
	"time"
)

func TestCoreMemoryActionUpdatesPreserveTerminalAndConfirmationBoundary(t *testing.T) {
	for _, from := range []action.State{action.Confirmed, action.Manual, action.AbandonedReconciled, action.SafeToRetry, action.Reconciling} {
		for _, to := range []action.State{action.Dispatched, action.Planned, action.Confirmed} {
			t.Run(string(from)+"/"+string(to), func(t *testing.T) {
				s := NewMemory()
				ctx := context.Background()
				original := action.Operation{ID: "reserved", RunID: "run", State: from, RiskClass: action.ControlledMutation, IdempotencyKey: "once", UpdatedAt: time.Unix(1, 0)}
				if err := s.CreateContext(ctx, original); err != nil {
					t.Fatal(err)
				}
				changed := original
				changed.State = to
				changed.UpdatedAt = time.Unix(2, 0)
				if err := s.UpdateContext(ctx, changed); !errors.Is(err, action.ErrInvalidTransition) {
					t.Fatal("Core memory accepted non-authoritative transition", err)
				}
				got, err := s.GetByIdempotencyKeyContext(ctx, original.IdempotencyKey)
				if err != nil || got != original {
					t.Fatal("rejected update changed Core memory", got, err)
				}
			})
		}
	}
}
