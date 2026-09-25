package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

// Synthetic filesystem observations test DB authority only. Real container/file
// observations are separately exercised by the command-level integration suite.
func offlineFixture(t *testing.T, s *Store) offline.Start {
	t.Helper()
	ctx := context.Background()
	p := sandbox.Profile{Image: sandbox.Hash([]byte("fixture image")), GuardDigest: sandbox.Hash([]byte("guard")), Argv: []string{"/probe"}, Seconds: 10}
	pd, err := p.Digest()
	workerOK(t, err)
	work := core.WorkItem{ID: "offline-work", Title: "offline fixture", HumanOwner: "fixture", State: core.WorkDraft, Version: 1, CreatedAt: time.Now().UTC()}
	workerOK(t, s.CreateWork(work))
	plan := verification.Plan{ID: "offline-plan", Criteria: []verification.Criterion{{ID: "ac", Statement: "test", Requirements: []verification.EvidenceRequirement{{ID: "req", Procedure: "test"}}}}}
	planDigest, err := plan.Digest()
	workerOK(t, err)
	task := core.TaskContract{ID: "offline-task", WorkItemID: work.ID, TaskType: "FEATURE", Repository: "repo", BaseCommit: strings.Repeat("a", 40), AcceptanceCriteria: []string{"test"}, AllowedActions: []string{offline.Action}, VerificationPlanID: plan.ID, VerificationPlanDigest: planDigest, Revision: 1}
	td, err := task.Digest()
	workerOK(t, err)
	work.State = core.WorkReady
	work.ActiveTaskContractDigest = td
	workerOK(t, s.CreateTaskAndUpdateWork(task, plan, 1, work))
	input := core.RunInputManifest{RunID: "offline-run", TaskContractDigest: td, RuntimeProfile: "offline-check", ToolProfile: "offline/" + pd, WorkerProfile: "worker/ubuntu", PolicyProfile: "policy"}
	id, err := input.Digest()
	workerOK(t, err)
	value := run.New(input.RunID, td, id)
	attempt, err := value.StartAttempt("attempt", time.Now().UTC())
	workerOK(t, err)
	sess := session.New(value.ID, attempt.Epoch)
	work, err = s.GetWork(work.ID)
	workerOK(t, err)
	version := work.Version
	work.State = core.WorkExecuting
	work.ActiveRunID = value.ID
	workerOK(t, s.CreateExecutionAndUpdateWork(*value, attempt, *sess, input, version, work))
	relayWorkerFixture(t, s)
	a := *claimWorkerFixture(t, s, "offline-worker")
	ir, err := workerqueue.NewReport(a)
	workerOK(t, err)
	manifest := contextbundle.Manifest{SchemaVersion: 1, RunInputDigest: id, Entries: []contextbundle.Entry{}}
	raw, _ := json.Marshal(manifest)
	facts := preparation.Facts{Version: 1, IntentDigest: a.IntentDigest, InputDigest: id, TaskDigest: td, ApprovalDigest: sandbox.Hash([]byte("approval")), BaseCommit: task.BaseCommit, TreeCommit: strings.Repeat("b", 40), WorkspaceRecipe: workspace.Recipe, SourceDigest: sandbox.Hash([]byte("source")), ConfigDigest: sandbox.Hash([]byte("config")), BundleDigest: sandbox.Hash(raw), Context: manifest}
	_, err = s.ReportPrepared(ctx, "offline-worker", preparation.Report{Input: ir, Facts: facts})
	workerOK(t, err)
	return offline.Start{RunID: input.RunID, WorkerProfile: input.WorkerProfile, Profile: p}
}
func offlineResult(p offline.Permit) sandbox.Result {
	return sandbox.Result{Recipe: sandbox.Recipe, ProfileDigest: p.Token.ProfileDigest, ContainerID: strings.Repeat("f", 64), ExitCode: 0, UserID: 1000, Stdout: []byte("actual reported bytes"), Stderr: []byte{}, StdoutDigest: sandbox.Hash([]byte("actual reported bytes")), StderrDigest: sandbox.Hash(nil)}
}
func TestOfflineStoreFreshGrantReplayAndMigration(t *testing.T) {
	s := integrationStore(t)
	req := offlineFixture(t, s)
	ctx := context.Background()
	p, err := s.StartOffline(ctx, "offline-worker", req)
	workerOK(t, err)
	workerOK(t, p.Check("offline-worker", req))
	if _, err = s.StartOffline(ctx, "offline-worker", req); err == nil {
		t.Fatal("duplicate start allowed")
	}
	report := offline.Report{Token: p.Token, Result: offlineResult(p)}
	first, err := s.FinishOffline(ctx, "offline-worker", report)
	workerOK(t, err)
	workerOK(t, first.Verify("offline-worker", p, report.Result))
	_, err = s.BeginRecovery(p.Token.RecoveryEpoch)
	workerOK(t, err)
	second, err := s.FinishOffline(ctx, "offline-worker", report)
	workerOK(t, err)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatal("replay changed receipt")
	}
	report.Result.Stdout = []byte("changed")
	report.Result.StdoutDigest = sandbox.Hash(report.Result.Stdout)
	if _, err = s.FinishOffline(ctx, "offline-worker", report); !errors.Is(err, workerqueue.ErrIdentity) {
		t.Fatalf("altered replay accepted %v", err)
	}
	workerOK(t, s.ApplyCoreMigration(ctx))
	stored, err := s.GetOffline(ctx, req.RunID)
	workerOK(t, err)
	b, _ = json.Marshal(stored.Receipt)
	if string(a) != string(b) {
		t.Fatal("migration changed receipt")
	}
	workerOK(t, s.FailOffline(ctx, "offline-worker", p.Token))
	assertCount(t, s, "SELECT count(*) FROM worker_offline_executions WHERE state='FINISHED'", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.offline.finished'", 1)
	assertCount(t, s, "SELECT count(*) FROM evidence", 0)
	assertCount(t, s, "SELECT count(*) FROM runs WHERE state='RUNNING'", 1)
}
func TestOfflineStoreConcurrentStartHasOneReservation(t *testing.T) {
	s := integrationStore(t)
	req := offlineFixture(t, s)
	results := make(chan error, 6)
	for n := 0; n < 6; n++ {
		go func() { _, err := s.StartOffline(context.Background(), "offline-worker", req); results <- err }()
	}
	success := 0
	for n := 0; n < 6; n++ {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("accepted %d starts", success)
	}
	assertCount(t, s, "SELECT count(*) FROM worker_offline_executions", 1)
}
func TestOfflineStoreRevocationExpiryAndIdentity(t *testing.T) {
	for _, kind := range []string{"foreign", "id", "profile", "recovery", "pause", "expired"} {
		t.Run(kind, func(t *testing.T) {
			s := integrationStore(t)
			req := offlineFixture(t, s)
			ctx := context.Background()
			p, err := s.StartOffline(ctx, "offline-worker", req)
			workerOK(t, err)
			token := p.Token
			subject := "offline-worker"
			switch kind {
			case "foreign":
				subject = "other"
			case "id":
				token.ID = strings.Repeat("a", 64)
			case "profile":
				token.ProfileDigest = sandbox.Hash([]byte("other"))
			case "recovery":
				_, err = s.BeginRecovery(token.RecoveryEpoch)
				workerOK(t, err)
			case "pause":
				workerSQL(t, s, "UPDATE sessions SET paused=true")
			case "expired":
				workerSQL(t, s, "UPDATE worker_offline_executions SET lease_until=clock_timestamp()-interval '1 second'")
			}
			if _, err = s.RenewOffline(ctx, subject, token); err == nil {
				t.Fatal("invalid renewal accepted")
			}
			if _, err = s.FinishOffline(ctx, subject, offline.Report{Token: token, Result: offlineResult(p)}); err == nil {
				t.Fatal("invalid finish accepted")
			}
			assertCount(t, s, "SELECT count(*) FROM worker_offline_executions WHERE receipt_json IS NULL", 1)
			workerOK(t, s.FailOffline(ctx, "offline-worker", p.Token))
			if _, err = s.StartOffline(ctx, "offline-worker", req); err == nil {
				t.Fatal("unknown execution replayed")
			}
		})
	}
}
func TestOfflineStoreMissingSchemaAndAuditFailClosed(t *testing.T) {
	for _, kind := range []string{"schema", "start-audit", "finish-audit"} {
		t.Run(kind, func(t *testing.T) {
			s := integrationStore(t)
			req := offlineFixture(t, s)
			ctx := context.Background()
			if kind == "schema" {
				workerSQL(t, s, "DELETE FROM core_schema_migrations WHERE version=5")
			}
			if kind == "start-audit" {
				workerSQL(t, s, "DELETE FROM audit_journal_state")
			}
			if kind != "finish-audit" {
				if _, err := s.StartOffline(ctx, "offline-worker", req); err == nil {
					t.Fatal("missing prerequisite accepted")
				}
				assertCount(t, s, "SELECT count(*) FROM worker_offline_executions", 0)
				return
			}
			p, err := s.StartOffline(ctx, "offline-worker", req)
			workerOK(t, err)
			workerSQL(t, s, "DELETE FROM audit_journal_state")
			if _, err = s.FinishOffline(ctx, "offline-worker", offline.Report{Token: p.Token, Result: offlineResult(p)}); err == nil {
				t.Fatal("missing audit accepted")
			}
			assertCount(t, s, "SELECT count(*) FROM worker_offline_executions WHERE state='AUTHORIZED' AND receipt_json IS NULL", 1)
		})
	}
}
func TestOfflineStoreJournalWaitCannotReviveLease(t *testing.T) {
	s := integrationStore(t)
	req := offlineFixture(t, s)
	ctx := context.Background()
	p, err := s.StartOffline(ctx, "offline-worker", req)
	workerOK(t, err)
	blocker, err := s.pool.Begin(ctx)
	workerOK(t, err)
	defer rollbackOutbox(blocker)
	_, err = blocker.Exec(ctx, "SELECT singleton_id FROM audit_journal_state FOR UPDATE")
	workerOK(t, err)
	workerSQL(t, s, "UPDATE worker_offline_executions SET lease_until=clock_timestamp()+interval '200 milliseconds'")
	done := make(chan error, 1)
	go func() {
		_, err := s.FinishOffline(ctx, "offline-worker", offline.Report{Token: p.Token, Result: offlineResult(p)})
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	workerOK(t, blocker.Commit(ctx))
	if err = <-done; !errors.Is(err, workerqueue.ErrLease) {
		t.Fatalf("expired finish: %v", err)
	}
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.offline.finished'", 0)
}
func TestOfflineStoreFrozenProfileRejectsDrift(t *testing.T) {
	s := integrationStore(t)
	req := offlineFixture(t, s)
	req.Profile.Argv = []string{"/different"}
	if _, err := s.StartOffline(context.Background(), "offline-worker", req); !errors.Is(err, workerqueue.ErrIdentity) {
		t.Fatalf("frozen profile drift: %v", err)
	}
}
