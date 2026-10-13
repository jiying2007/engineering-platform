package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
)

// Development Core is an explicitly scoped implementation of the same Action
// Repository authority; it cannot silently restore a legacy Service fallback.
var _ action.Repository = (*Memory)(nil)

func TestCoreMemoryActionRepositoryRespectsCancelledContext(t *testing.T) {
	core := NewMemory()
	op := action.Operation{ID: "core-scoped", RunID: "run-1", State: action.Planned, IdempotencyKey: "scoped-idempotency"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := core.CreateContext(ctx, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Core Action insert accepted: %v", err)
	}
	if _, err := core.GetContext(ctx, op.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Core Action GET accepted: %v", err)
	}
	if _, err := core.GetByIdempotencyKeyContext(ctx, op.IdempotencyKey); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Core Action key lookup accepted: %v", err)
	}
	if err := core.UpdateContext(ctx, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Core Action update accepted: %v", err)
	}
	if _, err := core.GetContext(context.Background(), op.ID); !errors.Is(err, action.ErrOperationAbsent) {
		t.Fatalf("cancelled Action reservation leaked: %v", err)
	}
	if err := core.CreateContext(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	if got, err := core.GetByIdempotencyKeyContext(context.Background(), op.IdempotencyKey); err != nil || got.ID != op.ID {
		t.Fatalf("Core scoped Action readback failed: %+v %v", got, err)
	}
}
