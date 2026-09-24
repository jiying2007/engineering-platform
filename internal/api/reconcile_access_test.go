package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type reconcileGateway struct{ recordingGateway }

func (g *reconcileGateway) Reconcile(ctx context.Context, _ string) (action.Receipt, error) {
	id, ok := AuthenticatedIdentity(ctx)
	if !ok || id.Subject() != recoverySubject {
		return action.Receipt{}, action.ErrDenied
	}
	g.calls.Add(1)
	return action.Receipt{ID: "reconcile-test"}, nil
}

func TestReconcileCannotBypassMutationJSONGate(t *testing.T) {
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: recoverySubject, Scope: "platform", Capabilities: []string{access.ActionReconcile}}}})
	if err != nil {
		t.Fatal(err)
	}
	gateway := &reconcileGateway{}
	handler, err := NewAuthenticatedHandler(store.NewMemory(), gateway, policy, AuthenticatedOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	client := pki.Client(t, recoverySubject)
	for _, tc := range []struct {
		mediaType, body string
		want            int
	}{
		{"text/plain", "{}", http.StatusUnsupportedMediaType},
		{"application/x-www-form-urlencoded", "x=y", http.StatusUnsupportedMediaType},
		{"application/json", "", http.StatusBadRequest},
		{"application/json", "{}", http.StatusOK},
	} {
		response, err := client.Post(server.URL+"/api/v1/actions/op/reconcile", tc.mediaType, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("reconcile media=%s got=%d want=%d", tc.mediaType, response.StatusCode, tc.want)
		}
	}
	if gateway.calls.Load() != 1 {
		t.Fatal("invalid mutation reached gateway")
	}
}
