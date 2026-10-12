package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
)

func TestMemoryActionUpdatePreservesOriginalIdentity(t *testing.T) {
	original := action.Operation{ID: "operation", RunID: "run", ExecutionEpoch: 1, RecoveryEpoch: 2,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci.dispatch",
		IdempotencyKey: "original", RequestDigest: "sha256:original", State: action.Dispatched, UpdatedAt: time.Unix(1, 0).UTC()}
	for _, change := range []func(*action.Operation){
		func(o *action.Operation) { o.RunID = "different" }, func(o *action.Operation) { o.ExecutionEpoch++ },
		func(o *action.Operation) { o.RecoveryEpoch++ }, func(o *action.Operation) { o.Action = "other" },
		func(o *action.Operation) { o.RiskClass = action.Observe }, func(o *action.Operation) { o.Capability = "other" },
		func(o *action.Operation) { o.IdempotencyKey = "new" }, func(o *action.Operation) { o.RequestDigest = "sha256:other" },
	} {
		m := NewMemory()
		if err := m.CreateContext(context.Background(), original); err != nil {
			t.Fatal(err)
		}
		next := original
		next.State = action.Confirmed
		change(&next)
		if err := m.UpdateContext(context.Background(), next); !errors.Is(err, action.ErrInvalidTransition) {
			t.Fatalf("changed reservation accepted: %v", err)
		}
		read, err := m.GetByIdempotencyKeyContext(context.Background(), original.IdempotencyKey)
		if err != nil || read != original {
			t.Fatal("original ledger/key changed")
		}
	}
}
