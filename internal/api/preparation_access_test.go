package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPreparationRoutesRequireDedicatedVerifiedGrant(t *testing.T) {
	const subject = "urn:engineering-platform:worker:ordinary-input"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: subject, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{"worker/preparation"}}}})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewAuthenticatedHandler(store.NewMemory(), nil, policy, AuthenticatedOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client := pki.Client(t, subject)
	for _, path := range []string{"/api/v1/worker/prepare-claim", "/api/v1/worker/prepared"} {
		response, err := client.Post(server.URL+path, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("missing dedicated grant: %s returned %d, want 403", path, response.StatusCode)
		}
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		NewServer(store.NewMemory()).Handler().ServeHTTP(rec, r)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("anonymous fixture exposed preparation: %s returned %d", path, rec.Code)
		}
	}
}
