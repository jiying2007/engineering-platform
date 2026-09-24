package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

func setupAuthority(t *testing.T, allowed []string) (*Authority, *store.Memory, *run.Run) {
	t.Helper()
	s := store.NewMemory()
	work := core.WorkItem{
		ID:         "work-1",
		Title:      "test",
		HumanOwner: "owner",
		State:      core.WorkReady,
		Version:    1,
		CreatedAt:  time.Unix(1, 0),
	}
	if err := s.CreateWork(work); err != nil {
		t.Fatal(err)
	}
	plan := verification.Plan{
		ID: "vp-1",
		Criteria: []verification.Criterion{{
			ID:        "ac-1",
			Statement: "tests pass",
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
		ID:                     "task-1",
		WorkItemID:             work.ID,
		TaskType:               "FEATURE",
		Repository:             "repo",
		BaseCommit:             "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria:     []string{"tests pass"},
		AllowedActions:         allowed,
		VerificationPlanID:     plan.ID,
		VerificationPlanDigest: planDigest,
		Revision:               1,
	}
	if err := s.CreateTask(task, plan); err != nil {
		t.Fatal(err)
	}
	taskDigest, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	input := core.RunInputManifest{
		RunID:              "run-1",
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
	value := run.New(input.RunID, taskDigest, inputDigest)
	attempt, err := value.StartAttempt("attempt-1", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(value.ID, attempt.Epoch)
	if err := s.CreateExecution(*value, *sess, input); err != nil {
		t.Fatal(err)
	}
	return NewAuthority(s), s, value
}

func TestFrozenTaskAllowedActionIsRequired(t *testing.T) {
	auth, _, value := setupAuthority(t, []string{"ci.dispatch"})
	ctx := context.Background()

	allowed := action.Request{
		RunID:          value.ID,
		ExecutionEpoch: value.CurrentEpoch,
		RecoveryEpoch:  0,
		Action:         "ci.dispatch",
		Capability:     "ci.dispatch",
		RiskClass:      action.ControlledMutation,
	}
	if err := auth.Authorize(ctx, allowed); err != nil {
		t.Fatalf("expected allowed action: %v", err)
	}

	denied := allowed
	denied.Action = "device.flash"
	denied.Capability = "device.flash"
	if err := auth.Authorize(ctx, denied); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("expected frozen task action denial, got %v", err)
	}
}

func TestRunEpochGuardRejectsStaleRuntime(t *testing.T) {
	auth, _, value := setupAuthority(t, []string{"ci.dispatch"})
	if err := auth.CheckRunEpoch(context.Background(), value.ID, value.CurrentEpoch+1); !errors.Is(err, run.ErrStaleEpoch) {
		t.Fatalf("expected stale run epoch, got %v", err)
	}
}

func TestRecoveryModeAllowsObserveButBlocksMutations(t *testing.T) {
	auth, state, _ := setupAuthority(t, []string{"device.read", "device.flash"})
	active, err := state.BeginRecovery(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := auth.CheckRecoveryEpoch(ctx, active.Epoch, action.Observe); err != nil {
		t.Fatalf("observe should remain available during reconciliation: %v", err)
	}
	if err := auth.CheckRecoveryEpoch(ctx, active.Epoch, action.ControlledMutation); !errors.Is(err, recovery.ErrRecoveryMode) {
		t.Fatalf("controlled mutation should be blocked: %v", err)
	}
	if err := auth.CheckRecoveryEpoch(ctx, active.Epoch, action.HighRisk); !errors.Is(err, recovery.ErrRecoveryMode) {
		t.Fatalf("high-risk mutation should be blocked: %v", err)
	}
}

func TestRecoveryEpochMustMatchEvenForObserve(t *testing.T) {
	auth, state, _ := setupAuthority(t, []string{"device.read"})
	active, err := state.BeginRecovery(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.CheckRecoveryEpoch(context.Background(), active.Epoch-1, action.Observe); !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatalf("expected stale recovery epoch, got %v", err)
	}
}
