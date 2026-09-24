package authority

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
)

type countingProvider struct {
	calls int
}

func (p *countingProvider) Dispatch(_ context.Context, _ action.Request) (action.DispatchResult, error) {
	p.calls++
	return action.DispatchResult{Outcome: action.DispatchConfirmed, ExternalRef: "provider-op"}, nil
}

func (p *countingProvider) Reconcile(_ context.Context, _ action.Operation) (action.ReconcileResult, error) {
	return action.ReconcileResult{Outcome: action.ReconcileConfirmed}, nil
}

func createAuthorityExecution(t *testing.T, s *store.Memory) uint64 {
	t.Helper()
	input := core.RunInputManifest{
		RunID:              "run-1",
		TaskContractDigest: "sha256:task",
		RuntimeProfile:     "codex/default",
		ToolProfile:        "tools/m1",
		WorkerProfile:      "worker/ubuntu",
		PolicyProfile:      "policy/m1",
	}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	value := run.New(input.RunID, input.TaskContractDigest, inputDigest)
	attempt, err := value.StartAttempt("attempt-1", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(value.ID, attempt.Epoch)
	if err := s.CreateExecution(*value, *sess, input); err != nil {
		t.Fatal(err)
	}
	return attempt.Epoch
}

func TestActionServiceUsesRealRunAndRecoveryAuthority(t *testing.T) {
	s := store.NewMemory()
	executionEpoch := createAuthorityExecution(t, s)
	guard := New(s)
	provider := &countingProvider{}
	svc := action.NewService(
		action.AllowCapabilities{
			"device.flash": true,
			"git.read":     true,
		},
		guard,
		provider,
		s,
	)

	_, err := svc.Execute(context.Background(), action.Request{
		ID:               "op-normal",
		RunID:            "run-1",
		ExecutionEpoch:   executionEpoch,
		RecoveryEpoch:    0,
		Action:           "device.flash",
		RiskClass:        action.HighRisk,
		Capability:       "device.flash",
		ParametersDigest: "sha256:flash",
		IdempotencyKey:   "idem-normal",
		RequestedBy:      "runtime",
	})
	if err != nil {
		t.Fatalf("normal high-risk action should dispatch: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one provider call, got %d", provider.calls)
	}

	active, err := s.BeginRecovery(0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Execute(context.Background(), action.Request{
		ID:               "op-blocked",
		RunID:            "run-1",
		ExecutionEpoch:   executionEpoch,
		RecoveryEpoch:    active.Epoch,
		Action:           "device.flash",
		RiskClass:        action.HighRisk,
		Capability:       "device.flash",
		ParametersDigest: "sha256:flash2",
		IdempotencyKey:   "idem-blocked",
		RequestedBy:      "runtime",
	})
	if !errors.Is(err, recovery.ErrRecoveryMode) {
		t.Fatalf("expected recovery mode to block high-risk action, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("blocked action must not reach provider, got %d calls", provider.calls)
	}

	_, err = svc.Execute(context.Background(), action.Request{
		ID:               "op-observe",
		RunID:            "run-1",
		ExecutionEpoch:   executionEpoch,
		RecoveryEpoch:    active.Epoch,
		Action:           "git.read",
		RiskClass:        action.Observe,
		Capability:       "git.read",
		ParametersDigest: "sha256:read",
		IdempotencyKey:   "idem-observe",
		RequestedBy:      "runtime",
	})
	if err != nil {
		t.Fatalf("observe action should remain available during recovery: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected observe provider call, got %d", provider.calls)
	}

	_, err = svc.Execute(context.Background(), action.Request{
		ID:               "op-stale-recovery",
		RunID:            "run-1",
		ExecutionEpoch:   executionEpoch,
		RecoveryEpoch:    0,
		Action:           "git.read",
		RiskClass:        action.Observe,
		Capability:       "git.read",
		ParametersDigest: "sha256:read2",
		IdempotencyKey:   "idem-stale",
		RequestedBy:      "runtime",
	})
	if !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatalf("expected stale recovery epoch rejection, got %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("stale recovery request must not reach provider, got %d calls", provider.calls)
	}
}
