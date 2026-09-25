package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

// These database fixtures assert attributed receipt semantics, not real files.
// Actual bytes/Git and compiled Worker processing are tested by the command test.
func preparationFixture(t *testing.T, s *Store) (workerqueue.Assignment, preparation.Report) {
	t.Helper()
	startWorkerFixture(t, s, "preparation")
	relayWorkerFixture(t, s)
	a := *claimWorkerFixture(t, s, "prepare-worker")
	input, err := workerqueue.NewReport(a)
	workerOK(t, err)
	manifest := contextbundle.Manifest{SchemaVersion: 1, RunInputDigest: a.Intent.InputDigest, Entries: []contextbundle.Entry{}}
	raw, err := json.Marshal(manifest)
	workerOK(t, err)
	f := preparation.Facts{Version: 1, IntentDigest: a.IntentDigest, InputDigest: a.Intent.InputDigest, TaskDigest: a.Intent.TaskDigest, ApprovalDigest: canonical.BytesDigest([]byte("fixture-approval")), BaseCommit: a.Task.BaseCommit, TreeCommit: strings.Repeat("b", 40), WorkspaceRecipe: workspace.Recipe, SourceDigest: canonical.BytesDigest([]byte("fixture-source")), ConfigDigest: canonical.BytesDigest([]byte("fixture-config")), BundleDigest: canonical.BytesDigest(raw), Context: manifest}
	workerOK(t, f.Check(a))
	return a, preparation.Report{Input: input, Facts: f}
}
func TestPreparationStoreAtomicReceiptReplayAndMigration(t *testing.T) {
	s := integrationStore(t)
	a, report := preparationFixture(t, s)
	ctx := context.Background()
	workerOK(t, s.PreparationReady(ctx))
	first, err := s.ReportPrepared(ctx, "prepare-worker", report)
	workerOK(t, err)
	workerOK(t, preparation.Verify(a, first, report.Facts, "prepare-worker"))
	// Readback is history even during recovery; no new action is admitted.
	_, err = s.BeginRecovery(a.Token.RecoveryEpoch)
	workerOK(t, err)
	replay, err := s.ReportPrepared(ctx, "prepare-worker", report)
	workerOK(t, err)
	before, _ := json.Marshal(first)
	after, _ := json.Marshal(replay)
	if string(before) != string(after) {
		t.Fatal("duplicate report changed retained history")
	}
	workerOK(t, s.ApplyCoreMigration(ctx))
	stored, err := s.GetPreparation(ctx, a.Intent.RunID)
	workerOK(t, err)
	after, _ = json.Marshal(stored)
	if string(before) != string(after) {
		t.Fatal("migration changed preparation receipt")
	}
	assertCount(t, s, "SELECT count(*) FROM worker_preparations", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.preparation.recorded'", 1)
	assertCount(t, s, "SELECT count(*) FROM runs WHERE state='RUNNING'", 1)
	assertCount(t, s, "SELECT count(*) FROM evidence", 0)
	changed := report
	changed.Facts.SourceDigest = canonical.BytesDigest([]byte("changed"))
	if _, err := s.ReportPrepared(ctx, "prepare-worker", changed); !errors.Is(err, workerqueue.ErrIdentity) {
		t.Fatalf("changed replay accepted: %v", err)
	}
}
func TestPreparationStoreConcurrentIdenticalReportsHaveOneReceipt(t *testing.T) {
	s := integrationStore(t)
	_, report := preparationFixture(t, s)
	results := make(chan preparation.Receipt, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		go func() {
			r, err := s.ReportPrepared(context.Background(), "prepare-worker", report)
			results <- r
			errs <- err
		}()
	}
	first := ""
	for i := 0; i < 6; i++ {
		r := <-results
		workerOK(t, <-errs)
		raw, _ := json.Marshal(r)
		if first == "" {
			first = string(raw)
		}
		if string(raw) != first {
			t.Fatal("concurrent receipt changed")
		}
	}
	assertCount(t, s, "SELECT count(*) FROM worker_preparations", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.preparation.recorded'", 1)
}
func TestPreparationStoreRejectsStaleForeignAndInflatedReports(t *testing.T) {
	for _, kind := range []string{"foreign", "generation", "recovery", "paused", "expired", "execution", "bundle"} {
		t.Run(kind, func(t *testing.T) {
			s := integrationStore(t)
			a, report := preparationFixture(t, s)
			subject := "prepare-worker"
			switch kind {
			case "foreign":
				subject = "other-worker"
			case "generation":
				report.Input.Token.Generation++
			case "recovery":
				_, err := s.BeginRecovery(a.Token.RecoveryEpoch)
				workerOK(t, err)
			case "paused":
				workerSQL(t, s, "UPDATE sessions SET paused=true")
			case "expired":
				workerSQL(t, s, "UPDATE worker_inbox SET lease_until=clock_timestamp()-interval '1 second'")
			case "execution":
				report.Facts.ExecutionStarted = true
			case "bundle":
				report.Facts.BundleDigest = canonical.BytesDigest([]byte("not manifest"))
			}
			if _, err := s.ReportPrepared(context.Background(), subject, report); err == nil {
				t.Fatal("invalid preparation accepted")
			}
			assertCount(t, s, "SELECT count(*) FROM worker_preparations", 0)
			assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='LEASED'", 1)
		})
	}
}
func TestPreparationStoreAuditFailureRollsBackBothReceipts(t *testing.T) {
	s := integrationStore(t)
	_, report := preparationFixture(t, s)
	workerSQL(t, s, "DELETE FROM audit_journal_state")
	if _, err := s.ReportPrepared(context.Background(), "prepare-worker", report); err == nil {
		t.Fatal("missing audit accepted")
	}
	assertCount(t, s, "SELECT count(*) FROM worker_preparations", 0)
	assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND receipt_json IS NULL", 1)
}
func TestPreparationStoreAuditWaitCannotExtendExpiredAuthority(t *testing.T) {
	s := integrationStore(t)
	_, report := preparationFixture(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	blocker, err := s.pool.Begin(ctx)
	workerOK(t, err)
	defer rollbackOutbox(blocker)
	_, err = blocker.Exec(ctx, "SELECT singleton_id FROM audit_journal_state FOR UPDATE")
	workerOK(t, err)
	workerSQL(t, s, "UPDATE worker_inbox SET lease_until=clock_timestamp()+interval '200 milliseconds'")
	done := make(chan error, 1)
	go func() { _, err := s.ReportPrepared(ctx, "prepare-worker", report); done <- err }()
	// The held journal forces the final expiry check to occur after this release.
	time.Sleep(300 * time.Millisecond)
	workerOK(t, blocker.Commit(ctx))
	if err := <-done; !errors.Is(err, workerqueue.ErrLease) {
		t.Fatalf("expired receipt survived journal wait: %v", err)
	}
	assertCount(t, s, "SELECT count(*) FROM worker_preparations", 0)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.preparation.recorded'", 0)
}
