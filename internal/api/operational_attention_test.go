package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestOperationalAttentionAPIRequiresTrustedReadAndPreservesSource(t *testing.T) {
	pki := testsupport.NewPKI(t)
	policy, _ := apiCapacityPolicy(t)
	s := production.Snapshot{
		Version:           production.OperationalStatusVersion,
		CapturedAt:        time.Now().UTC(),
		RecoveryMode:      "NORMAL",
		WorkerPolls:       []production.WorkerPollObservation{},
		UnknownOperations: 2,
	}
	status, err := production.EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	backend := &operationalMemory{Memory: store.NewMemory(), status: status}
	handler, err := NewAuthenticatedHandler(backend, nil, policy, AuthenticatedOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()

	client := pki.Client(t, capacityReaderSubject)
	response, err := client.Get(server.URL + "/api/v1/operations/attention")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("untrusted or cacheable attention response: %d %#v", response.StatusCode, response.Header)
	}
	var projection production.OperationalAttention
	if err := json.NewDecoder(response.Body).Decode(&projection); err != nil {
		t.Fatal(err)
	}
	if !projection.RequiresAttention || len(projection.Items) != 1 ||
		projection.Items[0].Reason != "unknown_external_actions" ||
		projection.Items[0].Count != 2 ||
		projection.SourceScope != production.OperationalAttentionScope {
		t.Fatal("projection detached from validated source", projection)
	}
	secureRaw(t, pki.Client(t, capacityWorkerSubject), http.MethodGet, server.URL+"/api/v1/operations/attention", nil, http.StatusForbidden)
	secureRaw(t, pki.Client(t, "urn:engineering-platform:unknown"), http.MethodGet, server.URL+"/api/v1/operations/attention", nil, http.StatusUnauthorized)
	mustRequest(t, NewServer(backend).Handler(), http.MethodGet, "/api/v1/operations/attention", nil, http.StatusForbidden)
}

func TestOperationalAttentionAPIFailsClosedOnUnavailableOrStaleFacts(t *testing.T) {
	pki := testsupport.NewPKI(t)
	policy, _ := apiCapacityPolicy(t)
	for _, backend := range []store.Store{
		store.NewMemory(),
		&operationalMemory{Memory: store.NewMemory(), err: errors.New("database unavailable")},
		&operationalMemory{Memory: store.NewMemory(), status: production.OperationalStatus{}},
	} {
		handler, err := NewAuthenticatedHandler(backend, nil, policy, AuthenticatedOptions{})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewUnstartedServer(handler)
		server.TLS = pki.ServerTLS()
		server.StartTLS()
		secureRaw(t, pki.Client(t, capacityReaderSubject), http.MethodGet, server.URL+"/api/v1/operations/attention", nil, http.StatusServiceUnavailable)
		server.Close()
	}
}
