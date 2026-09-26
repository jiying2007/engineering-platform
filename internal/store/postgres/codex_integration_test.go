package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

func codexFixture(t *testing.T, s *Store) codexexec.Start {
	t.Helper()
	ctx := context.Background()
	profile := codexexec.Profile{
		Version: 1, CodexVersion: codexapp.QualifiedCodexVersion,
		BinaryDigest:            "sha256:" + strings.Repeat("a", 64),
		EngineeringConfigDigest: codexapp.EngineeringConfigDigest(),
		Model:                   "gpt-test", Sandbox: "workspace-write", ApprovalPolicy: "never",
	}
	pd, err := profile.Digest()
	workerOK(t, err)
	work := core.WorkItem{ID: "codex-work", Title: "codex fixture", HumanOwner: "fixture", State: core.WorkDraft, Version: 1, CreatedAt: time.Now().UTC()}
	workerOK(t, s.CreateWork(work))
	plan := verification.Plan{ID: "codex-plan", Criteria: []verification.Criterion{{ID: "ac", Statement: "change", Requirements: []verification.EvidenceRequirement{{ID: "req", Procedure: "test"}}}}}
	planDigest, err := plan.Digest()
	workerOK(t, err)
	task := core.TaskContract{
		ID: "codex-task", WorkItemID: work.ID, TaskType: "FEATURE", Repository: "repo",
		BaseCommit: strings.Repeat("1", 40), AcceptanceCriteria: []string{"change source"},
		AllowedActions: []string{codexexec.Action}, ExpectedOutputs: []string{"source change"},
		VerificationPlanID: plan.ID, VerificationPlanDigest: planDigest, Revision: 1,
	}
	taskDigest, err := task.Digest()
	workerOK(t, err)
	work.State = core.WorkReady
	work.ActiveTaskContractDigest = taskDigest
	workerOK(t, s.CreateTaskAndUpdateWork(task, plan, 1, work))
	input := core.RunInputManifest{
		RunID: "codex-run", TaskContractDigest: taskDigest, RuntimeProfile: "codex/runtime",
		ToolProfile: "codex/" + pd, WorkerProfile: "worker/codex", PolicyProfile: "policy",
	}
	inputDigest, err := input.Digest()
	workerOK(t, err)
	value := run.New(input.RunID, taskDigest, inputDigest)
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
	assignment := *claimWorkerFixture(t, s, "codex-worker")
	inputReport, err := workerqueue.NewReport(assignment)
	workerOK(t, err)
	manifest := contextbundle.Manifest{SchemaVersion: 1, RunInputDigest: inputDigest, Entries: []contextbundle.Entry{}}
	raw, _ := json.Marshal(manifest)
	facts := preparation.Facts{
		Version: 1, IntentDigest: assignment.IntentDigest, InputDigest: inputDigest, TaskDigest: taskDigest,
		ApprovalDigest: canonical.BytesDigest([]byte("approval")), BaseCommit: task.BaseCommit,
		TreeCommit: strings.Repeat("2", 40), WorkspaceRecipe: workspace.Recipe,
		SourceDigest: canonical.BytesDigest([]byte("source")), ConfigDigest: canonical.BytesDigest([]byte("config")),
		BundleDigest: canonical.BytesDigest(raw), Context: manifest,
	}
	_, err = s.ReportPrepared(ctx, "codex-worker", preparation.Report{Input: inputReport, Facts: facts})
	workerOK(t, err)
	return codexexec.Start{RunID: input.RunID, WorkerProfile: input.WorkerProfile, Profile: profile}
}

func codexResult(t *testing.T, permit codexexec.Permit) codexexec.Result {
	t.Helper()
	promptIdentity, err := codexexec.PromptIdentityDigest(permit.Assignment, permit.Preparation)
	workerOK(t, err)
	modelReceipt := codexapp.EngineeringReceipt{
		SchemaVersion: 1, CLI: "codex-cli", Version: codexapp.QualifiedCodexVersion,
		BinaryDigest: permit.Profile.BinaryDigest, EngineeringConfigDigest: permit.Profile.EngineeringConfigDigest,
		CredentialMode: "workload_identity", FederationRuleID: "rule-test", Model: permit.Profile.Model,
		PromptDigest: canonical.BytesDigest([]byte("rendered prompt")), ThreadID: "thread", TurnID: "turn",
		TurnStatus: "completed", Output: "implemented", OutputDigest: canonical.BytesDigest([]byte("implemented")),
		CommandCount: 2, FileChangeCount: 1, AssertionRemovedBeforeTurn: true,
	}
	return codexexec.Result{
		PromptIdentityDigest: promptIdentity,
		Codex:                modelReceipt,
		Change: workspace.ChangeFacts{
			Recipe: workspace.FinalizeRecipe, BaseCommit: permit.Preparation.Facts.BaseCommit,
			BaseTree: permit.Preparation.Facts.TreeCommit, BaseSourceDigest: permit.Preparation.Facts.SourceDigest,
			ResultCommit: strings.Repeat("3", 40), ResultTree: strings.Repeat("4", 40),
			ResultSourceDigest: canonical.BytesDigest([]byte("result source")),
			BundleDigest:       canonical.BytesDigest([]byte("bundle")), BundleSize: 128,
		},
	}
}

func TestCodexStoreFreshGrantReplayAndMigration(t *testing.T) {
	s := integrationStore(t)
	req := codexFixture(t, s)
	ctx := context.Background()
	permit, err := s.StartCodex(ctx, "codex-worker", req)
	workerOK(t, err)
	workerOK(t, permit.Check("codex-worker", req))
	if _, err = s.StartCodex(ctx, "codex-worker", req); err == nil {
		t.Fatal("duplicate Codex start allowed")
	}
	result := codexResult(t, permit)
	report := codexexec.Report{Token: permit.Token, Result: result}
	first, err := s.FinishCodex(ctx, "codex-worker", report)
	workerOK(t, err)
	workerOK(t, first.Verify("codex-worker", permit, result))
	_, err = s.BeginRecovery(permit.Token.RecoveryEpoch)
	workerOK(t, err)
	second, err := s.FinishCodex(ctx, "codex-worker", report)
	workerOK(t, err)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatal("identical Codex report replay changed receipt")
	}
	report.Result.Codex.Output = "changed"
	report.Result.Codex.OutputDigest = canonical.BytesDigest([]byte("changed"))
	if _, err = s.FinishCodex(ctx, "codex-worker", report); !errors.Is(err, workerqueue.ErrIdentity) {
		t.Fatalf("altered Codex replay accepted: %v", err)
	}
	workerOK(t, s.ApplyCoreMigration(ctx))
	stored, err := s.GetCodex(ctx, req.RunID)
	workerOK(t, err)
	b, _ = json.Marshal(stored.Receipt)
	if string(a) != string(b) {
		t.Fatal("migration changed Codex receipt")
	}
	assertCount(t, s, "SELECT count(*) FROM worker_codex_executions WHERE state='FINISHED'", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.codex.finished'", 1)
	assertCount(t, s, "SELECT count(*) FROM evidence", 0)
	assertCount(t, s, "SELECT count(*) FROM runs WHERE state='RUNNING'", 1)
}

func TestCodexStoreUnknownBlocksRecoveryAndReplay(t *testing.T) {
	s := integrationStore(t)
	req := codexFixture(t, s)
	ctx := context.Background()
	permit, err := s.StartCodex(ctx, "codex-worker", req)
	workerOK(t, err)
	workerOK(t, s.FailCodex(ctx, "codex-worker", permit.Token))
	if _, err := s.StartCodex(ctx, "codex-worker", req); err == nil {
		t.Fatal("UNKNOWN Codex execution was replayed")
	}
	recoveryState, err := s.BeginRecovery(permit.Token.RecoveryEpoch)
	workerOK(t, err)
	if _, err := s.CreateRecoveryProof(ctx, recoveryState.Epoch, "reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("UNKNOWN Codex execution did not block recovery proof: %v", err)
	}
}

func TestCodexStoreLeaseAndIdentityFailClosed(t *testing.T) {
	for _, kind := range []string{"foreign", "profile", "recovery", "pause", "expired"} {
		t.Run(kind, func(t *testing.T) {
			s := integrationStore(t)
			req := codexFixture(t, s)
			ctx := context.Background()
			permit, err := s.StartCodex(ctx, "codex-worker", req)
			workerOK(t, err)
			token := permit.Token
			subject := "codex-worker"
			switch kind {
			case "foreign":
				subject = "other"
			case "profile":
				token.ProfileDigest = canonical.BytesDigest([]byte("other"))
			case "recovery":
				_, err = s.BeginRecovery(token.RecoveryEpoch)
				workerOK(t, err)
			case "pause":
				workerSQL(t, s, "UPDATE sessions SET paused=true")
			case "expired":
				workerSQL(t, s, "UPDATE worker_codex_executions SET lease_until=clock_timestamp()-interval '1 second'")
			}
			if _, err = s.RenewCodex(ctx, subject, token); err == nil {
				t.Fatal("invalid Codex renewal accepted")
			}
			if _, err = s.FinishCodex(ctx, subject, codexexec.Report{Token: token, Result: codexResult(t, permit)}); err == nil {
				t.Fatal("invalid Codex finish accepted")
			}
			assertCount(t, s, "SELECT count(*) FROM worker_codex_executions WHERE receipt_json IS NULL", 1)
		})
	}
}
