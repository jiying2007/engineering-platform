package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
)

// Test-only historical fixture. Production code must never create this state
// without independently retained original-dispatcher quiescence; this seeds
// a pre-existing database row to prove it remains observable and blocked.
func seedHistoricalSafeToRetry(t *testing.T, s *Store, operationID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		"UPDATE external_operations SET state='SAFE_TO_RETRY' WHERE operation_id=$1 AND state='RECONCILING'",
		operationID,
	)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("could not seed historical retry state: tag=%v err=%v", tag, err)
	}
}

func TestLegacySafeToRetryCannotBeRedispatchedThroughPostgresStore(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, "no-legacy-replay")
	now := time.Now().UTC()
	req := action.Request{
		ID: "no-legacy-replay", RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci",
		ParametersDigest: "sha256:" + strings.Repeat("a", 64), IdempotencyKey: "no-legacy-replay",
		RequestedBy: "runtime", RequestedAt: now,
	}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("b", 64), now)
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	for _, state := range []action.State{action.Dispatched, action.Unknown, action.Reconciling} {
		if err := op.Transition(state, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := s.Update(*op); err != nil {
			t.Fatal(err)
		}
	}
	seedHistoricalSafeToRetry(t, s, op.ID)
	stored, err := s.Get(op.ID)
	if err != nil || stored.State != action.SafeToRetry || stored.IdempotencyKey != req.IdempotencyKey {
		t.Fatalf("historical original identity not preserved: %+v err=%v", stored, err)
	}
	for _, target := range []action.State{action.Dispatched, action.SafeToRetry} {
		forged := stored
		forged.State = target
		if err := s.Update(forged); !errors.Is(err, action.ErrInvalidTransition) {
			t.Fatalf("unqualified legacy transition %s was accepted: %v", target, err)
		}
	}
	retained, err := s.GetByIdempotencyKey(req.IdempotencyKey)
	if err != nil || retained.State != action.SafeToRetry {
		t.Fatalf("denied redispatch modified unique reservation: %+v err=%v", retained, err)
	}
}
