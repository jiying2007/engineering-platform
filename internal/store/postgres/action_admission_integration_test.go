package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/recovery"
)

func actionAdmissionFixture(t *testing.T) (*Store, action.Operation) {
	t.Helper()
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, fmt.Sprintf("%d", time.Now().UnixNano()))
	op := *action.NewWithRequestDigest(action.Request{
		ID: "test-action-admission", RunID: runID,
		ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation,
		Capability: "ci.dispatch", IdempotencyKey: "test-admission-once",
	}, "sha256:admission-test", time.Now().UTC())
	return s, op
}

func TestPostgresActionAdmissionChecksRecoveryInSameTransaction(t *testing.T) {
	s, op := actionAdmissionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A Recovery Begin update owns the platform row before its commit.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE platform_state SET recovery_epoch=1,recovery_mode='RECOVERY_RECONCILIATION'
WHERE singleton_id=true AND recovery_epoch=0 AND recovery_mode='NORMAL'`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Create(op) }()
	select {
	case err := <-done:
		t.Fatalf("admission bypassed unfinished recovery transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, recovery.ErrStaleEpoch) {
			t.Fatalf("stale recovery epoch admitted: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("admission did not complete after recovery committed")
	}
	if _, err := s.Get(op.ID); !errors.Is(err, action.ErrOperationAbsent) {
		t.Fatalf("rejected action created durable operation: %v", err)
	}
	op.RecoveryEpoch = 1
	if err := s.Create(op); !errors.Is(err, recovery.ErrRecoveryMode) {
		t.Fatalf("mutation admitted during recovery: %v", err)
	}
	op.RiskClass = action.Observe
	if err := s.Create(op); err != nil {
		t.Fatalf("read-only observation unexpectedly denied during recovery: %v", err)
	}
}

func TestPostgresActionAdmissionFencesRunControlAndSession(t *testing.T) {
	for _, tc := range []struct {
		name     string
		runState string
		runOwner string
		paused   bool
		epoch    uint64
	}{
		{name: "paused", runState: "PAUSED", runOwner: "RUNTIME", paused: true, epoch: 1},
		{name: "session-paused", runState: "RUNNING", runOwner: "RUNTIME", paused: true, epoch: 1},
		{name: "aborted", runState: "ABORTED", runOwner: "RUNTIME", epoch: 1},
		{name: "completed", runState: "COMPLETED", runOwner: "RUNTIME", epoch: 1},
		{name: "human-takeover", runState: "HUMAN_CONTROLLED", runOwner: "HUMAN", epoch: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, op := actionAdmissionFixture(t)
			ctx := context.Background()
			if _, err := s.pool.Exec(ctx,
				"UPDATE runs SET state=$1,control_owner=$2,current_epoch=$3 WHERE run_id=$4",
				tc.runState, tc.runOwner, tc.epoch, op.RunID,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := s.pool.Exec(ctx, "UPDATE sessions SET paused=$1 WHERE run_id=$2", tc.paused, op.RunID); err != nil {
				t.Fatal(err)
			}
			if err := s.Create(op); !errors.Is(err, action.ErrDenied) {
				t.Fatalf("non-runtime or paused Run admitted privileged Action: %v", err)
			}
			if _, err := s.Get(op.ID); !errors.Is(err, action.ErrOperationAbsent) {
				t.Fatalf("denied Action left a reservation: %v", err)
			}
		})
	}
}

func TestPostgresActionDispatchRechecksChangesAfterPlannedReservation(t *testing.T) {
	t.Run("recovery-begin", func(t *testing.T) {
		s, op := actionAdmissionFixture(t)
		if err := s.Create(op); err != nil {
			t.Fatal(err)
		}
		if _, err := s.BeginRecovery(0); err != nil {
			t.Fatal(err)
		}
		if err := op.Transition(action.Dispatched, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if err := s.Update(op); !errors.Is(err, recovery.ErrStaleEpoch) {
			t.Fatalf("dispatched stale recovery grant: %v", err)
		}
		stored, err := s.Get(op.ID)
		if err != nil || stored.State != action.Planned {
			t.Fatalf("recovery race changed operation into dispatched: %#v %v", stored, err)
		}
	})
	t.Run("pause-after-plan", func(t *testing.T) {
		s, op := actionAdmissionFixture(t)
		if err := s.Create(op); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(context.Background(), "UPDATE runs SET state='PAUSED' WHERE run_id=$1", op.RunID); err != nil {
			t.Fatal(err)
		}
		if err := op.Transition(action.Dispatched, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if err := s.Update(op); !errors.Is(err, action.ErrDenied) {
			t.Fatalf("dispatched privileged action after Pause: %v", err)
		}
		stored, err := s.Get(op.ID)
		if err != nil || stored.State != action.Planned {
			t.Fatalf("pause race changed operation into dispatched: %#v %v", stored, err)
		}
	})
}
