package codexapp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
func observeTerminalClaim(t *testing.T, controller *terminalClaimController, ready <-chan struct{}, release func(), mode string) (EngineeringObservation, error) {
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
	if mode == "invalid-terminal" {
		terminal = strings.Replace(terminal, `"method":"turn/completed","params":{"threadId":"thread"`, `"method":"turn/completed","params":{"threadId":"foreign"`, 1)
	}
	if _, err := io.WriteString(remote, terminal); err != nil {
		t.Fatal(err)
	}
	if mode != "invalid-terminal" {
		// Observe the real Adapter's terminal transition before releasing the
		// withheld HTTP response. This is test synchronization, not a new
		// production callback or a substitute terminal/receipt authority.
		poll := time.NewTicker(time.Millisecond)
		defer poll.Stop()
		for {
			adapter.mu.Lock()
			completed := adapter.turn == ""
			adapter.mu.Unlock()
			if completed {
				break
			}
			select {
			case <-poll.C:
			case <-ctx.Done():
				t.Fatal("terminal event did not reach the Adapter")
			}
		}
		if mode == "parent-cancel" {
			cancel()
		} else {
			// Keep the actual response unavailable while terminal observation
			// completes. An owned polling stop must not truncate this POST.
			timer := time.NewTimer(25 * time.Millisecond)
			defer timer.Stop()
			select {
			case got := <-done:
				t.Fatalf("terminal event cut off the pending Claim response: %v", got.err)
			case <-timer.C:
			case <-ctx.Done():
				t.Fatal("terminal/claim barrier exceeded its budget")
			}
			release()
		}
	}
	select {
	case got := <-done:
		wantStatus := "completed"
		if mode == "invalid-terminal" {
			wantStatus = ""
		}
		if (mode != "parent-cancel" && ctx.Err() != nil) || got.value.Status != wantStatus || controller.claimCalls != 1 || controller.reportCalls != 0 {
			t.Fatalf("terminal/claim lifetime lost or replayed: %+v %v", got.value, got.err)
		}
		return got.value, got.err
	case <-ctx.Done():
		// Parent cancellation is an intentional test case, so still require
		// the real observer and in-flight request to join after that signal.
		select {
		case got := <-done:
			if mode != "parent-cancel" || got.value.Status != "completed" || controller.claimCalls != 1 || controller.reportCalls != 0 {
				t.Fatalf("terminal/claim cancellation lifetime lost: %+v %v", got.value, got.err)
			}
			return got.value, got.err
		case <-time.After(time.Second):
			t.Fatal("terminal and claim were not joined within their budget")
			return EngineeringObservation{}, ctx.Err()
		}
	}
}

func TestEngineeringTerminalPreservesClaimFailure(t *testing.T) {
	independent := errors.New("test-only independent claim failure")
	for _, tc := range []struct {
		name        string
		err         func(error) error
		want        error
		keepPolling bool
	}{
		{"owned-cancel", func(err error) error { return err }, nil, false},
		{"wrapped-owned-cancel", func(err error) error { return fmt.Errorf("poll stopped: %w", err) }, nil, false},
		{"joined-owned-cancels", func(err error) error { return errors.Join(err, fmt.Errorf("poll stopped: %w", err)) }, nil, false},
		{"mixed-failure", func(err error) error { return errors.Join(err, independent) }, independent, false},
		{"semantic-unwrapped-cancel", func(err error) error { return &terminalClaimSemanticError{cause: err} }, controlclient.ErrPostOutcomeUnobserved, false},
		{"independent-deadline", func(error) error { return context.DeadlineExceeded }, context.DeadlineExceeded, false},
		{"unrequested-cancel", func(err error) error { return err }, context.Canceled, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			polling, stopPolling := context.WithCancel(owner)
			defer stopPolling()
			inFlight, stopCall := context.WithCancel(owner)
			defer stopCall()
			ready, release := make(chan struct{}), make(chan struct{})
			controller := &terminalClaimController{exitController: &exitController{fixtureController: &fixtureController{bound: make(chan struct{})}}}
			controller.claim = func(ctx context.Context) error {
				close(ready)
				select {
				case <-release:
					return tc.err(context.Canceled)
				case <-owner.Done():
					return owner.Err()
				}
			}
			done := make(chan error, 1)
			go func() { done <- pumpControls(polling, inFlight, &Adapter{}, "thread", "turn", controller) }()
			select {
			case <-ready:
			case <-owner.Done():
				t.Fatal("Claim did not reach the error barrier")
			}
			if !tc.keepPolling {
				stopPolling()
			}
			close(release)
			var err error
			select {
			case err = <-done:
			case <-owner.Done():
				t.Fatal("Claim classification did not join")
			}
			if controller.claimCalls != 1 || controller.reportCalls != 0 || inFlight.Err() != nil {
				t.Fatal("Claim classification replayed or detached its owner")
			}
			if tc.want == nil {
				if err != nil {
					t.Fatal("ordinary owned polling cancellation became a claim failure", err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("terminal completion erased original claim failure: got=%v want=%v", err, tc.want)
			}
		})
	}
	t.Run("in-flight-owner-deadline", func(t *testing.T) {
		owner, cancel := context.WithTimeout(context.WithValue(t.Context(), exitContextKey{}, "original"), time.Second)
		defer cancel()
		polling, stopPolling := context.WithCancel(owner)
		defer stopPolling()
		inFlight, stopCall := context.WithTimeout(owner, 30*time.Millisecond)
		defer stopCall()
		var claimed context.Context
		controller := &terminalClaimController{exitController: &exitController{fixtureController: &fixtureController{}}}
		controller.claim = func(ctx context.Context) error {
			claimed = ctx
			stopPolling()
			<-ctx.Done()
			return ctx.Err()
		}
		err := pumpControls(polling, inFlight, &Adapter{}, "thread", "turn", controller)
		deadline, ok := claimed.Deadline()
		wantDeadline, _ := inFlight.Deadline()
		if !errors.Is(err, context.DeadlineExceeded) || owner.Err() != nil || !ok || !deadline.Equal(wantDeadline) || claimed.Value(exitContextKey{}) != "original" || controller.claimCalls != 1 {
			t.Fatal("Claim escaped its earlier owner deadline or terminal stop erased the timeout", err)
		}
	})
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
		{"complete-empty-claim", http.StatusOK, `{"delivery":null}`, nil},
		{"lost-claim-response", 0, "", controlclient.ErrPostOutcomeUnobserved},
		{"parent-cancel", 0, "", controlclient.ErrPostOutcomeUnobserved},
		{"invalid-terminal", 0, "", controlclient.ErrPostOutcomeUnobserved},
		{"commit-outcome-unknown", http.StatusServiceUnavailable, `{"error":"commit_outcome_unknown: read back exact identity"}`, controlclient.ErrCommitOutcomeUnknown},
		{"external-effect-unresolved", http.StatusServiceUnavailable, `{"error":"external_effect_unresolved: read back original operation"}`, controlclient.ErrExternalEffectUnresolved},
		{"complete-conflict", http.StatusConflict, `{"error":"conflict"}`, nil},
		{"malformed-success", http.StatusOK, `{"delivery":`, controlclient.ErrPostOutcomeUnobserved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ready, handlerDone, response := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(response) }) }
			defer release()
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
				close(ready)
				select {
				case <-response:
				case <-r.Context().Done():
					return
				case <-ctx.Done():
					return
				}
				if tc.name == "lost-claim-response" {
					// A real connection cut is still unknown even after a valid
					// terminal event. It is not replaced with an empty success.
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					connection.Close()
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
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
					return errors.New("Claim lost its existing bounded deadline")
				}
				var response struct {
					Delivery *struct{} `json:"delivery"`
				}
				original = client.Call(ctx, http.MethodPost, "/api/v1/worker/codex/controls/claim", struct{}{}, &response)
				return original
			}
			value, err := observeTerminalClaim(t, controller, ready, release, tc.name)
			if tc.name == "complete-empty-claim" {
				if original != nil || err != nil {
					t.Fatal("valid terminal lost the complete empty Claim response", original, err)
				}
			} else if original == nil || !errors.Is(err, original) || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("terminal completion erased the real POST error: got=%v original=%v want=%v", err, original, tc.want)
			}
			if (tc.name == "parent-cancel" || tc.name == "invalid-terminal") && !errors.Is(err, context.Canceled) {
				t.Fatal("POST cancellation cause was lost", err)
			}
			if tc.name == "lost-claim-response" && (!errors.Is(err, io.EOF) || errors.Is(err, context.Canceled)) {
				t.Fatal("lost transport reply was replaced with owned cancellation", err)
			}
			if tc.name == "invalid-terminal" && !errors.Is(err, ErrLifecycle) {
				t.Fatal("invalid terminal error was lost", err)
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
			if tc.name != "complete-empty-claim" {
				controller.closeError = fixtureCloseFailure
			}
			_, settled := settleEngineeringExit(ctx, nil, exitTestProof(), controller, true, value.Status, err)
			if tc.name == "complete-empty-claim" {
				if settled != nil {
					t.Fatal("complete empty Claim and successful Close did not retain success", settled)
				}
			} else if !errors.Is(settled, original) || !errors.Is(settled, fixtureCloseFailure) || !errors.Is(settled, ErrEngineeringCloseUnobserved) {
				t.Fatalf("Close erased the original claim outcome or retried: %v", settled)
			}
			wantStatus := "completed"
			if tc.name == "invalid-terminal" {
				wantStatus = "unknown"
			}
			if controller.closeCalls != 1 || controller.status != wantStatus || controller.invalidCloseContext {
				t.Fatalf("Close lost its original identity/lifecycle or replayed: %v", settled)
			}
		})
	}
}
