package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// This deliberately synthetic observer never contacts GitHub. TLS identities,
// Core HTTP routing, durable transaction/audit and independent completion are real.
// Publisher journal generation has separate package tests; do not call this
// fixture a live provider/dispatcher quiescence qualification.
type databaseNonInvocationObserver struct {
	evidence    githubpublish.NonInvocationEvidence
	calls       atomic.Int32
	unavailable atomic.Bool
}

func (o *databaseNonInvocationObserver) FenceUninvokedPublication(ctx context.Context, op action.Operation) (githubpublish.NonInvocationEvidence, error) {
	o.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return githubpublish.NonInvocationEvidence{}, err
	}
	if o.unavailable.Load() || op.ID != o.evidence.Start.Identity.OperationID {
		return githubpublish.NonInvocationEvidence{}, githubpublish.ErrDispatchEvidenceUnavailable
	}
	return o.evidence, nil
}

func TestNoInvocationDatabaseMTLSIndependentCompletion(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseMTLSIndependentCompletion)
}
func testNoInvocationDatabaseMTLSIndependentCompletion(t *testing.T, version int) {
	s, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
	observer := &databaseNonInvocationObserver{evidence: e}
	const reader = "urn:engineering-platform:operator:readonly"
	const proof = "urn:engineering-platform:operator:proof"
	const completer = "urn:engineering-platform:operator:completer"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: reader, Scope: "platform", Capabilities: []string{access.Read}},
		// Production policy keeps reconciliation and completion grants disjoint.
		{Subject: plannedReconciler, Scope: "platform", Capabilities: []string{access.Read, access.RecoveryReconcile}},
		{Subject: proof, Scope: "platform", Capabilities: []string{access.Read, access.RecoveryReconcile}},
		{Subject: completer, Scope: "platform", Capabilities: []string{access.Read, access.RecoveryComplete}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewAuthenticatedHandler(s, nil, policy, api.AuthenticatedOptions{RecoveryCompletion: s, PublicationRecovery: observer})
	if err != nil {
		t.Fatal(err)
	}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	readonly, reconciler, proofClient, completeClient := pki.Client(t, reader), pki.Client(t, plannedReconciler), pki.Client(t, proof), pki.Client(t, completer)
	call := func(client *http.Client, method, path string, in any, want int) []byte {
		t.Helper()
		var data []byte
		if in != nil {
			var x error
			data, x = json.Marshal(in)
			if x != nil {
				t.Fatal(x)
			}
		}
		r, x := http.NewRequestWithContext(context.Background(), method, server.URL+path, bytes.NewReader(data))
		if x != nil {
			t.Fatal(x)
		}
		if in != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		response, x := client.Do(r)
		if x != nil {
			t.Fatal(x)
		}
		defer response.Body.Close()
		raw, x := io.ReadAll(io.LimitReader(response.Body, 256<<10))
		if x != nil {
			t.Fatal(x)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.StatusCode, want, raw)
		}
		return raw
	}
	path := "/api/v1/recovery/actions/abandon-uninvoked"
	getPath := "/api/v1/recovery/actions/" + req.Identity.OperationID + "/uninvoked"
	proofBody := map[string]any{"recovery_epoch": 1, "reconciler": proof}
	completeBody := map[string]any{"recovery_epoch": 1}
	call(readonly, http.MethodPost, path, req, http.StatusForbidden)
	call(completeClient, http.MethodPost, "/api/v1/recovery/complete", completeBody, http.StatusForbidden)
	if observer.calls.Load() != 0 {
		t.Fatal("denied request reached observer")
	}
	first := call(reconciler, http.MethodPost, path, req, http.StatusCreated)
	observer.unavailable.Store(true)
	repeated := call(reconciler, http.MethodPost, path, req, http.StatusOK)
	if !bytes.Equal(first, repeated) {
		t.Fatal("exact repeat changed saved receipt")
	}
	call(proofClient, http.MethodPost, path, req, http.StatusConflict)
	if !bytes.Equal(first, call(readonly, http.MethodGet, getPath, nil, http.StatusOK)) {
		t.Fatal("GET receipt mismatch")
	}
	call(proofClient, http.MethodPost, "/api/v1/recovery/proofs", proofBody, http.StatusCreated)
	call(reconciler, http.MethodPost, "/api/v1/recovery/complete", completeBody, http.StatusForbidden)
	call(completeClient, http.MethodPost, "/api/v1/recovery/complete", completeBody, http.StatusOK)
	if !bytes.Equal(first, call(reconciler, http.MethodPost, path, req, http.StatusOK)) {
		t.Fatal("post-completion readback changed")
	}
	if observer.calls.Load() != 1 {
		t.Fatal("readbacks or denied calls repeated observer", observer.calls.Load())
	}
	var count int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_events WHERE event_type='recovery.github.publication.not_invoked'").Scan(&count); err != nil || count != 1 {
		t.Fatal("settlement was not exactly once", count, err)
	}
}

func (o *databaseNonInvocationObserver) ObserveOriginalPublication(context.Context, action.Operation) (githubpublish.PublicationEvidence, error) {
	return githubpublish.PublicationEvidence{}, githubpublish.ErrDispatchEvidenceUnavailable
}
