package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type restoreStateCount struct {
	State string `json:"state"`
	Count uint64 `json:"count"`
}

type restoreSnapshot struct {
	Recovery           recovery.Manager    `json:"recovery"`
	WorkDigest         string              `json:"work_digest"`
	DeliveryDigest     string              `json:"delivery_digest"`
	EvidenceDigest     string              `json:"evidence_digest"`
	VerificationDigest string              `json:"verification_digest"`
	ReviewDigest       string              `json:"review_digest"`
	ClosureDigest      string              `json:"closure_digest"`
	OperationDigest    string              `json:"operation_digest"`
	ProofDigest        string              `json:"proof_digest"`
	AuditSequence      uint64              `json:"audit_sequence"`
	AuditDigest        string              `json:"audit_digest"`
	Migrations         []uint64            `json:"migrations"`
	Outbox             []restoreStateCount `json:"outbox"`
}

func TestPostgresAuthorityRestoreDrill(t *testing.T) {
	if os.Getenv("EP_RESTORE_DRILL") != "1" {
		t.Skip("EP_RESTORE_DRILL is not set")
	}
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		t.Fatal("POSTGRES_TEST_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	adminCfg, err := pgxpool.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	adminCfg.ConnConfig.Database = "postgres"
	admin, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	sourceDB, restoredDB := restoreDatabaseName(t, "source"), restoreDatabaseName(t, "restored")
	for _, name := range []string{sourceDB, restoredDB} {
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		for _, name := range []string{sourceDB, restoredDB} {
			_, _ = admin.Exec(cleanup, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		}
	})
	sourceURL := databaseURLWithName(t, base, sourceDB)
	restoredURL := databaseURLWithName(t, base, restoredDB)
	source, err := Open(ctx, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ApplyCoreMigration(ctx); err != nil {
		source.Close()
		t.Fatal(err)
	}
	seedRestoreAuthority(t, source)
	want, err := authoritySnapshot(ctx, source)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	wantDigest, err := canonical.Digest(want)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	source.Close()

	dumpDir := t.TempDir()
	dumpPath := filepath.Join(dumpDir, "authority.dump")
	runPG17(t, base, []string{"pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file=/dump/authority.dump", sourceDB}, dumpDir)
	if info, err := os.Stat(dumpPath); err != nil || info.Size() == 0 {
		t.Fatalf("pg_dump did not produce bytes: %v", err)
	}
	runPG17(t, base, []string{"pg_restore", "--no-owner", "--no-privileges", "--exit-on-error", "--dbname=" + restoredDB, "/dump/authority.dump"}, dumpDir)

	restored, err := Open(ctx, restoredURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := authoritySnapshot(ctx, restored)
	if err != nil {
		t.Fatal(err)
	}
	gotDigest, err := canonical.Digest(got)
	if err != nil {
		t.Fatal(err)
	}
	if gotDigest != wantDigest {
		t.Fatalf("restored authority snapshot mismatch\nwant=%s\n got=%s\nwant=%#v\n got=%#v", wantDigest, gotDigest, want, got)
	}
	if err := restored.AuthorizeCompletion(ctx, "urn:engineering-platform:operator:completer", 1); err != nil {
		t.Fatal(err)
	}
	state, err := restored.CompleteRecovery(1, true)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != recovery.Normal || state.Epoch != 1 {
		t.Fatalf("restored recovery proof could not complete recovery: %#v", state)
	}
}

func seedRestoreAuthority(t *testing.T, s *Store) {
	t.Helper()
	now := time.Unix(100, 0).UTC()
	work := core.WorkItem{ID: "restore-work", Title: "restore drill", HumanOwner: "owner", State: core.WorkDraft, Version: 1, CreatedAt: now}
	if err := s.CreateWork(work); err != nil {
		t.Fatal(err)
	}
	plan := verification.Plan{ID: "restore-plan", Criteria: []verification.Criterion{{ID: "criterion", Statement: "CI passes", Requirements: []verification.EvidenceRequirement{{ID: "ci", Procedure: "ci.test", Issuer: "ci"}}}}}
	planDigest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{ID: "restore-task", WorkItemID: work.ID, TaskType: "FEATURE", Repository: "repo", BaseCommit: strings.Repeat("a", 40), AcceptanceCriteria: []string{"CI passes"}, AllowedActions: []string{"ci.dispatch"}, VerificationPlanID: plan.ID, VerificationPlanDigest: planDigest, Revision: 1}
	taskDigest, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	ready := work
	ready.ActiveTaskContractDigest = taskDigest
	if err := ready.Transition(core.WorkReady); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTaskAndUpdateWork(task, plan, work.Version, ready); err != nil {
		t.Fatal(err)
	}
	ready, _ = s.GetWork(work.ID)
	input := core.RunInputManifest{RunID: "restore-run", TaskContractDigest: taskDigest, RuntimeProfile: "runtime", ToolProfile: "tool", WorkerProfile: "worker/restore", PolicyProfile: "policy"}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	value := run.New(input.RunID, taskDigest, inputDigest)
	attempt, err := value.StartAttempt("attempt", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(value.ID, attempt.Epoch)
	executing := ready
	executing.ActiveRunID = value.ID
	if err := executing.Transition(core.WorkExecuting); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateExecutionAndUpdateWork(*value, attempt, *sess, input, ready.Version, executing); err != nil {
		t.Fatal(err)
	}
	currentRun, currentSession, _ := s.GetExecution(value.ID)
	currentWork, _ := s.GetWork(work.ID)
	if err := currentRun.Complete(currentRun.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	verifying := currentWork
	if err := verifying.Transition(core.WorkVerifying); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExecutionAndWork(value.ID, currentRun.Version, currentRun, currentSession, currentWork.Version, verifying); err != nil {
		t.Fatal(err)
	}
	delivery := core.DeliveryReceipt{ID: "restore-delivery", WorkItemID: work.ID, TaskContractDigest: taskDigest, RunID: value.ID, BaseCommit: task.BaseCommit, ResultCommit: strings.Repeat("b", 40), CreatedAt: now.Add(2 * time.Second)}
	delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDelivery(delivery); err != nil {
		t.Fatal(err)
	}
	evidence := core.EvidenceRef{ID: "restore-evidence", DeliveryReceiptID: delivery.ID, RequirementID: "ci", SubjectDigest: delivery.SubjectDigest, Issuer: "ci", Procedure: "ci.test", Result: "PASS", Applicable: true}
	if err := s.CreateEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	verificationReport := verification.Evaluate(plan, delivery.SubjectDigest, []core.EvidenceRef{evidence})
	verificationReport.ID = "restore-verification"
	verificationReport.DeliveryReceiptID = delivery.ID
	verificationReport.VerificationPlanDigest = planDigest
	verificationReport.Verifier = "verifier"
	verificationReport.CreatedAt = now.Add(3 * time.Second)
	if err := s.CreateVerification(verificationReport); err != nil {
		t.Fatal(err)
	}
	verifying, _ = s.GetWork(work.ID)
	reviewReport := review.Report{ID: "restore-review", DeliveryReceiptID: delivery.ID, VerificationReportID: verificationReport.ID, TaskContractDigest: taskDigest, SubjectDigest: delivery.SubjectDigest, Reviewer: "reviewer", Result: review.ResultPass, CreatedAt: now.Add(4 * time.Second)}
	reviewing := verifying
	if err := reviewing.Transition(core.WorkReviewing); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateReviewAndUpdateWork(reviewReport, verifying.Version, reviewing); err != nil {
		t.Fatal(err)
	}
	reviewing, _ = s.GetWork(work.ID)
	closure := core.ClosureReceipt{ID: "restore-closure", WorkItemID: work.ID, TaskContractDigest: taskDigest, RunID: value.ID, DeliveryReceiptID: delivery.ID, VerificationReportID: verificationReport.ID, ReviewReportID: reviewReport.ID, SubjectDigest: delivery.SubjectDigest, Result: "CLOSED", CreatedAt: now.Add(5 * time.Second)}
	closed := reviewing
	if err := closed.Transition(core.WorkClosed); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateClosureAndUpdateWork(closure, reviewing.Version, closed); err != nil {
		t.Fatal(err)
	}
	req := action.Request{ID: "restore-operation", RunID: value.ID, ExecutionEpoch: 1, RecoveryEpoch: 0, Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci", ParametersDigest: "sha256:" + strings.Repeat("c", 64), IdempotencyKey: "restore-operation", RequestedBy: "runtime", RequestedAt: now.Add(6 * time.Second)}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("d", 64), now.Add(6*time.Second))
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(action.Dispatched, now.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(*op); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(action.Confirmed, now.Add(8*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); err != nil {
		t.Fatal(err)
	}
}

func authoritySnapshot(ctx context.Context, s *Store) (restoreSnapshot, error) {
	var snapshot restoreSnapshot
	var err error
	if snapshot.Recovery, err = s.GetRecovery(); err != nil {
		return snapshot, err
	}
	work, err := s.GetWork("restore-work")
	if err != nil {
		return snapshot, err
	}
	snapshot.WorkDigest, _ = canonical.Digest(work)
	delivery, err := s.GetDelivery("restore-delivery")
	if err != nil {
		return snapshot, err
	}
	snapshot.DeliveryDigest, _ = canonical.Digest(delivery)
	evidence, err := s.GetEvidence("restore-evidence")
	if err != nil {
		return snapshot, err
	}
	snapshot.EvidenceDigest, _ = canonical.Digest(evidence)
	verificationReport, err := s.GetVerification("restore-verification")
	if err != nil {
		return snapshot, err
	}
	snapshot.VerificationDigest, _ = canonical.Digest(verificationReport)
	reviewReport, err := s.GetReview("restore-review")
	if err != nil {
		return snapshot, err
	}
	snapshot.ReviewDigest, _ = reviewReport.Digest()
	closure, err := s.GetClosure("restore-closure")
	if err != nil {
		return snapshot, err
	}
	snapshot.ClosureDigest, _ = canonical.Digest(closure)
	op, err := s.Get("restore-operation")
	if err != nil {
		return snapshot, err
	}
	snapshot.OperationDigest, _ = canonical.Digest(op)
	proof, err := s.GetRecoveryProof(ctx, 1)
	if err != nil {
		return snapshot, err
	}
	snapshot.ProofDigest, _ = proof.Digest()
	if err := s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&snapshot.AuditSequence, &snapshot.AuditDigest); err != nil {
		return snapshot, err
	}
	rows, err := s.pool.Query(ctx, "SELECT version FROM core_schema_migrations ORDER BY version")
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var version uint64
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return snapshot, err
		}
		snapshot.Migrations = append(snapshot.Migrations, version)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, "SELECT state,count(*) FROM outbox_events GROUP BY state ORDER BY state")
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var item restoreStateCount
		if err := rows.Scan(&item.State, &item.Count); err != nil {
			rows.Close()
			return snapshot, err
		}
		snapshot.Outbox = append(snapshot.Outbox, item)
	}
	rows.Close()
	return snapshot, rows.Err()
}

func restoreDatabaseName(t *testing.T, suffix string) string {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	return "ep_restore_" + suffix + "_" + hex.EncodeToString(nonce[:])
}

func databaseURLWithName(t *testing.T, raw, name string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

func runPG17(t *testing.T, raw string, args []string, mount string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	user := parsed.User.Username()
	password, _ := parsed.User.Password()
	command := []string{"run", "--rm", "--network", "host", "-e", "PGPASSWORD=" + password, "-v", mount + ":/dump", "postgres:17-alpine"}
	command = append(command, args[0], "-h", host, "-p", port, "-U", user)
	command = append(command, args[1:]...)
	cmd := exec.Command("docker", command...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PostgreSQL 17 %s failed: %v\n%s", args[0], err, output)
	}
}
