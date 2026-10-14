package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

// RegisterOnShutdown is a deterministic barrier: request-scoped work must be
// cancelled BEFORE the service drains. The HTTP client remains connected.
func TestControlLifecycleRequestCancellationBeforeDrain(t *testing.T) {
	for _, reason := range []string{"parent-stop", "relay-failure"} {
		t.Run(reason, func(t *testing.T) {
			parent, stop := context.WithCancel(context.Background())
			defer stop()
			type key struct{}
			parent = context.WithValue(parent, key{}, "original-service-value")
			fault := errors.New("relay stopped independently")
			entered := make(chan context.Context, 1)
			draining, release, failRelay := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			writes := make(chan error, 1)
			store := corestore.NewMemory()
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				entered <- r.Context()
				<-release
				writes <- store.CreateWorkContext(r.Context(), core.WorkItem{ID: "after-stop", Title: "must not enter", HumanOwner: "local-test", State: core.WorkDraft, CreatedAt: time.Now().UTC()})
				_, _ = io.WriteString(w, "drained")
			})}
			server.RegisterOnShutdown(func() { close(draining) })
			listener := lifecycleListener(t)
			done := make(chan error, 1)
			var relay func(context.Context) error
			if reason == "relay-failure" {
				relay = func(ctx context.Context) error {
					select {
					case <-failRelay:
						return fault
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			go func() { done <- runControlServer(parent, server, listener, true, relay) }()
			client := &http.Client{Timeout: 5 * time.Second}
			defer client.CloseIdleConnections()
			response := make(chan error, 1)
			go func() {
				r, err := client.Get("http://" + listener.Addr().String())
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
				t.Fatal("request not admitted")
			}
			if reason == "parent-stop" {
				stop()
			} else {
				close(failRelay)
			}
			select {
			case <-draining:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not start")
			}
			if !errors.Is(request.Err(), context.Canceled) {
				t.Errorf("request stayed live after service stop: %v", request.Err())
			}
			if request.Value(key{}) != "original-service-value" {
				t.Error("HTTP request lost service root values")
			}
			select {
			case err := <-done:
				t.Errorf("returned before request drain: %v", err)
			default:
			}
			releaseOnce.Do(func() { close(release) })
			if err := lifecycleResult(t, writes); !errors.Is(err, context.Canceled) {
				t.Errorf("stopped service admitted new authority write: %v", err)
			}
			if err := lifecycleResult(t, response); err != nil {
				t.Fatal(err)
			}
			err := lifecycleResult(t, done)
			if reason == "relay-failure" && !errors.Is(err, fault) {
				t.Errorf("lost relay failure: %v", err)
			}
			if reason == "parent-stop" && err != nil {
				t.Errorf("cooperative stop failed: %v", err)
			}
			if _, err := store.GetWorkContext(context.Background(), "after-stop"); err == nil {
				t.Error("new Work survived service cancellation")
			}
		})
	}
}
