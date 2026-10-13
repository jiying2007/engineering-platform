package action

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func identityFixture() Operation {
	return Operation{ID: "operation", RunID: "run", ExecutionEpoch: 1, RecoveryEpoch: 2,
		Action: "ci.dispatch", RiskClass: ControlledMutation, Capability: "ci.dispatch",
		IdempotencyKey: "once", RequestDigest: "sha256:request", State: Dispatched, UpdatedAt: time.Unix(1, 0).UTC()}
}

func TestMemoryRepositoryRejectsReservationIdentityDrift(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*Operation)
	}{
		{"run", func(o *Operation) { o.RunID = "other" }},
		{"execution-epoch", func(o *Operation) { o.ExecutionEpoch++ }},
		{"recovery-epoch", func(o *Operation) { o.RecoveryEpoch++ }},
		{"action", func(o *Operation) { o.Action = "other" }},
		{"risk", func(o *Operation) { o.RiskClass = Observe }},
		{"capability", func(o *Operation) { o.Capability = "other" }},
		{"idempotency", func(o *Operation) { o.IdempotencyKey = "other" }},
		{"request-digest", func(o *Operation) { o.RequestDigest = "sha256:other" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			r := NewMemoryRepository()
			original := identityFixture()
			if err := r.CreateContext(context.Background(), original); err != nil {
				t.Fatal(err)
			}
			changed := original
			changed.State = Confirmed
			tc.change(&changed)
			if err := r.UpdateContext(context.Background(), changed); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("identity drift accepted: %v", err)
			}
			stored, err := r.GetContext(context.Background(), original.ID)
			if err != nil || stored != original {
				t.Fatalf("original reservation changed: %+v %v", stored, err)
			}
			byKey, err := r.GetByIdempotencyKeyContext(context.Background(), original.IdempotencyKey)
			if err != nil || byKey != original {
				t.Fatal("original idempotency binding changed")
			}
			original.State = Confirmed
			original.ExternalRef = "provider:original"
			original.ObservedState = "observed"
			if err := r.UpdateContext(context.Background(), original); err != nil {
				t.Fatalf("original settlement refused: %v", err)
			}
		})
	}
}

type actionCheckedContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *actionCheckedContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestMemoryRepositoryCancelledWaiterCannotAccessLedger(t *testing.T) {
	for _, operation := range []string{"create", "get", "by-key", "update"} {
		t.Run(operation, func(t *testing.T) {
			r := NewMemoryRepository()
			original := identityFixture()
			if err := r.CreateContext(context.Background(), original); err != nil {
				t.Fatal(err)
			}
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &actionCheckedContext{Context: parent, checked: make(chan struct{})}
			r.mu.Lock()
			locked := true
			defer func() {
				if locked {
					r.mu.Unlock()
				}
			}()
			done := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "create":
					next := original
					next.ID = "new"
					next.IdempotencyKey = "new"
					err = r.CreateContext(ctx, next)
				case "get":
					_, err = r.GetContext(ctx, original.ID)
				case "by-key":
					_, err = r.GetByIdempotencyKeyContext(ctx, original.IdempotencyKey)
				case "update":
					next := original
					next.State = Confirmed
					err = r.UpdateContext(ctx, next)
				}
				done <- err
			}()
			select {
			case <-ctx.checked:
			case <-time.After(2 * time.Second):
				t.Fatal("initial context check not reached")
			}
			cancel()
			r.mu.Unlock()
			locked = false
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled waiter accessed ledger: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("waiter did not return after lock release")
			}
			stored, err := r.GetContext(context.Background(), original.ID)
			if err != nil || stored != original {
				t.Fatal("cancelled update changed ledger")
			}
			if _, err := r.GetContext(context.Background(), "new"); !errors.Is(err, ErrOperationAbsent) {
				t.Fatal("cancelled creation persisted")
			}
		})
	}
}
