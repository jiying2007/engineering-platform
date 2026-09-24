package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/gateway"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type apiActionProvider struct {
	dispatchCalls  int
	reconcileCalls int
	dispatch       action.DispatchResult
	dispatchErr    error
	reconcile      action.ReconcileResult
	reconcileErr   error
}

func (p *apiActionProvider) Dispatch(context.Context, action.Request) (action.DispatchResult, error) {
	p.dispatchCalls++
	return p.dispatch, p.dispatchErr
}

func (p *apiActionProvider) Reconcile(context.Context, action.Operation) (action.ReconcileResult, error) {
	p.reconcileCalls++
	return p.reconcile, p.reconcileErr
}

func actionServer(t *testing.T, allowed []string, provider *apiActionProvider) (http.Handler, *store.Memory) {
	t.Helper()
	memory := store.NewMemory()
	authority := gateway.NewAuthority(memory)
	service := action.NewService(authority, authority, provider, memory)
	server := NewServerWithActionGateway(memory, service)
	h := server.Handler()

	mustRequest(t, h, http.MethodPost, "/api/v1/work-items", map[string]any{
		"work_item_id": "work-action",
		"title":        "action test",
		"human_owner":  "owner",
	}, http.StatusCreated)

	taskBody := mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", map[string]any{
		"contract": map[string]any{
			"task_contract_id": "task-action",
			"work_item_id":     "work-action",
			"task_type":        "FEATURE",
			"allowed_actions":  allowed,
		},
		"material": map[string]any{
			"repository":          "repo",
			"base_commit":         "0123456789abcdef0123456789abcdef01234567",
			"target_id":           "target-1",
			"acceptance_criteria": []string{"tests pass"},
		},
		"subsystem": "driver",
		"verification_plan": map[string]any{
			"verification_plan_id": "vp-action",
			"criteria": []any{
				map[string]any{
					"criterion_id": "ac-1",
					"statement":    "tests pass",
					"evidence_requirements": []any{
						map[string]any{
							"requirement_id": "req-1",
							"procedure":      "ci.test",
						},
					},
				},
			},
		},
	}, http.StatusCreated)

	var taskResponse struct {
		Digest string `json:"digest"`
	}
	mustJSON(t, taskBody, &taskResponse)
	if taskResponse.Digest == "" {
		t.Fatal("expected task digest")
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-action",
		"task_contract_digest": taskResponse.Digest,
		"attempt_id":           "attempt-action",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)

	return h, memory
}

func TestActionAPIFrozenAuthorizationAndIdempotency(t *testing.T) {
	provider := &apiActionProvider{
		dispatch: action.DispatchResult{
			Outcome:       action.DispatchConfirmed,
			ExternalRef:   "ci-42",
			ObservedState: "queued",
		},
	}
	h, _ := actionServer(t, []string{"ci.dispatch"}, provider)

	body := map[string]any{
		"action_request_id": "act-1",
		"execution_epoch":   1,
		"recovery_epoch":    0,
		"action":            "ci.dispatch",
		"risk_class":        "CONTROLLED_MUTATION",
		"capability":        "ci.dispatch",
		"parameters_digest": "sha256:params",
		"idempotency_key":   "idem-1",
		"requested_by":      "runtime",
	}
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", body, http.StatusAccepted)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", body, http.StatusAccepted)
	if provider.dispatchCalls != 1 {
		t.Fatalf("idempotent retry dispatched %d times", provider.dispatchCalls)
	}

	mustRequest(t, h, http.MethodGet, "/api/v1/actions/act-1", nil, http.StatusOK)

	denied := map[string]any{
		"action_request_id": "act-denied",
		"execution_epoch":   1,
		"recovery_epoch":    0,
		"action":            "device.flash",
		"risk_class":        "HIGH_RISK",
		"capability":        "device.flash",
		"parameters_digest": "sha256:flash",
		"idempotency_key":   "idem-denied",
		"requested_by":      "runtime",
	}
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", denied, http.StatusForbidden)
	if provider.dispatchCalls != 1 {
		t.Fatalf("denied action reached provider; calls=%d", provider.dispatchCalls)
	}
}

func TestActionAPIRecoveryModeAllowsObserveAndBlocksMutation(t *testing.T) {
	provider := &apiActionProvider{
		dispatch: action.DispatchResult{Outcome: action.DispatchConfirmed},
	}
	h, _ := actionServer(t, []string{"device.read", "device.flash"}, provider)

	recoveryBody := mustRequest(t, h, http.MethodPost, "/api/v1/recovery/begin", map[string]any{
		"expected_recovery_epoch": 0,
	}, http.StatusOK)
	var recoveryState struct {
		Epoch uint64 `json:"recovery_epoch"`
	}
	mustJSON(t, recoveryBody, &recoveryState)
	if recoveryState.Epoch != 1 {
		t.Fatalf("expected recovery epoch 1, got %d", recoveryState.Epoch)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", map[string]any{
		"action_request_id": "observe-1",
		"execution_epoch":   1,
		"recovery_epoch":    recoveryState.Epoch,
		"action":            "device.read",
		"risk_class":        "OBSERVE",
		"capability":        "device.read",
		"parameters_digest": "sha256:read",
		"idempotency_key":   "idem-read",
		"requested_by":      "runtime",
	}, http.StatusAccepted)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", map[string]any{
		"action_request_id": "flash-1",
		"execution_epoch":   1,
		"recovery_epoch":    recoveryState.Epoch,
		"action":            "device.flash",
		"risk_class":        "HIGH_RISK",
		"capability":        "device.flash",
		"parameters_digest": "sha256:flash",
		"idempotency_key":   "idem-flash",
		"requested_by":      "runtime",
	}, http.StatusConflict)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", map[string]any{
		"action_request_id": "observe-stale",
		"execution_epoch":   1,
		"recovery_epoch":    0,
		"action":            "device.read",
		"risk_class":        "OBSERVE",
		"capability":        "device.read",
		"parameters_digest": "sha256:read2",
		"idempotency_key":   "idem-read2",
		"requested_by":      "runtime",
	}, http.StatusConflict)

	if provider.dispatchCalls != 1 {
		t.Fatalf("only the observe action should reach provider, calls=%d", provider.dispatchCalls)
	}
}

func TestActionAPIReconcilesUnknownOperation(t *testing.T) {
	provider := &apiActionProvider{
		dispatchErr: errors.New("response lost"),
		reconcile: action.ReconcileResult{
			Outcome:       action.ReconcileConfirmed,
			ExternalRef:   "ci-99",
			ObservedState: "completed",
		},
	}
	h, _ := actionServer(t, []string{"ci.dispatch"}, provider)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-action/actions", map[string]any{
		"action_request_id": "act-unknown",
		"execution_epoch":   1,
		"recovery_epoch":    0,
		"action":            "ci.dispatch",
		"risk_class":        "CONTROLLED_MUTATION",
		"capability":        "ci.dispatch",
		"parameters_digest": "sha256:params",
		"idempotency_key":   "idem-unknown",
		"requested_by":      "runtime",
	}, http.StatusAccepted)

	mustRequest(t, h, http.MethodPost, "/api/v1/actions/act-unknown/reconcile", nil, http.StatusOK)
	if provider.dispatchCalls != 1 || provider.reconcileCalls != 1 {
		t.Fatalf("unexpected provider calls dispatch=%d reconcile=%d", provider.dispatchCalls, provider.reconcileCalls)
	}
}

func TestActionAPIWithoutProviderFailsClosed(t *testing.T) {
	h := NewServer(store.NewMemory()).Handler()
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/anything/actions", map[string]any{}, http.StatusServiceUnavailable)
}
