package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
)

func lockRunForControlTest(t *testing.T, ctx context.Context, s *Store, runID string) (func(), error) {
	t.Helper()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT 1 FROM runs WHERE run_id=$1 FOR UPDATE", runID); err != nil {
		rollbackOutbox(tx)
		return nil, err
	}
	return func() {
		rollbackOutbox(tx)
	}, nil
}

func TestPostgresRunControlCancellationStopsLockedRunUpdate(t *testing.T) {
	f := newRunStartContextFixture(t)
	s := f.store
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := s.CreateExecutionAndUpdateWorkContext(ctx, f.run, f.attempt, f.session,
		f.input, f.version, f.work); err != nil {
		t.Fatal(err)
	}
	value, sess, err := s.GetExecutionContext(ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	version := value.Version
	if err := value.Pause(value.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := sess.Pause(sess.ExecutionEpoch); err != nil {
		t.Fatal(err)
	}
	release, err := lockRunForControlTest(t, ctx, s, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- s.UpdateExecutionContext(callCtx, f.run.ID, version, value, sess)
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("locked Run control unexpectedly completed: %v", err)
	case <-time.After(80*time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || !errors.Is(callCtx.Err(), context.Canceled) {
			t.Fatalf("canceled Run control reported success: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run control did not respect request cancellation")
	}
	release()
	stored, currentSession, err := s.GetExecutionContext(ctx, f.run.ID)
	if err != nil || stored.State != run.Running || stored.Version != version ||
		currentSession.Paused {
		t.Fatalf("canceled Run control changed state: %#v %#v err=%v",
			stored, currentSession, err)
	}
	var audits int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='run.updated' AND aggregate_id=$1",
		f.run.ID).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("canceled Run control wrote audit: count=%d err=%v", audits, err)
	}
	if err := s.UpdateExecutionContext(ctx, f.run.ID, version, value, sess); err != nil {
		t.Fatal(err)
	}
	stored, currentSession, err = s.GetExecutionContext(ctx, f.run.ID)
	if err != nil || stored.State != run.Paused || !currentSession.Paused {
		t.Fatalf("valid Run pause was denied after cancelled request: %#v %#v err=%v", stored, currentSession, err)
	}
}

func TestPostgresRunCompletionCancellationPreservesWorkAndAudit(t *testing.T) {
	f := newRunStartContextFixture(t)
	s := f.store
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := s.CreateExecutionAndUpdateWorkContext(ctx, f.run, f.attempt, f.session,
		f.input, f.version, f.work); err != nil {
		t.Fatal(err)
	}
	value, sess, err := s.GetExecutionContext(ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	version := value.Version
	if err := value.Complete(value.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	work, err := s.GetWorkContext(ctx, f.work.ID)
	if err != nil {
		t.Fatal(err)
	}
	workVersion := work.Version
	if err := work.Transition(core.WorkVerifying); err != nil {
		t.Fatal(err)
	}
	release, err := lockRunForControlTest(t, ctx, s, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- s.UpdateExecutionAndWorkContext(callCtx,
			f.run.ID, version, value, sess, workVersion, work)
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("locked Run completion unexpectedly finished: %v", err)
	case <-time.After(80*time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || callCtx.Err() != context.Canceled {
			t.Fatalf("cancelled Run completion reported success: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run completion ignored caller cancellation")
	}
	release()
	stored, currentSession, err := s.GetExecutionContext(ctx, f.run.ID)
	if err != nil || stored.State != run.Running || stored.Version != version ||
		currentSession.ExecutionEpoch != sess.ExecutionEpoch {
		t.Fatalf("cancelled completion changed Run/Session: %#v %#v err=%v", stored, currentSession, err)
	}
	storedWork, err := s.GetWorkContext(ctx, f.work.ID)
	if err != nil || storedWork.State != core.WorkExecuting || storedWork.Version != workVersion {
		t.Fatalf("cancelled completion changed Work: %#v err=%v", storedWork, err)
	}
	var audits int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='run.work.updated' AND aggregate_id=$1",
		f.run.ID).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("cancelled completion retained partial audit: count=%d err=%v", audits, err)
	}
	if err := s.UpdateExecutionAndWorkContext(ctx,
		f.run.ID, version, value, sess, workVersion, work); err != nil {
		t.Fatalf("valid completion after canceled non-commit failed: %v", err)
	}
	stored, _, err = s.GetExecutionContext(ctx, f.run.ID)
	if err != nil || stored.State != run.Completed {
		t.Fatalf("Run completion was not durable: %#v err=%v", stored, err)
	}
	storedWork, err = s.GetWorkContext(ctx, f.work.ID)
	if err != nil || storedWork.State != core.WorkVerifying {
		t.Fatalf("Work did not match completed Run: %#v err=%v", storedWork, err)
	}
	for _, q := range []string{
		"SELECT count(*) FROM audit_events WHERE event_type='run.work.updated' AND aggregate_id=$1",
		"SELECT count(*) FROM audit_events WHERE event_type='run.started' AND aggregate_id=$1",
	} {
		var count int
		if err := s.pool.QueryRow(ctx, q, f.run.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("valid Run history not auditable: count=%d err=%v query=%s", count, err, q)
		}
	}
}

