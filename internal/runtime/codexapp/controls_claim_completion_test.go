package codexapp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type terminalClaimController struct {
	*exitController
	claim      func(context.Context) error
	claimCalls int
}

func (c *terminalClaimController) Claim(ctx context.Context) (*LiveControl, error) {
	c.claimCalls++
	return nil, c.claim(ctx)
}

type terminalClaimSemanticError struct{ cause error }

func (e *terminalClaimSemanticError) Error() string { return "test-only claim outcome unobserved" }
func (e *terminalClaimSemanticError) Unwrap() error { return e.cause }
func (e *terminalClaimSemanticError) Is(target error) bool {
	return target == controlclient.ErrPostOutcomeUnobserved
}

// The actual JSONL client observes a terminal event while the Core claim is
// still in flight. No process, model invocation or production proof is created.
func observeTerminalClaim(t *testing.T, controller *terminalClaimController, ready <-chan struct{}) (EngineeringObservation, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, remote := pair(t)
	defer client.Close()
	defer remote.Close()
	adapter, err := NewAdapter(client, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	adapter.initialized, adapter.thread, adapter.turn = true, "thread", "turn"
	type result struct {
		value EngineeringObservation
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := observeControlledEngineering(ctx, adapter, "thread", "turn", controller)
		done <- result{value, err}
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("Core claim did not reach the terminal-event barrier")
	}
	if err := remote.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	terminal := `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"result","type":"agentMessage","text":"test-only result"}}}` + "\n" +
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}` + "\n"
	if _, err := io.WriteString(remote, terminal); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if ctx.Err() != nil || got.value.Status != "completed" || controller.claimCalls != 1 || controller.reportCalls != 0 {
			t.Fatalf("terminal/claim lifetime lost or replayed: %+v %v", got.value, got.err)
		}
		return got.value, got.err
	case <-ctx.Done():
		t.Fatal("terminal and claim were not joined within their budget")
		return EngineeringObservation{}, ctx.Err()
	}
}

func TestEngineeringTerminalPreservesClaimFailure(t *testing.T) {
	independent := errors.New("test-only independent claim failure")
	for _, tc := range []struct {
		name string
		err  func(error) error
		want error
	}{
		{"owned-cancel", func(err error) error { return err }, nil},
		{"wrapped-owned-cancel", func(err error) error { return fmt.Errorf("poll stopped: %w", err) }, nil},
		{"joined-owned-cancels", func(err error) error { return errors.Join(err, fmt.Errorf("poll stopped: %w", err)) }, nil},
		{"mixed-failure", func(err error) error { return errors.Join(err, independent) }, independent},
		{"semantic-unwrapped-cancel", func(err error) error { return &terminalClaimSemanticError{cause: err} }, controlclient.ErrPostOutcomeUnobserved},
		{"independent-deadline", func(error) error { return context.DeadlineExceeded }, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ready := make(chan struct{})
			controller := &terminalClaimController{exitController: &exitController{fixtureController: &fixtureController{bound: make(chan struct{})}}}
			controller.claim = func(ctx context.Context) error {
				close(ready)
				<-ctx.Done()
				return tc.err(ctx.Err())
			}
			_, err := observeTerminalClaim(t, controller, ready)
			if tc.want == nil {
				if err != nil {
					t.Fatal("ordinary owned polling cancellation became a claim failure", err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("terminal completion erased original claim failure: got=%v want=%v", err, tc.want)
			}
		})
	}
}

// This uses the production direct-mTLS client and original POST route, with a
// deliberately bounded HTTP fixture. It proves transport/error propagation;
// the fixture is not a durable Core claim or provider qualification.
func TestEngineeringTerminalPreservesMTLSClaimOutcome(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"lost-claim-response", 0, "", controlclient.ErrPostOutcomeUnobserved},
		{"commit-outcome-unknown", http.StatusServiceUnavailable, `{"error":"commit_outcome_unknown: read back exact identity"}`, controlclient.ErrCommitOutcomeUnknown},
		{"external-effect-unresolved", http.StatusServiceUnavailable, `{"error":"external_effect_unresolved: read back original operation"}`, controlclient.ErrExternalEffectUnresolved},
		{"complete-conflict", http.StatusConflict, `{"error":"conflict"}`, nil},
		{"malformed-success", http.StatusOK, `{"delivery":`, controlclient.ErrPostOutcomeUnobserved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ready, handlerDone := make(chan struct{}), make(chan struct{})
			var requests atomic.Int32
			pki := testsupport.NewPKI(t)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				requests.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/worker/codex/controls/claim" || r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
					http.Error(w, "invalid fixture request", http.StatusBadRequest)
					return
				}
				if _, err := io.Copy(io.Discard, io.LimitReader(r.Body, 4097)); err != nil {
					return
				}
				if tc.status == 0 {
					// The real POST reached the authenticated endpoint, but no response
					// is available before the observer stops its polling context.
					close(ready)
					select {
					case <-r.Context().Done():
					case <-ctx.Done():
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			certificate := pki.ClientCertificate(t, "urn:engineering-platform:worker:terminal-claim-fixture")
			client, err := controlclient.New(server.URL, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{certificate}})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			controller := &terminalClaimController{exitController: &exitController{fixtureController: &fixtureController{bound: make(chan struct{})}}}
			var original error
			controller.claim = func(ctx context.Context) error {
				var response struct {
					Delivery *struct{} `json:"delivery"`
				}
				original = client.Call(ctx, http.MethodPost, "/api/v1/worker/codex/controls/claim", struct{}{}, &response)
				if tc.status != 0 {
					close(ready)
					<-ctx.Done()
				}
				return original
			}
			value, err := observeTerminalClaim(t, controller, ready)
			if original == nil || !errors.Is(err, original) || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("terminal completion erased the real POST error: got=%v original=%v want=%v", err, original, tc.want)
			}
			if tc.status == 0 && !errors.Is(err, context.Canceled) {
				t.Fatal("POST cancellation cause was lost", err)
			}
			select {
			case <-handlerDone:
			case <-ctx.Done():
				t.Fatal("HTTP claim handler did not exit")
			}
			if requests.Load() != 1 {
				t.Fatal("claim POST replayed", requests.Load())
			}
			// Production settlement must preserve the original claim error even
			// when independent Close adds its own unresolved outcome. This proof
			// is explicitly a unit fixture, not evidence of a process reap.
			controller.closeError = fixtureCloseFailure
			_, settled := settleEngineeringExit(ctx, nil, exitTestProof(), controller, true, value.Status, err)
			if !errors.Is(settled, original) || !errors.Is(settled, fixtureCloseFailure) || !errors.Is(settled, ErrEngineeringCloseUnobserved) ||
				controller.closeCalls != 1 || controller.status != "completed" || controller.invalidCloseContext {
				t.Fatalf("Close erased the original claim outcome or retried: %v", settled)
			}
		})
	}
}
