package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func lifecycleListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func lifecycleResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Control did not join its service and relay")
		return nil
	}
}

func TestControlLifecycleMixedRelayFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fault := errors.New("independent relay failure")
	err := runControlServer(ctx, &http.Server{}, lifecycleListener(t), true, func(context.Context) error {
		return errors.Join(context.Canceled, fault)
	})
	if !errors.Is(err, fault) {
		t.Fatalf("real relay failure was swallowed: %v", err)
	}
}

func TestControlLifecycleRelayCleanupFailureAfterCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fault := errors.New("relay cleanup failed")
	started := make(chan struct{})
	done := make(chan error, 1)
	listener := lifecycleListener(t)
	go func() {
		done <- runControlServer(ctx, &http.Server{}, listener, true, func(run context.Context) error {
			close(started)
			<-run.Done()
			return errors.Join(fmt.Errorf("stopping: %w", run.Err()), fault)
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("relay did not start")
	}
	cancel()
	if err := lifecycleResult(t, done); !errors.Is(err, fault) {
		t.Fatalf("post-cancel cleanup failure was lost: %v", err)
	}
}

func TestControlLifecycleUnexpectedRelayCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runControlServer(ctx, &http.Server{}, lifecycleListener(t), true, func(context.Context) error {
		return fmt.Errorf("unexpected independent cancellation: %w", context.Canceled)
	})
	if ctx.Err() != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("unrequested relay cancellation became success: %v (parent=%v)", err, ctx.Err())
	}
}

// A bound listener returning an injected error before shutdown begins.
// This checks the error reported by the actual net/http serving goroutine.
type lifecycleFaultListener struct {
	net.Listener
	fault error
}

func (l *lifecycleFaultListener) Accept() (net.Conn, error) {
	return nil, l.fault
}

func TestControlLifecycleMixedServerFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fault := errors.New("independent listener failure")
	listener := &lifecycleFaultListener{Listener: lifecycleListener(t), fault: errors.Join(http.ErrServerClosed, fault)}
	err := runControlServer(ctx, &http.Server{}, listener, true, nil)
	if !errors.Is(err, fault) {
		t.Fatalf("real HTTP failure was swallowed: %v", err)
	}
}

func TestControlLifecycleWrappedCooperativeCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	listener := lifecycleListener(t)
	go func() {
		done <- runControlServer(ctx, &http.Server{}, listener, true, func(run context.Context) error {
			close(started)
			<-run.Done()
			return errors.Join(fmt.Errorf("relay: %w", run.Err()), context.Canceled)
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("relay did not start")
	}
	cancel()
	if err := lifecycleResult(t, done); err != nil {
		t.Fatalf("pure requested cancellation became failure: %v", err)
	}
}

func TestControlLifecycleGracefulRequestDrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started, draining, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "drained")
	})}
	server.RegisterOnShutdown(func() { close(draining) })
	listener := lifecycleListener(t)
	done := make(chan error, 1)
	go func() { done <- runControlServer(ctx, server, listener, true, nil) }()
	response := make(chan error, 1)
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	go func() {
		r, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			b, readErr := io.ReadAll(r.Body)
			_ = r.Body.Close()
			err = readErr
			if err == nil && string(b) != "drained" {
				err = fmt.Errorf("unexpected response %q", b)
			}
		}
		response <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case <-draining:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before the in-flight request completed: %v", err)
	default:
	}
	unblock.Do(func() { close(release) })
	if err := lifecycleResult(t, response); err != nil {
		t.Fatal(err)
	}
	if err := lifecycleResult(t, done); err != nil {
		t.Fatalf("graceful request drain failed: %v", err)
	}
}

type lifecycleOpaqueCancel struct{}

func (lifecycleOpaqueCancel) Error() string        { return "opaque failure" }
func (lifecycleOpaqueCancel) Is(target error) bool { return target == context.Canceled }

type lifecycleCycle struct{}

func (*lifecycleCycle) Error() string   { return "cyclic failure" }
func (e *lifecycleCycle) Unwrap() error { return e }

func TestControlStopErrorClassification(t *testing.T) {
	fault := errors.New("database failure")
	cases := []struct {
		name     string
		err      error
		expected error
		ok       bool
	}{
		{"nil is not a stop error", nil, context.Canceled, false},
		{"direct", context.Canceled, context.Canceled, true},
		{"wrapped", fmt.Errorf("wrapped: %w", context.Canceled), context.Canceled, true},
		{"joined expected", errors.Join(context.Canceled, fmt.Errorf("wrapped: %w", context.Canceled)), context.Canceled, true},
		{"mixed", errors.Join(context.Canceled, fault), context.Canceled, false},
		{"nested mixed", fmt.Errorf("wrapped: %w", errors.Join(context.Canceled, fault)), context.Canceled, false},
		{"foreign stop", errors.Join(context.Canceled, http.ErrServerClosed), context.Canceled, false},
		{"opaque Is is not proof", lifecycleOpaqueCancel{}, context.Canceled, false},
		{"cycle", &lifecycleCycle{}, context.Canceled, false},
		{"server", fmt.Errorf("wrapped: %w", http.ErrServerClosed), http.ErrServerClosed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := onlyControlStopError(tc.err, tc.expected); got != tc.ok {
				t.Fatalf("onlyControlStopError(%v)=%v, want %v", tc.err, got, tc.ok)
			}
		})
	}
}
