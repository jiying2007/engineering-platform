package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type observationReaderFunc func(context.Context, string, string, []byte) ([]byte, error)

func (f observationReaderFunc) Raw(c context.Context, m, p string, b []byte) ([]byte, error) {
	return f(c, m, p, b)
}

func observerPayload(t *testing.T, now, old time.Time) []byte {
	t.Helper()
	s, e := production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL", PendingWorkerIntents: 1000, OldestPendingWorkerAt: &old})
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestProductionObservationCLIOptionsRemainNarrow(t *testing.T) {
	good := []string{"--observe-for", "10s", "--interval", "5s", "--max-pending-age", "2m", "--require-no-alert"}
	o, e := parseProductionStatus(good)
	if e != nil || !o.observe || !o.requireNoAlert || o.policy.WindowSeconds != 10 {
		t.Fatal(o, e)
	}
	for _, args := range [][]string{{}, {"--require-ready"}, {"--require-authority-clear"}} {
		if o, e := parseProductionStatus(args); e != nil || o.observe {
			t.Fatal(args, o, e)
		}
	}
	bad := [][]string{
		{"--observe-for", "10s"}, {"--interval", "5s"}, {"--max-pending-age", "2m"}, {"--require-no-alert"},
		{"--observe-for", "1s", "--interval", "1s", "--max-pending-age", "1m"},
		{"--observe-for", "2.5s", "--interval", "1s", "--max-pending-age", "1m"},
		{"--observe-for", "10s", "--interval", "3s", "--max-pending-age", "1m"},
		{"--observe-for", "10s", "--interval", "5s", "--max-pending-age", "0s"},
		{"--observe-for", "300s", "--interval", "1s", "--max-pending-age", "1m"},
		{"--require-ready", "--require-ready=false"},
	}
	for _, extra := range [][]string{{"--require-ready"}, {"--require-authority-clear=false"}, {"--execute"}, {"--repair"}, {"--identity", "owner"}, {"--observe-for=20s"}, {"--interval=1s"}, {"--max-pending-age=1s"}, {"unexpected"}} {
		bad = append(bad, append(append([]string{}, good...), extra...))
	}
	for _, args := range bad {
		if _, err := parseProductionStatus(args); err == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestProductionObservationAbortsWithoutRetryOrPartialSuccess(t *testing.T) {
	for _, mode := range []string{"success", "request-error", "bad-json", "forged-ready", "duplicate-json", "cancel-before", "cancel-during", "late-slot", "request-too-slow", "repeated-snapshot"} {
		t.Run(mode, func(t *testing.T) {
			p := production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60}
			now := time.Now().Add(time.Second)
			start := now
			old := now.Add(-time.Hour)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel-before" {
				cancel()
			}
			calls := 0
			reader := observationReaderFunc(func(c context.Context, m, path string, b []byte) ([]byte, error) {
				calls++
				if m != http.MethodGet || path != "/api/v1/operations/status" || b != nil {
					t.Fatal("unexpected side effect", m, path)
				}
				if _, ok := c.Deadline(); !ok {
					t.Fatal("unbounded request")
				}
				if calls == 2 {
					switch mode {
					case "request-error":
						return nil, fmt.Errorf("TEST reply unavailable")
					case "bad-json":
						return []byte("{"), nil
					case "forged-ready":
						return []byte(`{"ready":true}`), nil
					case "duplicate-json":
						return []byte(`{"ready":false,"ready":true}`), nil
					case "cancel-during":
						cancel()
					case "request-too-slow":
						now = now.Add(2 * time.Second)
					}
				}
				at := now
				if mode == "repeated-snapshot" {
					at = start
				}
				return observerPayload(t, at, old), nil
			})
			wait := func(c context.Context, at time.Time) error {
				now = at
				if mode == "late-slot" && at.After(start) {
					now = now.Add(time.Second)
				}
				return c.Err()
			}
			r, err := collectProductionProgressWithClock(ctx, reader, p, func() time.Time { return now }, wait)
			if mode == "success" {
				if err != nil || calls != 3 || !r.Complete || !r.DiagnosticAlert || r.Ready {
					t.Fatal(calls, r, err)
				}
				return
			}
			if err == nil || r.Complete || len(r.Observations) != 0 {
				t.Fatal("incomplete result became success", mode, r, err)
			}
			expected := 2
			if mode == "cancel-before" {
				expected = 0
			}
			if mode == "late-slot" {
				expected = 1
			}
			if calls != expected {
				t.Fatal("unexpected retries/burst", mode, calls, expected)
			}
		})
	}
}

func TestProductionObservationRealMTLSFiniteReadOnlyWindow(t *testing.T) {
	pki := testsupport.NewPKI(t)
	var calls atomic.Int32
	old := time.Now().UTC().Add(-time.Hour)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/status" || r.ContentLength > 0 || r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("unexpected or unauthenticated request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(observerPayload(t, time.Now().UTC(), old))
	}))
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	cert := pki.ClientCertificate(t, "urn:engineering-platform:operator:observation-test")
	client, err := controlclient.New(server.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	r, err := collectProductionProgress(ctx, client, production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || !r.Complete || !r.DiagnosticAlert || r.WorkerQueue.State != "AGED_BACKLOG_NO_PROGRESS_MARKER" || r.Ready || r.CapacityObserved || r.ProductionQualified || r.ExecutionAuthorized {
		t.Fatal("incorrect mTLS observation", calls.Load(), r)
	}
}

func TestObservationTimerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitObservation(ctx, time.Now().Add(time.Hour)) == nil {
		t.Fatal("wait ignored cancellation")
	}
}
