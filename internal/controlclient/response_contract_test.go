package controlclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// All probes observe precisely one POST, including unusable responses. A
// successful test is a negative safety contract, never a remote effect proof.
func TestResponseObjectContractPreservesOutputAndNeverRetries(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	var reply string
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	type receipt struct {
		ID     string            `json:"id"`
		Count  int               `json:"count"`
		Values map[string]string `json:"values,omitempty"`
	}
	for _, body := range []string{
		`null`, `[]`, `false`, `"receipt"`, `{}`, `{"id":"new"}`,
		`{"id":null,"count":1}`, `{"ID":"new","count":1}`,
		`{"id":"new","count":1,"id":"different"}`,
		`{"id":"new","count":1,"values":{"a":"one","a":"two"}}`,
		`{"id":"new","values":{"a":"changed"},"count":"invalid"}`,
		`{"id":"new","count":1,"extra":true}`,
		`{"id":"new","count":1} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			reply = body
			out := receipt{ID: "previous", Count: 42, Values: map[string]string{"a": "old"}}
			before := calls.Load()
			err := c.Call(context.Background(), http.MethodPost, "/api/v1/work-items", map[string]string{"id": "new"}, &out)
			if !errors.Is(err, ErrPostOutcomeUnobserved) || calls.Load() != before+1 {
				t.Fatalf("unusable receipt was accepted or retried: %v", err)
			}
			if out.ID != "previous" || out.Count != 42 || !reflect.DeepEqual(out.Values, map[string]string{"a": "old"}) {
				t.Fatal("failed decode contaminated previous receipt", out)
			}
		})
	}
	reply = `{"id":"new","count":0}`
	out := receipt{ID: "previous", Count: 42, Values: map[string]string{"a": "old"}}
	if err := c.Call(context.Background(), http.MethodPost, "/api/v1/work-items", map[string]string{"id": "new"}, &out); err != nil || out.ID != "new" || out.Count != 0 || out.Values != nil {
		t.Fatalf("valid receipt retained stale fields: %+v %v", out, err)
	}
}

func TestResponseClaimDistinguishesMissingAndExplicitNull(t *testing.T) {
	p := newPKI(t)
	var reply string
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, body := range []string{`null`, `{}`, `{"ASSIGNMENT":null}`, `{"assignment":null}`} {
		reply = body
		var out struct {
			Assignment *struct{} `json:"assignment"`
		}
		err := c.Call(context.Background(), http.MethodPost, "/api/v1/worker/claim", map[string]string{"worker_profile": "test"}, &out)
		if body == `{"assignment":null}` {
			if err != nil || out.Assignment != nil {
				t.Fatal("explicit empty claim rejected", err)
			}
		} else if !errors.Is(err, ErrPostOutcomeUnobserved) {
			t.Fatal("missing claim receipt became empty queue", body, err)
		}
	}
}

func TestResponseDestinationRejectedBeforePOST(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var nilPointer *struct{}
	for _, out := range []any{struct{}{}, nilPointer} {
		if err := c.Call(context.Background(), http.MethodPost, "/api/v1/work-items", map[string]string{}, out); err == nil || errors.Is(err, ErrPostOutcomeUnobserved) {
			t.Fatal("invalid local destination admitted or classified as remote uncertainty", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid destination caused POST")
	}
}

func TestBroken503RetainsHTTPStatusUncertaintyAndReadCause(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/cut":
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"commit_outcome_unknown: read back private-token`)
		case "/api/v1/malformed":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"private-token`)
		case "/api/v1/oversized":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"`+strings.Repeat("x", 1024)+`"}`)
		case "/api/v1/alias":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"Error":"commit_outcome_unknown: read back"}`)
		case "/api/v1/duplicate":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"unavailable","error":"commit_outcome_unknown: read back"}`)
		case "/api/v1/null":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `null`)
		default:
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "private-token")
		}
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, name := range []string{"cut", "malformed", "oversized", "alias", "duplicate", "null", "wrong-mime"} {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			before := calls.Load()
			var body []byte
			if method == http.MethodPost {
				body = []byte(`{}`)
			}
			_, err := c.Raw(context.Background(), method, "/api/v1/"+name, body)
			var status *HTTPError
			if !errors.As(err, &status) || status.Status != http.StatusServiceUnavailable ||
				errors.Is(err, ErrPostOutcomeUnobserved) != (method == http.MethodPost) ||
				errors.Is(err, ErrCommitOutcomeUnknown) || errors.Is(err, ErrExternalEffectUnresolved) || calls.Load() != before+1 {
				t.Fatalf("bad response classification/replay for %s %s: %v", method, name, err)
			}
			if name == "cut" && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("lost read cause", err)
			}
			if strings.Contains(err.Error(), "private-token") {
				t.Fatal("private error body leaked", err)
			}
		}
	}
}

func TestRawNullRejectedEvenWithoutTypedDestination(t *testing.T) {
	p := newPKI(t)
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "null")
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Call(context.Background(), http.MethodPost, "/api/v1/mutation", struct{}{}, nil); !errors.Is(err, ErrPostOutcomeUnobserved) {
		t.Fatal("untyped null receipt accepted", err)
	}
	if _, err := c.Raw(context.Background(), http.MethodGet, "/healthz", nil); err == nil || errors.Is(err, ErrPostOutcomeUnobserved) {
		t.Fatal("GET null accepted or incorrectly classified as POST", err)
	}
}
