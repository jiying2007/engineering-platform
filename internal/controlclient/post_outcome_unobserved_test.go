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

func TestPostTransportCutAfterHandlerNeverClaimsRollbackOrRetries(t *testing.T) {
	p := newPKI(t)
	var observed atomic.Int32
	server := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server did not allow an HTTP/1.1 transport cut")
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		// The server has received the POST; a real network close discards
		// any success acknowledgement. No second request is ever sent.
		_ = conn.Close()
	}))
	c, err := New(server.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Raw(context.Background(), http.MethodPost, "/api/v1/mutation", []byte(`{}`))
	if !errors.Is(err, ErrPostOutcomeUnobserved) ||
		!strings.Contains(err.Error(), "read back") ||
		!strings.Contains(err.Error(), "before any retry") {
		t.Fatalf("lost POST transport did not demand exact readback: %v", err)
	}
	var httpError *HTTPError
	if errors.As(err, &httpError) {
		t.Fatalf("missing response was incorrectly represented as HTTP status %d", httpError.Status)
	}
	if n := observed.Load(); n != 1 {
		t.Fatalf("the client retried a potentially committed POST: %d", n)
	}
}

func TestUnusablePostSuccessCannotBeTreatedAsCompletedOrRetried(t *testing.T) {
	p := newPKI(t)
	var observed atomic.Int32
	server := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Add(1)
		switch r.URL.Path {
		case "/api/v1/malformed":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":`)
		case "/api/v1/wrong-mime":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, `{"result":"ok"}`)
		case "/api/v1/wrong-shape":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":"not-an-integer"}`)
		case "/api/v1/commit-unknown":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back the exact resource ID, epoch, idempotency key and audit record before considering any retry"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	c, err := New(server.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, path := range []string{"malformed", "wrong-mime"} {
		_, err = c.Raw(context.Background(), http.MethodPost, "/api/v1/"+path, []byte(`{}`))
		if !errors.Is(err, ErrPostOutcomeUnobserved) {
			t.Fatalf("POST %s accepted an unusable success acknowledgement: %v", path, err)
		}
	}
	var target struct {
		Result int `json:"result"`
	}
	err = c.Call(context.Background(), http.MethodPost, "/api/v1/wrong-shape", map[string]any{}, &target)
	if !errors.Is(err, ErrPostOutcomeUnobserved) {
		t.Fatalf("typed POST decode failure lost readback boundary: %v", err)
	}
	// A fixed 503 from the server already has the more specific COMMIT
	// classification. Do not collapse it into the generic transport case.
	_, err = c.Raw(context.Background(), http.MethodPost, "/api/v1/commit-unknown", []byte(`{}`))
	if !errors.Is(err, ErrCommitOutcomeUnknown) ||
		errors.Is(err, ErrPostOutcomeUnobserved) {
		t.Fatalf("explicit unknown COMMIT lost its distinct classification: %v", err)
	}
	if n := observed.Load(); n != 4 {
		t.Fatalf("unobserved POST result caused a replay: calls=%d", n)
	}
	_, err = c.Raw(context.Background(), http.MethodGet, "/api/v1/malformed", nil)
	if err == nil || errors.Is(err, ErrPostOutcomeUnobserved) {
		t.Fatalf("read-only GET misclassified as an unobserved POST: %v", err)
	}
}

func TestPostCancelledBeforeSendPreservesContextCauseWithoutReplay(t *testing.T) {
	p := newPKI(t)
	var observed atomic.Int32
	server := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	c, err := New(server.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Raw(ctx, http.MethodPost, "/api/v1/mutation", []byte(`{}`))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrPostOutcomeUnobserved) {
		t.Fatalf("cancelled POST did not preserve original context and safety label: %v", err)
	}
	if observed.Load() != 0 {
		t.Fatal("pre-cancelled POST reached the server")
	}
}
