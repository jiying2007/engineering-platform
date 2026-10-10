package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type runStartContextFixture struct {
	store   *Store
	run     run.Run
	attempt run.Attempt
	session session.Session
	input   core.RunInputManifest
	version uint64
	work    core.WorkItem
}

func newRunStartContextFixture(t *testing.T) runStartContextFixture {
	t.Helper()
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	work := core.WorkItem{
		ID:         "start-work-" + suffix,
		Title:      "run cancellation",
		HumanOwner: "integration-test",
		TargetID:   "target-linux",
		State:      core.WorkDraft,
		Version:    1,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.CreateWorkContext(ctx, work); err != nil {
		t.Fatal(err)
	}
	plan := verification.Plan{
		ID: "start-plan-" + suffix,
		Criteria: []verification.Criterion{{
			ID: "ac-1", Statement: "CI passes",
			Requirements: []verification.EvidenceRequirement{{
				ID: "req-1", Procedure: "ci.test", Issuer: "ci",
			}},
		}},
	}
	planDigest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{
		ID: "start-task-" + suffix, WorkItemID: work.ID,
		TaskType: "FEATURE", Repository: "jiying2007/example",
		BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		TargetID:   work.TargetID, Revision: 1,
		VerificationPlanID: plan.ID, VerificationPlanDigest: planDigest,
		AcceptanceCriteria: []string{"CI passes"},
		AllowedActions:     []string{"ci.dispatch"},
	}
	digest, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	ready := work
	ready.ActiveTaskContractDigest = digest
	if err := ready.Transition(core.WorkReady); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTaskAndUpdateWorkContext(ctx, task, plan, work.Version, ready); err != nil {
		t.Fatal(err)
	}
	ready, err = s.GetWorkContext(ctx, work.ID)
	if err != nil {
		t.Fatal(err)
	}
	runID := "start-run-" + suffix
	input := core.RunInputManifest{
		RunID: runID, TaskContractDigest: digest,
		RuntimeProfile: "codex/default", ToolProfile: "tools/m1",
		WorkerProfile: "worker/ubuntu", PolicyProfile: "policy/m1",
	}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	value := run.New(runID, digest, inputDigest)
	attempt, err := value.StartAttempt("attempt-"+suffix, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(runID, attempt.Epoch)
	executing := ready
	executing.ActiveRunID = runID
	if err := executing.Transition(core.WorkExecuting); err != nil {
		t.Fatal(err)
	}
	return runStartContextFixture{
		store: s, run: *value, attempt: attempt, session: *sess,
		input: input, version: ready.Version, work: executing,
	}
}

func TestPostgresRunStartCancellationPreservesAtomicRunOutboxAndAudit(t *testing.T) {
	f := newRunStartContextFixture(t)
	s := f.store
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	blocker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(blocker)
	if _, err := blocker.Exec(ctx,
		"SELECT 1 FROM work_items WHERE work_item_id=$1 FOR UPDATE", f.work.ID,
	); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- s.CreateExecutionAndUpdateWorkContext(callCtx,
			f.run, f.attempt, f.session, f.input, f.version, f.work)
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("locked Run start unexpectedly completed: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	// This is the critical test: waiting SQL must stop while the blocker
	// still owns the Work row, not after an eventual unlocked commit.
	select {
	case err := <-done:
		if err == nil || !errors.Is(callCtx.Err(), context.Canceled) {
			t.Fatalf("cancelled Run start unexpectedly succeeded: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run start did not honor caller cancellation")
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetExecutionContext(ctx, f.run.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("cancelled Run left durable execution: %v", err)
	}
	ready, err := s.GetWorkContext(ctx, f.work.ID)
	if err != nil || ready.State != core.WorkReady ||
		ready.ActiveRunID != "" || ready.Version != f.version {
		t.Fatalf("cancelled Run changed frozen Work: %#v err=%v", ready, err)
	}
	for _, q := range []string{
		"SELECT count(*) FROM run_input_manifests WHERE run_id=$1",
		"SELECT count(*) FROM audit_events WHERE event_type='run.started' AND aggregate_id=$1",
		"SELECT count(*) FROM outbox_events WHERE topic='run.started' AND aggregate_id=$1",
	} {
		var count int
		if err := s.pool.QueryRow(ctx, q, f.run.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cancelled Run left partial authority: count=%d err=%v query=%s", count, err, q)
		}
	}
	// The same immutable subject may start once after an authoritatively
	// verified no-commit cancellation. No duplicate Outbox is manufactured.
	if err := s.CreateExecutionAndUpdateWorkContext(ctx,
		f.run, f.attempt, f.session, f.input, f.version, f.work,
	); err != nil {
		t.Fatal(err)
	}
	value, sess, err := s.GetExecutionContext(ctx, f.run.ID)
	if err != nil || value.CurrentEpoch != f.run.CurrentEpoch ||
		sess.ExecutionEpoch != f.session.ExecutionEpoch {
		t.Fatalf("valid Run start/readback failed: %#v %#v err=%v", value, sess, err)
	}
	input, err := s.GetRunInputByDigestContext(ctx, f.run.RunInputManifestDigest)
	if err != nil || input.RunID != f.run.ID {
		t.Fatalf("immutable Run input lost: %#v err=%v", input, err)
	}
	for _, q := range []string{
		"SELECT count(*) FROM audit_events WHERE event_type='run.started' AND aggregate_id=$1",
		"SELECT count(*) FROM outbox_events WHERE topic='run.started' AND aggregate_id=$1",
	} {
		var count int
		if err := s.pool.QueryRow(ctx, q, f.run.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("valid Run start did not atomically retain audit/outbox: count=%d err=%v", count, err)
		}
	}
}

func TestPostgresRunStartContextDoesNotUseCancelledReads(t *testing.T) {
	f := newRunStartContextFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.store.GetTaskByDigestContext(ctx, f.run.TaskContractDigest); err == nil {
		t.Fatal("cancelled frozen Task read returned success")
	}
	if _, _, err := f.store.GetExecutionContext(ctx, f.run.ID); err == nil {
		t.Fatal("cancelled Run read returned success")
	}
	if _, err := f.store.GetRunInputByDigestContext(ctx, f.run.RunInputManifestDigest); err == nil {
		t.Fatal("cancelled Run input read returned success")
	}
}
