package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/gateway"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type postgresActionProvider struct {
	dispatchCalls  int
	reconcileCalls int
	dispatch       action.DispatchResult
	dispatchErr    error
	reconcile      action.ReconcileResult
	reconcileErr   error
}

func (p *postgresActionProvider) Dispatch(context.Context, action.Request) (action.DispatchResult, error) {
	p.dispatchCalls++
	return p.dispatch, p.dispatchErr
}

func (p *postgresActionProvider) Reconcile(context.Context, action.Operation) (action.ReconcileResult, error) {
	p.reconcileCalls++
	return p.reconcile, p.reconcileErr
}

func TestPostgresActionRepositoryAndService(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	runID := setupPostgresActionRun(t, s, suffix)

	recoveryState, err := s.GetRecovery()
	if err != nil {
		t.Fatal(err)
	}

	provider := &postgresActionProvider{
		dispatch: action.DispatchResult{
			Outcome:       action.DispatchConfirmed,
			ExternalRef:   "ci-confirmed-" + suffix,
			ObservedState: "queued",
		},
		reconcile: action.ReconcileResult{
			Outcome:       action.ReconcileConfirmed,
			ExternalRef:   "ci-reconciled-" + suffix,
			ObservedState: "completed",
		},
	}
	authority := gateway.NewAuthority(s)
	service := action.NewService(authority, authority, provider, s)

	req := action.Request{
		ID:               "action-confirmed-" + suffix,
		RunID:            runID,
		ExecutionEpoch:   1,
		RecoveryEpoch:    recoveryState.Epoch,
		Action:           "ci.dispatch",
		RiskClass:        action.ControlledMutation,
		Capability:       "ci.dispatch",
		ParametersDigest: "sha256:params-confirmed",
		IdempotencyKey:   "idem-confirmed-" + suffix,
		RequestedBy:      "runtime",
		RequestedAt:      time.Now().UTC(),
	}
	first, err := service.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Result != string(action.Confirmed) {
		t.Fatalf("expected CONFIRMED receipt, got %#v", first)
	}
	second, err := service.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if provider.dispatchCalls != 1 {
		t.Fatalf("idempotent retry dispatched %d times", provider.dispatchCalls)
	}
	if second.OperationID != first.OperationID || second.Result != string(action.Confirmed) {
		t.Fatalf("unexpected idempotent retry receipt: %#v", second)
	}
	stored, err := s.Get(req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != action.Confirmed || stored.ExternalRef == "" {
		t.Fatalf("unexpected stored action operation: %#v", stored)
	}

	provider.dispatchErr = errors.New("provider response lost")
	provider.dispatch = action.DispatchResult{}
	unknownReq := action.Request{
		ID:               "action-unknown-" + suffix,
		RunID:            runID,
		ExecutionEpoch:   1,
		RecoveryEpoch:    recoveryState.Epoch,
		Action:           "ci.dispatch",
		RiskClass:        action.ControlledMutation,
		Capability:       "ci.dispatch",
		ParametersDigest: "sha256:params-unknown",
		IdempotencyKey:   "idem-unknown-" + suffix,
		RequestedBy:      "runtime",
		RequestedAt:      time.Now().UTC(),
	}
	unknownReceipt, err := service.Execute(ctx, unknownReq)
	if err != nil {
		t.Fatal(err)
	}
	if unknownReceipt.Result != string(action.Unknown) {
		t.Fatalf("expected UNKNOWN receipt, got %#v", unknownReceipt)
	}

	provider.dispatchErr = nil
	reconciled, err := service.Reconcile(ctx, unknownReq.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Result != string(action.Confirmed) {
		t.Fatalf("expected reconciled CONFIRMED receipt, got %#v", reconciled)
	}
	if provider.reconcileCalls != 1 {
		t.Fatalf("expected one reconciliation call, got %d", provider.reconcileCalls)
	}

	var auditCount int
	if err := s.pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM audit_events WHERE aggregate_type='ExternalOperation' AND aggregate_id IN ($1,$2)",
		req.ID,
		unknownReq.ID,
	).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 7 {
		t.Fatalf("expected persisted action transition audit trail, got %d events", auditCount)
	}
}

func setupPostgresActionRun(t *testing.T, s *Store, suffix string) string {
	t.Helper()

	work := core.WorkItem{
		ID:         "work-action-pg-" + suffix,
		Title:      "PostgreSQL action integration",
		HumanOwner: "integration-test",
		State:      core.WorkDraft,
		Version:    1,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.CreateWork(work); err != nil {
		t.Fatal(err)
	}

	plan := verification.Plan{
		ID: "vp-action-pg-" + suffix,
		Criteria: []verification.Criterion{{
			ID:        "ac-1",
			Statement: "CI passes",
			Requirements: []verification.EvidenceRequirement{{
				ID:        "req-1",
				Procedure: "ci.test",
			}},
		}},
	}
	planDigest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{
		ID:                     "task-action-pg-" + suffix,
		WorkItemID:             work.ID,
		TaskType:               "FEATURE",
		Repository:             "repo",
		BaseCommit:             "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria:     []string{"CI passes"},
		AllowedActions:         []string{"ci.dispatch"},
		VerificationPlanID:     plan.ID,
		VerificationPlanDigest: planDigest,
		Revision:               1,
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
	if err := s.CreateTaskAndUpdateWork(task, plan, work.Version, ready); err != nil {
		t.Fatal(err)
	}
	ready, err = s.GetWork(work.ID)
	if err != nil {
		t.Fatal(err)
	}

	runID := "run-action-pg-" + suffix
	input := core.RunInputManifest{
		RunID:              runID,
		TaskContractDigest: taskDigest,
		RuntimeProfile:     "codex/default",
		ToolProfile:        "tools/m1",
		WorkerProfile:      "worker/ubuntu",
		PolicyProfile:      "policy/m1",
	}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	value := run.New(runID, taskDigest, inputDigest)
	attempt, err := value.StartAttempt("attempt-1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(runID, attempt.Epoch)
	executing := ready
	executing.ActiveRunID = runID
	if err := executing.Transition(core.WorkExecuting); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateExecutionAndUpdateWork(
		*value,
		attempt,
		*sess,
		input,
		ready.Version,
		executing,
	); err != nil {
		t.Fatal(err)
	}
	return runID
}
