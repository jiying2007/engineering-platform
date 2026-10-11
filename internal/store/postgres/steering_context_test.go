package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/session"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestSteeringAndCoreReadContextsCancelRealPostgresLockWithoutAuditResidue(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, "steering-context")
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()

	value, sess, err := s.GetExecutionContext(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetTaskByDigestContext(ctx, value.TaskContractDigest)
	if err != nil {
		t.Fatal(err)
	}
	cmd := session.SteeringCommand{
		ID:             "cancelled-steering",
		RunID:          runID,
		ExecutionEpoch: value.CurrentEpoch,
		Sequence:       1,
		Actor:          "engineer",
		ContentDigest:  "sha256:cancelled-steering",
		CreatedAt:      time.Now().UTC(),
	}
	if err := sess.ApplySteering(cmd); err != nil {
		t.Fatal(err)
	}

	// All calls must block on their original PostgreSQL authority table,
	// respect the request cancellation and release their locks.
	for _, tc := range []struct {
		name  string
		table string
		call  func(context.Context) error
	}{
		{
			name:  "steering-read",
			table: "steering_commands",
			call: func(c context.Context) error {
				_, err := s.GetSteeringContext(c, cmd.ID)
				return err
			},
		},
		{
			name:  "attempt-read",
			table: "run_attempts",
			call: func(c context.Context) error {
				_, err := s.GetAttemptContext(c, runID, value.CurrentAttemptID)
				return err
			},
		},
		{
			name:  "task-revision-read",
			table: "task_contracts",
			call: func(c context.Context) error {
				_, err := s.GetTaskRevisionContext(c, task.ID, task.Revision)
				return err
			},
		},
		{
			name:  "steering-mutation",
			table: "runs",
			call: func(c context.Context) error {
				return s.RecordSteeringContext(c, runID, value.Version, value, sess, cmd)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lock, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollbackOutbox(lock)
			if _, err := lock.Exec(ctx, "LOCK TABLE "+tc.table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
				t.Fatal(err)
			}
			caller, cancel := context.WithCancel(ctx)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tc.call(caller) }()
			select {
			case err := <-done:
				t.Fatalf("Core path bypassed held lock: %v", err)
			case <-time.After(80 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				if err == nil || !errors.Is(caller.Err(), context.Canceled) {
					t.Fatalf("Core path ignored cancellation: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("Core SQL did not cancel before lock release")
			}
			if err := lock.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}

	if _, err := s.GetSteeringContext(ctx, cmd.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("cancelled Steering mutation persisted record: %v", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='steering.recorded' AND aggregate_id=$1", runID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cancelled Steering mutation wrote audit: count=%d err=%v", count, err)
	}
	if attempt, err := s.GetAttemptContext(ctx, runID, value.CurrentAttemptID); err != nil || attempt.ID != value.CurrentAttemptID {
		t.Fatalf("historical Attempt readback changed: attempt=%+v err=%v", attempt, err)
	}
	if revision, err := s.GetTaskRevisionContext(ctx, task.ID, task.Revision); err != nil || revision.ID != task.ID {
		t.Fatalf("historical Task revision changed: revision=%+v err=%v", revision, err)
	}
	// A valid caller may still perform the original single-audit mutation
	// after the cancelled transaction, with the same immutable record.
	if err := s.RecordSteeringContext(ctx, runID, value.Version, value, sess, cmd); err != nil {
		t.Fatalf("valid Steering context path regressed: %v", err)
	}
	if persisted, err := s.GetSteeringContext(ctx, cmd.ID); err != nil || persisted.ContentDigest != cmd.ContentDigest {
		t.Fatalf("persisted Steering changed: command=%+v err=%v", persisted, err)
	}
}
