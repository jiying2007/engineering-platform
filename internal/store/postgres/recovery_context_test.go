package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/recovery"
)

func TestRecoveryLifecycleContextCancelsPostgresLockWithoutStateLeak(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()

	// platform_state is the same single Recovery authority across all three
	// calls. Blocking the table fences both an initial read and mutations.
	waitThenCancel := func(name string, call func(context.Context) error) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			lock, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollbackOutbox(lock)
			if _, err := lock.Exec(ctx, "LOCK TABLE platform_state IN ACCESS EXCLUSIVE MODE"); err != nil {
				t.Fatal(err)
			}
			caller, cancel := context.WithCancel(ctx)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- call(caller) }()
			select {
			case err := <-done:
				t.Fatalf("Recovery operation bypassed held lock: %v", err)
			case <-time.After(80 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				if err == nil || !errors.Is(caller.Err(), context.Canceled) {
					t.Fatalf("Recovery operation ignored cancellation: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("Recovery transaction failed to cancel before lock release")
			}
			if err := lock.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
	waitThenCancel("get", func(c context.Context) error {
		_, err := s.GetRecoveryContext(c)
		return err
	})
	waitThenCancel("begin", func(c context.Context) error {
		_, err := s.BeginRecoveryContext(c, 0)
		return err
	})
	state, err := s.GetRecoveryContext(ctx)
	if err != nil || state.Epoch != 0 || state.Mode != recovery.Normal {
		t.Fatalf("cancelled begin mutated Recovery epoch: state=%+v err=%v", state, err)
	}
	var beginCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='recovery.started'").Scan(&beginCount); err != nil || beginCount != 0 {
		t.Fatalf("cancelled begin created audit: count=%d err=%v", beginCount, err)
	}

	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:reconciler")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeCompletion(ctx, "urn:engineering-platform:operator:completer", proof.RecoveryEpoch); err != nil {
		t.Fatal(err)
	}
	waitThenCancel("complete", func(c context.Context) error {
		_, err := s.CompleteRecoveryContext(c, 1, true)
		return err
	})
	state, err = s.GetRecoveryContext(ctx)
	if err != nil || state.Epoch != 1 || state.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("cancelled completion changed recovery mode: state=%+v err=%v", state, err)
	}
	var completedCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='recovery.completed'").Scan(&completedCount); err != nil || completedCount != 0 {
		t.Fatalf("cancelled completion left success audit: count=%d err=%v", completedCount, err)
	}
	if _, err := s.GetRecoveryProof(ctx, 1); err != nil {
		t.Fatalf("cancelled completion lost immutable proof: %v", err)
	}
	if final, err := s.CompleteRecoveryContext(ctx, 1, true); err != nil || final.Mode != recovery.Normal {
		t.Fatalf("subsequent authorized completion failed: state=%+v err=%v", final, err)
	}
}
