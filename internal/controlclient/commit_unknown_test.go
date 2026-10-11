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

func TestUnknownCommitTransportErrorRequiresReadbackWithoutSQLLeakOrReplay(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	server := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/commit-unknown":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back the exact resource ID, epoch, idempotency key and audit record before considering any retry"}`)
		case "/api/v1/private-sql":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back private SQL credentials must not be printed"}`)
		case "/api/v1/ordinary-unavailable":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"service temporarily unavailable"}`)
		case "/api/v1/invalid-json":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":`)
		case "/api/v1/oversize":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back `+strings.Repeat("x", 2048)+`"}`)
		case "/api/v1/wrong-type":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	client, err := New(server.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cases := []struct {
		path    string
		unknown bool
	}{
		{path: "commit-unknown", unknown: true},
		{path: "private-sql", unknown: true},
		{path: "ordinary-unavailable", unknown: false},
		{path: "invalid-json", unknown: false},
		{path: "oversize", unknown: false},
		{path: "wrong-type", unknown: false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			_, err := client.Raw(context.Background(), http.MethodPost, "/api/v1/"+tc.path, []byte(`{}`))
			var reply *HTTPError
			if !errors.As(err, &reply) || reply.Status != http.StatusServiceUnavailable {
				t.Fatalf("HTTP status boundary changed: %v", err)
			}
			if errors.Is(err, ErrCommitOutcomeUnknown) != tc.unknown {
				t.Fatalf("COMMIT readback classification mismatch: %v", err)
			}
			if strings.Contains(err.Error(), "private SQL") ||
				strings.Contains(err.Error(), "credentials must not") {
				t.Fatalf("server-provided SQL details leaked to operator: %v", err)
			}
			if tc.unknown && (!strings.Contains(err.Error(), "read back") ||
				!strings.Contains(err.Error(), "before any retry")) {
				t.Fatalf("unknown COMMIT discarded no-retry/readback instruction: %v", err)
			}
		})
	}
	if got := calls.Load(); got != int32(len(cases)) {
		t.Fatalf("Control client retried a potentially effectful POST: calls=%d expected=%d", got, len(cases))
	}
}
