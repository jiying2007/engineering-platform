package controlclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExternalEffectUnresolvedPreservesExactHTTPStatusAndNoReplay(t *testing.T) {
	p := newPKI(t)
	var requests atomic.Int32
	server := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		switch r.URL.Path {
		case "/api/v1/external-effect":
			_, _ = io.WriteString(w, `{"error":"external_effect_unresolved: read back the exact original Action operation ID, idempotency key, Run/Recovery epochs, receipt and audit before any retry"}`)
		case "/api/v1/private-error":
			_, _ = io.WriteString(w, `{"error":"external_effect_unresolved: read back private SQL password values"}`)
		case "/api/v1/commit":
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back the exact resource ID, epoch, idempotency key and audit record before considering any retry"}`)
		case "/api/v1/ordinary":
			_, _ = io.WriteString(w, `{"error":"service unavailable"}`)
		}
	}))
	client, err := New(server.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, tc := range []struct {
		name           string
		externalEffect bool
		commitUnknown  bool
	}{
		{name: "external-effect", externalEffect: true},
		{name: "private-error", externalEffect: true},
		{name: "commit", commitUnknown: true},
		{name: "ordinary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Raw(context.Background(), http.MethodPost,
				"/api/v1/"+tc.name, []byte(`{}`))
			var status *HTTPError
			if !errors.As(err, &status) || status.Status != http.StatusServiceUnavailable ||
				errors.Is(err, ErrExternalEffectUnresolved) != tc.externalEffect ||
				errors.Is(err, ErrCommitOutcomeUnknown) != tc.commitUnknown ||
				errors.Is(err, ErrPostOutcomeUnobserved) {
				t.Fatalf("unexpected external-effect/COMMIT/transport classification: %v", err)
			}
			if strings.Contains(err.Error(), "private SQL") ||
				strings.Contains(err.Error(), "password") {
				t.Fatalf("unsafe upstream SQL leaked into client diagnostics: %v", err)
			}
			if tc.externalEffect && (!strings.Contains(err.Error(), "read back") ||
				!strings.Contains(err.Error(), "before any retry")) {
				t.Fatalf("external Action effect status lost no-replay readback text: %v", err)
			}
		})
	}
	if requests.Load() != 4 {
		t.Fatalf("mTLS client silently replayed an unresolved external-effect POST: calls=%d", requests.Load())
	}
}
