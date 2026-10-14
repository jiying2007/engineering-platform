package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type publisherLifecycleGuard struct{}

func (publisherLifecycleGuard) CheckRunEpoch(ctx context.Context, _ string, _ uint64) error {
	return ctx.Err()
}
func (publisherLifecycleGuard) CheckRecoveryEpoch(ctx context.Context, _ uint64, _ action.RiskClass) error {
	return ctx.Err()
}

type publisherLifecycleEffect struct {
	entered chan context.Context
	release <-chan struct{}
	calls   atomic.Int32
}

func (p *publisherLifecycleEffect) Dispatch(ctx context.Context, _ action.Request) (action.DispatchResult, error) {
	p.calls.Add(1)
	if p.entered != nil {
		p.entered <- ctx
		<-p.release
	}
	// Controlled local success after cancellation tests the real Action
	// settlement, not a live GitHub effect or dispatcher-quiescence proof.
	return action.DispatchResult{Outcome: action.DispatchConfirmed, ExternalRef: "local-only-completed", ObservedState: "known local result"}, nil
}
func (*publisherLifecycleEffect) Reconcile(context.Context, action.Operation) (action.ReconcileResult, error) {
	return action.ReconcileResult{}, errors.New("unexpected retry/observation")
}

func publisherLifecycleListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}
func publisherLifecycleResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not join serving/drain")
		return nil
	}
}

func TestPublisherLifecycleRequestStopAndOriginalSettlement(t *testing.T) {
	for _, phase := range []string{"before-effect", "after-original-effect-entered"} {
		t.Run(phase, func(t *testing.T) {
			parent, stop := context.WithCancel(context.Background())
			defer stop()
			type key struct{}
			parent = context.WithValue(parent, key{}, "publisher-root")
			entered := make(chan context.Context, 1)
			draining, release := make(chan struct{}), make(chan struct{})
			var unblock sync.Once
			defer unblock.Do(func() { close(release) })
			effect := &publisherLifecycleEffect{release: release}
			if phase == "after-original-effect-entered" {
				effect.entered = entered
			}
			repository := action.NewMemoryRepository()
			service := action.NewService(action.AllowCapabilities{"local.dispatch": true}, publisherLifecycleGuard{}, effect, repository)
			request := action.Request{ID: "stop-operation", RunID: "local-run", ExecutionEpoch: 1, Action: "local.dispatch", Capability: "local.dispatch", RiskClass: action.ControlledMutation, IdempotencyKey: "original-only", RequestedBy: "local-fixture"}
			results := make(chan struct {
				receipt action.Receipt
				err     error
			}, 1)
			pki := testsupport.NewPKI(t)
			server := &http.Server{TLSConfig: pki.ServerTLS(), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
					t.Error("not an authenticated mTLS request")
				}
				if phase == "before-effect" {
					entered <- r.Context()
					<-release
				}
				receipt, err := service.Execute(r.Context(), request)
				results <- struct {
					receipt action.Receipt
					err     error
				}{receipt, err}
				_, _ = io.WriteString(w, "drained")
			})}
			server.RegisterOnShutdown(func() { close(draining) })
			listener := publisherLifecycleListener(t)
			done, response := make(chan error, 1), make(chan error, 1)
			go func() { done <- runPublisherServer(parent, server, listener) }()
			client := pki.Client(t, "urn:engineering-platform:control:local-lifecycle")
			go func() {
				r, err := client.Get("https://" + listener.Addr().String())
				if err == nil {
					_, err = io.Copy(io.Discard, r.Body)
					err = errors.Join(err, r.Body.Close())
				}
				response <- err
			}()
			var original context.Context
			select {
			case original = <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach controlled boundary")
			}
			stop()
			select {
			case <-draining:
			case <-time.After(5 * time.Second):
				t.Fatal("publisher did not begin drain")
			}
			if !errors.Is(original.Err(), context.Canceled) {
				t.Errorf("publisher request still live during service drain: %v", original.Err())
			}
			if original.Value(key{}) != "publisher-root" {
				t.Error("publisher request lost service values")
			}
			select {
			case err := <-done:
				t.Errorf("publisher exited before original handler settled: %v", err)
			default:
			}
			unblock.Do(func() { close(release) })
			if err := publisherLifecycleResult(t, response); err != nil {
				t.Fatal(err)
			}
			if err := publisherLifecycleResult(t, done); err != nil {
				t.Fatal(err)
			}
			result := <-results
			retained, readErr := repository.GetContext(context.Background(), request.ID)
			if phase == "before-effect" {
				if !errors.Is(result.err, context.Canceled) || result.receipt.OperationID != "" || effect.calls.Load() != 0 || !errors.Is(readErr, action.ErrOperationAbsent) {
					t.Errorf("service stop admitted new effect: receipt=%+v error=%v calls=%d read=%v", result.receipt, result.err, effect.calls.Load(), readErr)
				}
			} else {
				if result.err != nil || result.receipt.Result != string(action.Confirmed) || readErr != nil || retained.State != action.Confirmed || effect.calls.Load() != 1 {
					t.Fatalf("caller cancellation erased original completion: %+v %v retained=%+v read=%v", result.receipt, result.err, retained, readErr)
				}
				again, err := service.Execute(context.Background(), request)
				if err != nil || again.OperationID != result.receipt.OperationID || effect.calls.Load() != 1 {
					t.Error("retained original completion was replayed")
				}
			}
		})
	}
}

type publisherLifecycleFaultListener struct {
	net.Listener
	fault error
}

func (l *publisherLifecycleFaultListener) Accept() (net.Conn, error) { return nil, l.fault }

func TestPublisherLifecyclePreservesMixedListenerFailure(t *testing.T) {
	pki := testsupport.NewPKI(t)
	fault := errors.New("independent publisher listener failure")
	listener := &publisherLifecycleFaultListener{publisherLifecycleListener(t), errors.Join(http.ErrServerClosed, fault)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runPublisherServer(ctx, &http.Server{TLSConfig: pki.ServerTLS()}, listener)
	if !errors.Is(err, fault) {
		t.Fatalf("publisher swallowed joined listener failure: %v", err)
	}
}

func TestPublisherLifecyclePreCancelledStartsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fault := errors.New("listener must not be called")
	listener := &publisherLifecycleFaultListener{publisherLifecycleListener(t), fault}
	if err := runPublisherServer(ctx, &http.Server{}, listener); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled service started: %v", err)
	}
}

// Fail the next Accept after a real mTLS request is already active. The
// listener failure must not let the command exit before handler settlement.
type publisherLateFaultListener struct {
	net.Listener
	first bool
	fail  <-chan struct{}
	fault error
}

func (l *publisherLateFaultListener) Accept() (net.Conn, error) {
	if !l.first {
		l.first = true
		return l.Listener.Accept()
	}
	<-l.fail
	return nil, l.fault
}
func TestPublisherLifecycleListenerFailureCancelsAndDrainsActiveRequest(t *testing.T) {
	pki := testsupport.NewPKI(t)
	fault := errors.New("listener unavailable after admission")
	fail, release, draining := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	entered := make(chan context.Context, 1)
	server := &http.Server{TLSConfig: pki.ServerTLS(), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- r.Context()
		<-release
		_, _ = io.WriteString(w, "drained")
	})}
	server.RegisterOnShutdown(func() { close(draining) })
	listener := &publisherLateFaultListener{Listener: publisherLifecycleListener(t), fail: fail, fault: fault}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done, response := make(chan error, 1), make(chan error, 1)
	go func() { done <- runPublisherServer(ctx, server, listener) }()
	client := pki.Client(t, "urn:engineering-platform:control:local-lifecycle")
	go func() {
		r, err := client.Get("https://" + listener.Addr().String())
		if err == nil {
			_, err = io.Copy(io.Discard, r.Body)
			err = errors.Join(err, r.Body.Close())
		}
		response <- err
	}()
	var request context.Context
	select {
	case request = <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	close(fail)
	select {
	case <-draining:
	case err := <-done:
		unblock.Do(func() { close(release) })
		_ = publisherLifecycleResult(t, response)
		t.Fatalf("publisher returned on listener failure without draining active handler: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("listener failure did not start drain")
	}
	if !errors.Is(request.Err(), context.Canceled) {
		t.Error("listener failure left request active")
	}
	if ctx.Err() != nil {
		t.Error("child failure cancelled its parent")
	}
	select {
	case err := <-done:
		t.Errorf("returned before handler drain: %v", err)
	default:
	}
	unblock.Do(func() { close(release) })
	if err := publisherLifecycleResult(t, response); err != nil {
		t.Fatal(err)
	}
	if err := publisherLifecycleResult(t, done); !errors.Is(err, fault) {
		t.Fatalf("listener error lost after drain: %v", err)
	}
}
