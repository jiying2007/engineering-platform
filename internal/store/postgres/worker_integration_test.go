package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func workerOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func workerSQL(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	_, err := s.pool.Exec(context.Background(), query, args...)
	workerOK(t, err)
}
func startWorkerFixture(t *testing.T, s *Store, suffix string) core.RunInputManifest {
	t.Helper()
	work := core.WorkItem{ID: "worker-work-" + suffix, Title: "worker admission fixture", HumanOwner: "fixture", State: core.WorkDraft, Version: 1, CreatedAt: time.Now().UTC()}
	workerOK(t, s.CreateWork(work))
	plan := verification.Plan{ID: "worker-plan-" + suffix, Criteria: []verification.Criterion{{ID: "ac", Statement: "test", Requirements: []verification.EvidenceRequirement{{ID: "req", Procedure: "test"}}}}}
	pd, err := plan.Digest()
	workerOK(t, err)
	task := core.TaskContract{ID: "worker-task-" + suffix, WorkItemID: work.ID, TaskType: "FEATURE", Repository: "repo", BaseCommit: strings.Repeat("a", 40), AcceptanceCriteria: []string{"test"}, VerificationPlanID: plan.ID, VerificationPlanDigest: pd, Revision: 1}
	td, err := task.Digest()
	workerOK(t, err)
	work.State = core.WorkReady
	work.ActiveTaskContractDigest = td
	workerOK(t, s.CreateTaskAndUpdateWork(task, plan, 1, work))
	input := core.RunInputManifest{RunID: "worker-run-" + suffix, TaskContractDigest: td, RuntimeProfile: "codex", ToolProfile: "tools", WorkerProfile: "worker/ubuntu", PolicyProfile: "policy"}
	digest, err := input.Digest()
	workerOK(t, err)
	value := run.New(input.RunID, td, digest)
	attempt, err := value.StartAttempt("attempt", time.Now().UTC())
	workerOK(t, err)
	sess := session.New(value.ID, attempt.Epoch)
	work, err = s.GetWork(work.ID)
	workerOK(t, err)
	version := work.Version
	work.State = core.WorkExecuting
	work.ActiveRunID = value.ID
	workerOK(t, s.CreateExecutionAndUpdateWork(*value, attempt, *sess, input, version, work))
	return input
}
func claimWorkerFixture(t *testing.T, s *Store, subject string) *workerqueue.Assignment {
	t.Helper()
	a, err := s.ClaimInput(context.Background(), subject, "worker/ubuntu")
	workerOK(t, err)
	if a == nil {
		t.Fatal("expected assignment")
	}
	return a
}
func relayWorkerFixture(t *testing.T, s *Store) {
	t.Helper()
	_, err := s.RelayRunStarts(context.Background(), "relay", 16)
	workerOK(t, err)
}
func TestWorkerInboxAtomicRelayAndDeduplication(t *testing.T) {
	s := integrationStore(t)
	input := startWorkerFixture(t, s, "atomic")
	workerSQL(t, s, `INSERT INTO outbox_events(outbox_key,topic,risk_class,payload_json) VALUES('unrelated','other.topic','OBSERVE','{}')`)
	relayWorkerFixture(t, s)
	relayWorkerFixture(t, s)
	assertCount(t, s, "SELECT count(*) FROM worker_inbox", 1)
	assertCount(t, s, "SELECT count(*) FROM outbox_events WHERE topic='other.topic' AND state='PENDING'", 1)
	workerSQL(t, s, `UPDATE outbox_events SET state='PENDING',lease_owner=NULL,lease_until=NULL WHERE topic='run.started'`)
	relayWorkerFixture(t, s)
	assertCount(t, s, "SELECT count(*) FROM worker_inbox", 1)
	status, err := s.GetInbox(context.Background(), input.RunID)
	workerOK(t, err)
	if status.State != "PENDING" {
		t.Fatalf("unexpected status: %#v", status)
	}
}
func TestWorkerInboxAuditFailureRollsBackBothSides(t *testing.T) {
	s := integrationStore(t)
	startWorkerFixture(t, s, "rollback")
	workerSQL(t, s, `DELETE FROM audit_journal_state`)
	if _, err := s.RelayRunStarts(context.Background(), "relay", 1); err == nil {
		t.Fatal("missing audit accepted")
	}
	assertCount(t, s, "SELECT count(*) FROM worker_inbox", 0)
	assertCount(t, s, "SELECT count(*) FROM outbox_events WHERE state='PENDING' AND attempt_count=0", 1)
}
func TestWorkerInboxConcurrentRelayHasOneRecipient(t *testing.T) {
	s := integrationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	startWorkerFixture(t, s, "concurrent")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := s.RelayRunStarts(ctx, fmt.Sprintf("relay-%d", id), 1)
			errs <- err
		}(n)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		workerOK(t, err)
	}
	assertCount(t, s, "SELECT count(*) FROM worker_inbox", 1)
	assertCount(t, s, "SELECT count(*) FROM outbox_events WHERE state='DISPATCHED'", 1)
}
func TestWorkerInboxConcurrentSameIdentityHasOneLiveAssignment(t *testing.T) {
	s := integrationStore(t)
	startWorkerFixture(t, s, "one")
	startWorkerFixture(t, s, "two")
	relayWorkerFixture(t, s)
	results := make(chan *workerqueue.Assignment, 8)
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		go func() {
			a, err := s.ClaimInput(context.Background(), "same-worker", "worker/ubuntu")
			results <- a
			errs <- err
		}()
	}
	count := 0
	for n := 0; n < 8; n++ {
		if <-results != nil {
			count++
		}
		workerOK(t, <-errs)
	}
	if count != 1 {
		t.Fatalf("same identity received %d concurrent assignments", count)
	}
	assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='LEASED'", 1)
	assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='PENDING'", 1)
}
func TestWorkerInboxLeaseFencingAndRetainedReceipt(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	input := startWorkerFixture(t, s, "lease")
	relayWorkerFixture(t, s)
	first := claimWorkerFixture(t, s, "worker")
	report, err := workerqueue.NewReport(*first)
	workerOK(t, err)
	if _, err := s.ReportInput(ctx, "foreign", report); !errors.Is(err, workerqueue.ErrLease) {
		t.Fatalf("foreign report: %v", err)
	}
	workerSQL(t, s, `UPDATE worker_inbox SET lease_until=clock_timestamp()-interval '1 second' WHERE inbox_id=$1`, first.Token.InboxID)
	if _, err := s.ReportInput(ctx, "worker", report); !errors.Is(err, workerqueue.ErrLease) {
		t.Fatalf("expired report: %v", err)
	}
	second := claimWorkerFixture(t, s, "worker")
	if second.Token.Generation != first.Token.Generation+1 {
		t.Fatal("generation not fenced")
	}
	if _, err := s.ReportInput(ctx, "worker", report); !errors.Is(err, workerqueue.ErrLease) {
		t.Fatalf("old same-worker report: %v", err)
	}
	if _, err := s.RenewInput(ctx, "worker", first.Token); !errors.Is(err, workerqueue.ErrLease) {
		t.Fatalf("old same-worker renewal: %v", err)
	}
	report, err = workerqueue.NewReport(*second)
	workerOK(t, err)
	_, err = s.RenewInput(ctx, "worker", second.Token)
	workerOK(t, err)
	receipt, err := s.ReportInput(ctx, "worker", report)
	workerOK(t, err)
	again, err := s.ReportInput(ctx, "worker", report)
	workerOK(t, err)
	before, _ := json.Marshal(receipt)
	after, _ := json.Marshal(again)
	if string(before) != string(after) {
		t.Fatal("duplicate receipt not retained")
	}
	if receipt.Kind != workerqueue.Validated || receipt.Validation.ExecutionStarted || receipt.Validation.ContextBytesVerified {
		t.Fatal("receipt claims execution")
	}
	current, _, err := s.GetExecution(input.RunID)
	workerOK(t, err)
	if current.State != run.Running {
		t.Fatal("admission completed Run")
	}
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.input.validated'", 1)
	workerOK(t, s.ApplyCoreMigration(ctx))
	status, err := s.GetInbox(ctx, input.RunID)
	workerOK(t, err)
	stored, _ := json.Marshal(status.Receipt)
	if string(before) != string(stored) || status.Generation != second.Token.Generation {
		t.Fatal("migration changed retained receipt/generation")
	}
	assertCount(t, s, "SELECT count(*) FROM core_schema_migrations WHERE version=3", 1)
}
func TestWorkerInboxRecoveryAndTakeoverFenceActiveLease(t *testing.T) {
	for _, mode := range []string{"recovery", "takeover", "pause"} {
		t.Run(mode, func(t *testing.T) {
			s := integrationStore(t)
			ctx := context.Background()
			input := startWorkerFixture(t, s, mode)
			relayWorkerFixture(t, s)
			a := claimWorkerFixture(t, s, "worker")
			report, err := workerqueue.NewReport(*a)
			workerOK(t, err)
			switch mode {
			case "recovery":
				_, err := s.BeginRecovery(0)
				workerOK(t, err)
			case "takeover":
				workerSQL(t, s, `UPDATE runs SET current_epoch=current_epoch+1,state='HUMAN_CONTROLLED',control_owner='HUMAN' WHERE run_id=$1`, input.RunID)
			case "pause":
				workerSQL(t, s, `UPDATE runs SET state='PAUSED' WHERE run_id=$1`, input.RunID)
			}
			if _, err := s.ReportInput(ctx, "worker", report); err == nil {
				t.Fatal("stale authority acknowledged")
			}
			if _, err := s.RenewInput(ctx, "worker", a.Token); err == nil {
				t.Fatal("stale authority renewed")
			}
			assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='INPUT_VALIDATED'", 0)
		})
	}
}
func TestWorkerInboxCompletedRecoveryDoesNotReviveOldLease(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	startWorkerFixture(t, s, "epoch")
	relayWorkerFixture(t, s)
	a := claimWorkerFixture(t, s, "worker")
	report, err := workerqueue.NewReport(*a)
	workerOK(t, err)
	active, err := s.BeginRecovery(0)
	workerOK(t, err)
	// Direct Store call is only a fixture, not an implementation of the protected
	// API's independent reconciliation gate, which remains unavailable.
	_, err = s.CompleteRecovery(active.Epoch, true)
	workerOK(t, err)
	if _, err := s.ReportInput(ctx, "worker", report); !errors.Is(err, workerqueue.ErrRecovery) {
		t.Fatalf("old epoch revived: %v", err)
	}
	if _, err := s.RenewInput(ctx, "worker", a.Token); !errors.Is(err, workerqueue.ErrRecovery) {
		t.Fatalf("old epoch renewed: %v", err)
	}
}

func waitWorkerBlock(t *testing.T, ctx context.Context, s *Store, blocker uint32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, int32(blocker)).Scan(&waiting)
		workerOK(t, err)
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expected database lock wait was not observed")
}
func TestWorkerInboxPauseBetweenSelectionAndLockDoesNotRevoke(t *testing.T) {
	s := integrationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input := startWorkerFixture(t, s, "pause-race")
	relayWorkerFixture(t, s)
	blocker, err := s.pool.Begin(ctx)
	workerOK(t, err)
	defer rollbackOutbox(blocker)
	_, err = blocker.Exec(ctx, `SELECT run_id FROM runs WHERE run_id=$1 FOR UPDATE`, input.RunID)
	workerOK(t, err)
	type result struct {
		a   *workerqueue.Assignment
		err error
	}
	done := make(chan result, 1)
	go func() {
		a, err := s.ClaimInput(ctx, "worker", "worker/ubuntu")
		done <- result{a, err}
	}()
	waitWorkerBlock(t, ctx, s, blocker.Conn().PgConn().PID())
	_, err = blocker.Exec(ctx, `UPDATE runs SET state='PAUSED' WHERE run_id=$1`, input.RunID)
	workerOK(t, err)
	workerOK(t, blocker.Commit(ctx))
	got := <-done
	workerOK(t, got.err)
	if got.a != nil {
		t.Fatal("paused intent was claimed")
	}
	assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='PENDING' AND lease_generation=0", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.input.revoked'", 0)
	workerSQL(t, s, `UPDATE runs SET state='RUNNING' WHERE run_id=$1`, input.RunID)
	claimWorkerFixture(t, s, "worker")
}
func TestWorkerInboxReceiptAndRenewalCannotOutliveAuditWait(t *testing.T) {
	for _, operation := range []string{"report", "renew"} {
		t.Run(operation, func(t *testing.T) {
			s := integrationStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			startWorkerFixture(t, s, operation)
			relayWorkerFixture(t, s)
			a := claimWorkerFixture(t, s, "worker")
			report, err := workerqueue.NewReport(*a)
			workerOK(t, err)
			blocker, err := s.pool.Begin(ctx)
			workerOK(t, err)
			defer rollbackOutbox(blocker)
			_, err = blocker.Exec(ctx, `SELECT singleton_id FROM audit_journal_state WHERE singleton_id=true FOR UPDATE`)
			workerOK(t, err)
			workerSQL(t, s, `UPDATE worker_inbox SET lease_until=clock_timestamp()+interval '1 second' WHERE inbox_id=$1`, a.Token.InboxID)
			done := make(chan error, 1)
			go func() {
				var err error
				if operation == "report" {
					_, err = s.ReportInput(ctx, "worker", report)
				} else {
					_, err = s.RenewInput(ctx, "worker", a.Token)
				}
				done <- err
			}()
			waitWorkerBlock(t, ctx, s, blocker.Conn().PgConn().PID())
			time.Sleep(1100 * time.Millisecond)
			workerOK(t, blocker.Commit(ctx))
			if err := <-done; !errors.Is(err, workerqueue.ErrLease) {
				t.Fatalf("expired %s after journal wait: %v", operation, err)
			}
			assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='INPUT_VALIDATED'", 0)
			assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type IN ('worker.input.validated','worker.input.renewed')", 0)
		})
	}
}
