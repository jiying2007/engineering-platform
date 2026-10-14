package controlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type lifecycleTransport func(*http.Request) (*http.Response, error)

func (f lifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type lifecycleBody struct {
	source                 io.Reader
	readErr, closeErr      error
	onRead, onClose        func()
	reads, largest, closes int
}

func (b *lifecycleBody) Read(p []byte) (int, error) {
	if len(p) > b.largest {
		b.largest = len(p)
	}
	if b.onRead != nil {
		f := b.onRead
		b.onRead = nil
		f()
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	n, err := b.source.Read(p)
	b.reads += n
	return n, err
}
func (b *lifecycleBody) Close() error {
	b.closes++
	if b.onClose != nil {
		b.onClose()
	}
	return b.closeErr
}
func lifecycleClient(b *lifecycleBody, status int, calls *int) *Client {
	return &Client{endpoint: "https://control.invalid", http: &http.Client{Transport: lifecycleTransport(func(*http.Request) (*http.Response, error) {
		(*calls)++
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: b}, nil
	})}}
}
func lifecycleInput(method string) []byte {
	if method == http.MethodPost {
		return []byte(`{}`)
	}
	return nil
}

func TestControlResponseLifecycleIncompleteBody(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, status := range []int{200, 404, 503} {
			for _, kind := range []string{"read", "close", "read-close", "cancel-read", "cancel-close"} {
				t.Run(fmt.Sprintf("%s/%d/%s", method, status, kind), func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					first, second := errors.New("private-read-detail"), errors.New("private-close-detail")
					body := &lifecycleBody{source: strings.NewReader(`{"error":"commit_outcome_unknown: read back exact identity"}`)}
					switch kind {
					case "read":
						body.readErr = first
					case "close":
						body.closeErr = first
					case "read-close":
						body.readErr = first
						body.closeErr = second
					case "cancel-read":
						body.onRead = cancel
						first = context.Canceled
					case "cancel-close":
						body.onClose = cancel
						first = context.Canceled
					}
					calls := 0
					c := lifecycleClient(body, status, &calls)
					data, err := c.Raw(ctx, method, "/api/v1/probe", lifecycleInput(method))
					if err == nil || data != nil || !errors.Is(err, first) || errors.Is(err, ErrPostOutcomeUnobserved) != (method == http.MethodPost) {
						t.Fatalf("incomplete control response accepted or cause lost: data=%q err=%v", data, err)
					}
					if errors.Is(err, ErrCommitOutcomeUnknown) || errors.Is(err, ErrExternalEffectUnresolved) {
						t.Fatal("incomplete response gained precise authority", err)
					}
					if kind == "read-close" && !errors.Is(err, second) {
						t.Fatal("simultaneous close error lost")
					}
					if status != 200 {
						var httpErr *HTTPError
						if !errors.As(err, &httpErr) || httpErr.Status != status {
							t.Fatal("observed status lost", err)
						}
					}
					if strings.Contains(err.Error(), "private-") || calls != 1 || body.closes != 1 {
						t.Fatalf("leak, replay or body leak: %v calls=%d closes=%d", err, calls, body.closes)
					}
				})
			}
		}
	}
}

func TestControlResponseLifecycleCompleteStatuses(t *testing.T) {
	cases := []struct {
		name, raw string
		status    int
		want      error
	}{
		{"success", `{"value":"new"}`, 200, nil},
		{"not-found", `not found`, 404, nil},
		{"forbidden", ``, 403, nil},
		{"commit-unknown", `{"error":"commit_outcome_unknown: read back exact identity"}`, 503, ErrCommitOutcomeUnknown},
		{"effect-unresolved", `{"error":"external_effect_unresolved: read back original operation"}`, 503, ErrExternalEffectUnresolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &lifecycleBody{source: strings.NewReader(tc.raw)}
			calls := 0
			c := lifecycleClient(b, tc.status, &calls)
			data, err := c.Raw(context.Background(), http.MethodPost, "/api/v1/probe", []byte(`{}`))
			if tc.status == 200 {
				if err != nil || string(data) != tc.raw {
					t.Fatal("complete success rejected", err)
				}
			} else {
				var status *HTTPError
				if !errors.As(err, &status) || status.Status != tc.status || errors.Is(err, ErrPostOutcomeUnobserved) {
					t.Fatal("complete status misclassified", err)
				}
				if tc.want != nil && !errors.Is(err, tc.want) {
					t.Fatal("precise status lost", err)
				}
			}
			if b.closes != 1 || calls != 1 {
				t.Fatal("body or exchange lifecycle mismatch")
			}
		})
	}
}

type lifecycleMarshal struct {
	cancel context.CancelFunc
	calls  *int
}

func (v lifecycleMarshal) MarshalJSON() ([]byte, error) {
	(*v.calls)++
	v.cancel()
	return []byte(`{}`), nil
}
func TestControlResponseLifecycleCancelledBeforeExchange(t *testing.T) {
	for _, kind := range []string{"raw", "before-marshal", "during-marshal"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := &lifecycleBody{source: strings.NewReader(`{}`)}
			calls, marshals := 0, 0
			c := lifecycleClient(b, 200, &calls)
			if kind != "during-marshal" {
				cancel()
			}
			var err error
			if kind == "raw" {
				_, err = c.Raw(ctx, http.MethodPost, "/api/v1/probe", []byte(`{}`))
			} else {
				err = c.Call(ctx, http.MethodPost, "/api/v1/probe", lifecycleMarshal{cancel, &marshals}, nil)
			}
			expected := 0
			if kind == "during-marshal" {
				expected = 1
			}
			if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrPostOutcomeUnobserved) || calls != 0 || marshals != expected {
				t.Fatalf("cancelled control caller advanced: %v exchanges=%d marshals=%d", err, calls, marshals)
			}
		})
	}
}

// Per-key callbacks trigger cancellation inside a real UnmarshalJSON call.
// A fresh output correctly discards receiver fields, so no receiver-held hook
// or timing-dependent goroutine is used; distinct keys cannot cross requests.
var lifecycleDecodeCallbacks sync.Map

type lifecycleDecodeReceipt struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (r *lifecycleDecodeReceipt) UnmarshalJSON(data []byte) error {
	type wire lifecycleDecodeReceipt
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = lifecycleDecodeReceipt(decoded)
	if f, ok := lifecycleDecodeCallbacks.Load(decoded.Key); ok {
		f.(context.CancelFunc)()
	}
	return nil
}
func TestControlResponseLifecycleCancelledDecode(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			key := t.Name()
			lifecycleDecodeCallbacks.Store(key, cancel)
			defer lifecycleDecodeCallbacks.Delete(key)
			raw, _ := json.Marshal(map[string]string{"key": key, "value": "new"})
			b := &lifecycleBody{source: bytes.NewReader(raw)}
			calls := 0
			c := lifecycleClient(b, 200, &calls)
			before := lifecycleDecodeReceipt{Key: "retained", Value: "original"}
			out := before
			var input any
			if method == http.MethodPost {
				input = struct{}{}
			}
			err := c.Call(ctx, method, "/api/v1/probe", input, &out)
			if !errors.Is(err, context.Canceled) || errors.Is(err, ErrPostOutcomeUnobserved) != (method == http.MethodPost) || out != before {
				t.Fatalf("cancelled decode committed output: %+v err=%v", out, err)
			}
			if calls != 1 || b.closes != 1 {
				t.Fatal("cancelled decode retried or leaked body")
			}
		})
	}
}

func TestControlResponseLifecycleReadBudgets(t *testing.T) {
	for _, status := range []int{200, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			limit := 1024
			if status == 200 {
				limit = MaxResponse
			}
			b := &lifecycleBody{source: strings.NewReader(strings.Repeat("x", limit+4096))}
			calls := 0
			c := lifecycleClient(b, status, &calls)
			data, err := c.Raw(context.Background(), http.MethodPost, "/api/v1/probe", []byte(`{}`))
			if !errors.Is(err, ErrPostOutcomeUnobserved) || data != nil || b.reads != limit+1 || b.largest > 64<<10 || b.closes != 1 || calls != 1 {
				t.Fatalf("unbounded or unread control response: %v read=%d chunk=%d closes=%d calls=%d", err, b.reads, b.largest, b.closes, calls)
			}
		})
	}
}

func TestControlResponseLifecycleRealMTLSTruncatedError(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "256")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"unavailable"}`)
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	data, err := c.Raw(context.Background(), http.MethodPost, "/api/v1/probe", []byte(`{}`))
	var status *HTTPError
	if !errors.Is(err, ErrPostOutcomeUnobserved) || !errors.Is(err, io.ErrUnexpectedEOF) || !errors.As(err, &status) || status.Status != 404 || data != nil || calls.Load() != 1 {
		t.Fatalf("truncated error became a complete rejection: %v calls=%d", err, calls.Load())
	}
}

func TestControlResponseLifecycleOnlyCompleteConflictIsIgnorable(t *testing.T) {
	for _, kind := range []string{"complete", "read", "close", "cancel-close"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &lifecycleBody{source: strings.NewReader(`{"error":"conflict"}`)}
			switch kind {
			case "read":
				body.readErr = io.ErrUnexpectedEOF
			case "close":
				body.closeErr = errors.New("private close error")
			case "cancel-close":
				body.onClose = cancel
			}
			calls := 0
			c := lifecycleClient(body, 409, &calls)
			_, err := c.Raw(ctx, http.MethodPost, "/api/v1/probe", []byte(`{}`))
			if IsCompleteHTTPStatus(err, 409) != (kind == "complete") {
				t.Fatalf("Worker conflict decision erased uncertainty: %v", err)
			}
			if calls != 1 || body.closes != 1 {
				t.Fatal("conflict decision retried or leaked a body")
			}
		})
	}
	complete := &HTTPError{Status: 409}
	var absent *HTTPError
	for _, err := range []error{nil, absent, &HTTPError{Status: 403},
		fmt.Errorf("wrapped: %w", complete), errors.Join(complete, context.Canceled),
		errors.Join(complete, ErrPostOutcomeUnobserved)} {
		if IsCompleteHTTPStatus(err, 409) {
			t.Fatal("embedded or mismatched status gained a complete verdict", err)
		}
	}
}
