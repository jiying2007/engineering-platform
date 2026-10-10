package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type plannedReconcileMockStore struct {
	*store.Memory
	calls   int
	subject string
	receipt recovery.ActionPlannedAbandonReceipt
}

func (s *plannedReconcileMockStore) ReconcilePlannedActionAbandoned(_ context.Context, subject string, request recovery.ActionPlannedAbandonRequest) (recovery.ActionPlannedAbandonReceipt, error) {
	s.calls++
	s.subject = subject
	digest, _ := request.Digest()
	s.receipt = recovery.ActionPlannedAbandonReceipt{
		Kind: recovery.ActionPlannedAbandonmentKind, Request: request,
		RequestDigest: digest, Reconciler: subject, PreviousState: "PLANNED",
		CreatedAt: time.Now().UTC(),
	}
	return s.receipt, nil
}
func (s *plannedReconcileMockStore) GetPlannedActionAbandonReceipt(_ context.Context, operationID string) (recovery.ActionPlannedAbandonReceipt, error) {
	if s.receipt.Request.OperationID != operationID {
		return recovery.ActionPlannedAbandonReceipt{}, store.ErrNotFound
	}
	return s.receipt, nil
}

func plannedActionAPIFixture() recovery.ActionPlannedAbandonRequest {
	return recovery.ActionPlannedAbandonRequest{
		Version: 1, OperationID: "op-test", RunID: "run-test",
		ExecutionEpoch: 1, OriginalRecoveryEpoch: 0, RecoveryEpoch: 1,
		IdempotencyKey: "frozen-key",
		OriginalRequestDigest: "sha256:" + strings.Repeat("a", 64),
		ObservationDigest: "sha256:" + strings.Repeat("b", 64),
		Disposition: recovery.AbandonNoReplay,
	}
}

func TestPlannedActionRecoveryMTLSAdmissionAndReadback(t *testing.T) {
	mock := &plannedReconcileMockStore{Memory: store.NewMemory()}
	server, pki := securedTestServer(t, mock, nil, AuthenticatedOptions{})
	post := server.URL + "/api/v1/recovery/actions/abandon-planned"
	get := server.URL + "/api/v1/recovery/actions/op-test/abandon-planned"
	request := plannedActionAPIFixture()

	secureCall(t, pki.Client(t, engineerSubject), post, request, http.StatusForbidden)
	secureCall(t, pki.Client(t, recoverySubject), post, request, http.StatusForbidden)
	bad := request
	bad.Disposition = "ALLOW_REPLAY"
	secureCall(t, pki.Client(t, reconcilerSubject), post, bad, http.StatusBadRequest)
	secureRaw(t, pki.Client(t, reconcilerSubject), http.MethodPost, post,
		[]byte(`{"version":1,"version":1}`), http.StatusBadRequest)
	secureRaw(t, pki.Client(t, recoverySubject), http.MethodGet, get, nil, http.StatusForbidden)
	secureRaw(t, pki.Client(t, reconcilerSubject), http.MethodGet, get, nil, http.StatusNotFound)
	if mock.calls != 0 {
		t.Fatalf("denied requests crossed authorized Recovery boundary: %d", mock.calls)
	}
	raw := secureCall(t, pki.Client(t, reconcilerSubject), post, request, http.StatusCreated)
	var receipt recovery.ActionPlannedAbandonReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil || receipt.Validate() != nil ||
		mock.calls != 1 || mock.subject != reconcilerSubject ||
		receipt.ReplayAuthorized || receipt.EffectConfirmed {
		t.Fatalf("Recovery handler forged authority: %#v err=%v calls=%d", receipt, err, mock.calls)
	}
	readback := secureRaw(t, pki.Client(t, reconcilerSubject), http.MethodGet, get, nil, http.StatusOK)
	var got recovery.ActionPlannedAbandonReceipt
	if err := json.Unmarshal(readback, &got); err != nil ||
		got.RequestDigest != receipt.RequestDigest ||
		!got.CreatedAt.Equal(receipt.CreatedAt) {
		t.Fatalf("read-only mTLS receipt not exact: %#v err=%v", got, err)
	}
}

func TestPlannedActionRecoveryRequiresDurableStore(t *testing.T) {
	server, pki := securedTestServer(t, store.NewMemory(), nil, AuthenticatedOptions{})
	secureCall(t, pki.Client(t, reconcilerSubject),
		server.URL+"/api/v1/recovery/actions/abandon-planned",
		plannedActionAPIFixture(), http.StatusServiceUnavailable)
	secureRaw(t, pki.Client(t, reconcilerSubject), http.MethodGet,
		server.URL+"/api/v1/recovery/actions/op-test/abandon-planned",
		nil, http.StatusServiceUnavailable)
}
