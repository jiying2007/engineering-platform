package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

func TestPostgresCoreWorkCreateHonorsCallerCancellationDuringTableLock(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(tx)
	if _, err := tx.Exec(ctx, "LOCK TABLE work_items IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	item := core.WorkItem{
		ID: "cancelled-work-" + suffix, Title: "cancelled-intake",
		HumanOwner: "integration-test", State: core.WorkDraft, Version: 1,
		CreatedAt: time.Now().UTC(),
	}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- s.CreateWorkContext(cancelled, item)
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("locked Work insertion unexpectedly completed: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	// The real SQL must stop BEFORE releasing this independent table lock.
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Work insert claimed success: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Work creation did not honor caller deadline")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetWorkContext(ctx, item.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("unexpected Work after cancelled admission: %v", err)
	}
	var auditCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='work.created' AND aggregate_id=$1",
		item.ID,
	).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("cancellation persisted a Work/audit fragment: count=%d err=%v", auditCount, err)
	}
}

func TestPostgresTaskFreezeHonorsCallerCancellationDuringWorkRowLock(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	work := core.WorkItem{
		ID: "work-intake-" + suffix, Title: "source Work",
		HumanOwner: "integration-test", State: core.WorkDraft, Version: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.CreateWorkContext(ctx, work); err != nil {
		t.Fatal(err)
	}
	plan := verification.Plan{
		ID: "plan-" + suffix,
		Criteria: []verification.Criterion{{
			ID: "acceptance",
			Statement: "CI passes",
			Requirements: []verification.EvidenceRequirement{{
				ID: "ci", Issuer: "ci", Procedure: "ci.test",
			}},
		}},
	}
	planDigest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{
		ID: "task-" + suffix, WorkItemID: work.ID,
		TaskType: "FEATURE", Revision: 1,
		Repository: "jiying2007/engineering-platform",
		BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		VerificationPlanID: plan.ID, VerificationPlanDigest: planDigest,
		AcceptanceCriteria: []string{"CI passes"},
		AllowedActions: []string{"ci.dispatch"},
	}
	taskDigest, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	ready := work
	ready.ActiveTaskContractDigest = taskDigest
	if err := ready.Transition(core.WorkReady); err != nil {
		t.Fatal(err)
	}
	blocker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(blocker)
	if _, err := blocker.Exec(ctx,
		"SELECT 1 FROM work_items WHERE work_item_id=$1 FOR UPDATE", work.ID,
	); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- s.CreateTaskAndUpdateWorkContext(cancelled, task, plan, work.Version, ready)
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("locked Task freeze unexpectedly completed: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Task freeze reported success: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Task freeze did not honor request deadline")
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetWorkContext(ctx, work.ID); err != nil ||
		got.State != core.WorkDraft || got.ActiveTaskContractDigest != "" ||
		got.Version != 1 {
		t.Fatalf("canceled Task freeze changed Work: %#v err=%v", got, err)
	}
	if _, err := s.GetTaskContext(ctx, task.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("canceled Task freeze persisted Task: %v", err)
	}
	var auditCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='task.frozen' AND aggregate_id=$1",
		task.ID,
	).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("cancelled Task freeze wrote audit: count=%d err=%v", auditCount, err)
	}
}
