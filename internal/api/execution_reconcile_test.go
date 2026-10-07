package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type reconciliationMemory struct {
	*store.Memory
	calls      int
	reconciler string
	request    recovery.ExecutionAbandonRequest
}

func (m *reconciliationMemory) ReconcileExecutionAbandoned(_ context.Context, reconciler string, request recovery.ExecutionAbandonRequest) (recovery.ExecutionAbandonReceipt, error) {
	m.calls++
	m.reconciler, m.request = reconciler, request
	digest, err := request.Digest()
	if err != nil {
		return recovery.ExecutionAbandonReceipt{}, err
	}
	return recovery.ExecutionAbandonReceipt{
		Kind: recovery.ExecutionAbandonmentKind, Request: request, RequestDigest: digest,
		Reconciler: reconciler, PreviousState: "UNKNOWN", CreatedAt: time.Now().UTC(),
	}, nil
}

func TestExecutionAbandonRequiresDedicatedReconcilerIdentity(t *testing.T) {
	backend := &reconciliationMemory{Memory: store.NewMemory()}
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{})
	base := server.URL + "/api/v1/recovery/executions/abandon"
	request := recovery.ExecutionAbandonRequest{
		Version: 1, ExecutionKind: recovery.ExecutionOffline, RunID: "run-recovery",
		ExecutionID: strings.Repeat("a", 64), RecoveryEpoch: 1,
		ObservationDigest: "sha256:" + strings.Repeat("b", 64), Disposition: recovery.AbandonNoReplay,
	}
	secureCall(t, pki.Client(t, engineerSubject), base, request, http.StatusForbidden)
	if backend.calls != 0 {
		t.Fatal("non-reconciler reached store")
	}
	body := map[string]any{
		"version": 1, "execution_kind": recovery.ExecutionOffline, "run_id": "run-recovery",
		"execution_id": strings.Repeat("a", 64), "recovery_epoch": 1,
		"observation_digest": "sha256:" + strings.Repeat("b", 64),
		"disposition":        recovery.AbandonNoReplay, "reconciler": engineerSubject,
	}
	secureCall(t, pki.Client(t, reconcilerSubject), base, body, http.StatusBadRequest)
	raw := secureCall(t, pki.Client(t, reconcilerSubject), base, request, http.StatusCreated)
	var receipt recovery.ExecutionAbandonReceipt
	mustJSON(t, raw, &receipt)
	if backend.calls != 1 || backend.reconciler != reconcilerSubject ||
		receipt.Reconciler != reconcilerSubject || receipt.ReplayAuthorized || receipt.ExecutionAuthorized {
		t.Fatal("reconciliation identity/authority drift", backend.calls, backend.reconciler, receipt)
	}
}
