package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

const monitorTestSession = "0123456789abcdef0123456789abcdef"

var monitorTestEndpoint = "sha256:" + strings.Repeat("a", 64)

func monitorPayload(t *testing.T, at time.Time, epoch uint64, alert bool) []byte {
	t.Helper()
	s := production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: at, RecoveryEpoch: epoch, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}}
	if alert {
		old := at.Add(-time.Hour)
		s.PendingWorkerIntents = 1
		s.OldestPendingWorkerAt = &old
	}
	status, err := production.EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestProductionMonitorOptionsPreserveSingleWindow(t *testing.T) {
	args := []string{"--monitor-for", "6s", "--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m", "--stale-after", "4s", "--require-no-alert"}
	o, err := parseProductionStatus(args)
	if err != nil || !o.monitor || !o.observe || !o.requireNoAlert || o.monitorPolicy.WindowLimit() != 2 {
		t.Fatal(o, err)
	}
	for _, bad := range [][]string{
		{"--monitor-for", "6s"},
		{"--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m", "--stale-after", "4s"},
		{"--monitor-for", "6s", "--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m"},
		append(append([]string{}, args...), "--require-ready"), append(append([]string{}, args...), "--monitor-for=9s"),
	} {
		if _, err := parseProductionStatus(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	for _, value := range []string{"0s", "2s", "2.5s", "25h"} {
		bad := append([]string{}, args...)
		bad[1] = value
		if _, err := parseProductionStatus(bad); err == nil {
			t.Fatal("accepted monitor duration", value)
		}
	}
	if o, err := parseProductionStatus([]string{"--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m"}); err != nil || o.monitor || !o.observe {
		t.Fatal("single-window behavior changed", o, err)
	}
}

func TestProductionMonitorFixedScheduleFailureRecoveryAndTerminalBoundaries(t *testing.T) {
	for _, mode := range []string{"clear", "read-failure", "unauthorized", "invalid-json", "timeout", "monitor-deadline", "epoch-change", "mode-change", "cancel", "late-slot", "output-failure", "cancel-before"} {
		t.Run(mode, func(t *testing.T) {
			p := production.ObserverPolicy{Progress: production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60}, DurationSeconds: 9, StaleAfterSeconds: 4}
			now := time.Now().Add(time.Hour)
			start := now
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel-before" {
				cancel()
			}
			var events []production.ObserverEvent
			calls := 0
			reader := observationReaderFunc(func(c context.Context, m, path string, b []byte) ([]byte, error) {
				calls++
				if m != http.MethodGet || path != "/api/v1/operations/status" || b != nil {
					t.Fatal("unexpected mutation or endpoint")
				}
				if _, ok := c.Deadline(); !ok {
					t.Fatal("unbounded source read")
				}
				if calls == 2 {
					switch mode {
					case "read-failure":
						return nil, fmt.Errorf("PRIVATE-transport-secret")
					case "unauthorized":
						return nil, &controlclient.HTTPError{Status: 403}
					case "invalid-json":
						return []byte(`{"ready":true}`), nil
					case "timeout":
						return nil, context.DeadlineExceeded
					case "monitor-deadline":
						now = start.Add(time.Duration(p.DurationSeconds) * time.Second)
						return nil, context.DeadlineExceeded
					case "cancel":
						cancel()
						return nil, context.Canceled
					case "epoch-change":
						return monitorPayload(t, now, 2, false), nil
					case "mode-change":
						raw := monitorPayload(t, now, 1, false)
						var s production.OperationalStatus
						if err := json.Unmarshal(raw, &s); err != nil {
							t.Fatal(err)
						}
						s.Snapshot.RecoveryMode = "RECOVERY_RECONCILIATION"
						s, err := production.EvaluateSnapshot(s.Snapshot)
						if err != nil {
							t.Fatal(err)
						}
						raw, _ = json.Marshal(s)
						return raw, nil
					}
				}
				return monitorPayload(t, now, 1, false), nil
			})
			wait := func(c context.Context, at time.Time) error {
				if at.After(now) {
					now = at
				}
				if mode == "late-slot" && at.Equal(start.Add(3*time.Second)) {
					now = at.Add(2 * time.Second)
				}
				return c.Err()
			}
			emit := func(e production.ObserverEvent) error {
				events = append(events, e)
				if mode == "output-failure" && len(events) == 2 {
					return errors.New("OUTPUT_FAILED")
				}
				return nil
			}
			had, err := collectProductionMonitorWithClock(ctx, reader, p, monitorTestSession, monitorTestEndpoint, func() time.Time { return now }, wait, emit)
			switch mode {
			case "cancel-before":
				if !errors.Is(err, context.Canceled) || calls != 0 || len(events) != 0 {
					t.Fatal(calls, events, err)
				}
				return
			case "clear":
				if err != nil || had || calls != 9 || len(events) != 5 {
					t.Fatal(calls, had, len(events), err)
				}
			case "epoch-change", "mode-change":
				if !errors.Is(err, production.ErrObserverIdentityChanged) || calls != 2 || len(events) != 3 || events[1].Reason != "RECOVERY_IDENTITY_CHANGED" {
					t.Fatal(calls, events, err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) || calls != 2 || events[len(events)-1].Reason != "MONITOR_CANCELLED" {
					t.Fatal(calls, events, err)
				}
			case "monitor-deadline":
				if !errors.Is(err, context.DeadlineExceeded) || !had || calls != 2 || len(events) != 3 || events[1].Reason != "MONITOR_DEADLINE" || events[2].Reason != "MONITOR_DEADLINE" || !events[2].Stopped {
					t.Fatal(calls, events, err)
				}
			case "output-failure":
				if err == nil || calls != 3 || len(events) != 2 {
					t.Fatal(calls, events, err)
				}
			case "late-slot":
				if err != nil || !had || calls != 6 || events[2].Reason != "MISSED_WINDOW" || events[3].Kind != "RECOVERED" {
					t.Fatal(calls, events, err)
				}
			default:
				if err != nil || !had || calls != 8 || len(events) != 5 || events[1].Kind != "FAILED" || events[2].Kind != "RECOVERED" || events[4].FailedWindows != 1 {
					t.Fatal(calls, events, err)
				}
			}
			if mode == "unauthorized" && events[1].Reason != "SOURCE_UNAUTHORIZED" {
				t.Fatal(events[1])
			}
			if mode == "timeout" && events[1].Reason != "WINDOW_TIMEOUT" {
				t.Fatal(events[1])
			}
			for _, e := range events {
				if e.Ready || e.ProductionQualified || e.ExecutionAuthorized || e.CapacityObserved {
					t.Fatal("observer granted authority")
				}
				b, _ := json.Marshal(e)
				if bytes.Contains(b, []byte("PRIVATE-transport-secret")) {
					t.Fatal("transport diagnostic leaked")
				}
			}
		})
	}
}

type monitorWriterFunc func([]byte) (int, error)

func (f monitorWriterFunc) Write(b []byte) (int, error) { return f(b) }

func TestProductionMonitorOutputIsBoundedAndWriteFailuresStop(t *testing.T) {
	e := production.ObserverEvent{Version: 1, Kind: "STARTED"}
	for _, writer := range []io.Writer{
		monitorWriterFunc(func(b []byte) (int, error) { return 0, errors.New("PRIVATE-output-secret") }),
		monitorWriterFunc(func(b []byte) (int, error) { return len(b) - 1, nil }),
	} {
		if err := observerJSONEmitter(writer)(e); err == nil || strings.Contains(err.Error(), "PRIVATE-output-secret") {
			t.Fatal("output failure missing or leaking", err)
		}
	}
	var writes int
	emit := observerJSONEmitter(monitorWriterFunc(func(b []byte) (int, error) { writes++; return len(b), nil }))
	e.Reason = strings.Repeat("x", maxObserverEventBytes)
	if err := emit(e); err == nil || writes != 0 {
		t.Fatal("oversized event written", writes, err)
	}
	e.Reason = strings.Repeat("x", 1<<20)
	for i := 0; i < maxObserverOutputBytes/(1<<20)+1; i++ {
		if err := emit(e); err != nil {
			if writes < 100 || writes >= 128 {
				t.Fatal("unexpected total output bound", writes)
			}
			return
		}
	}
	t.Fatal("total output was not bounded")
}

type monitorCLIStore struct {
	*store.Memory
	calls     atomic.Int32
	failFirst bool
}

func (s *monitorCLIStore) ReadOperationalStatus(context.Context) (production.OperationalStatus, error) {
	if s.calls.Add(1) == 1 && s.failFirst {
		return production.OperationalStatus{}, fmt.Errorf("PRIVATE_MONITOR_FAILURE")
	}
	return production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: time.Now().UTC(), RecoveryEpoch: 3, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}})
}

// Exercise the actual compiled command and unchanged authenticated Core
// middleware. The source Store is explicitly synthetic, never PostgreSQL or
// a claimed production service. No private inputs are retained.
func TestProductionMonitorCLIThroughAuthenticatedCore(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "eng")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, goBinary, "build", "-trimpath", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v %s", err, out)
	}
	for _, mode := range []string{"clear", "failure-recovery", "denied", "sigterm"} {
		t.Run(mode, func(t *testing.T) {
			pki := testsupport.NewPKI(t)
			readSubject, deniedSubject := "urn:engineering-platform:operator:monitor-reader", "urn:engineering-platform:operator:monitor-no-read"
			policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: readSubject, Scope: "platform", Capabilities: []string{access.Read}}, {Subject: deniedSubject, Scope: "platform", Capabilities: []string{access.WorkCreate}}}})
			if err != nil {
				t.Fatal(err)
			}
			backend := &monitorCLIStore{Memory: store.NewMemory(), failFirst: mode == "failure-recovery"}
			handler, err := api.NewAuthenticatedHandler(backend, nil, policy, api.AuthenticatedOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/status" {
					t.Error("unexpected mutation or endpoint")
				}
				handler.ServeHTTP(w, r)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			subject := readSubject
			if mode == "denied" {
				subject = deniedSubject
			}
			cert := pki.ClientCertificate(t, subject)
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			inputs := map[string][]byte{"ca.pem": pki.CAPEM, "client.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), "client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})}
			for name, raw := range inputs {
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			duration := "3s"
			if mode == "failure-recovery" || mode == "sigterm" {
				duration = "6s"
			}
			command := exec.CommandContext(ctx, bin, "production-status", "--monitor-for", duration, "--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m", "--stale-after", "4s", "--require-no-alert")
			command.Env = []string{"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + server.URL, "CONTROL_CLIENT_CERT_FILE=" + filepath.Join(dir, "client.pem"), "CONTROL_CLIENT_KEY_FILE=" + filepath.Join(dir, "client.key"), "CONTROL_SERVER_CA_FILE=" + filepath.Join(dir, "ca.pem"), "HTTPS_PROXY=http://127.0.0.1:1"}
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if mode == "sigterm" {
				if err = command.Start(); err != nil {
					t.Fatal(err)
				}
				defer command.Process.Kill()
				limit := time.Now().Add(2 * time.Second)
				for calls.Load() == 0 && time.Now().Before(limit) {
					time.Sleep(5 * time.Millisecond)
				}
				if calls.Load() == 0 {
					t.Fatal("monitor did not read Core")
				}
				if err = command.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
				err = command.Wait()
			} else {
				err = command.Run()
			}
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			wantCode, wantCalls := 1, 1
			if mode == "clear" {
				wantCode, wantCalls = 0, 3
			}
			if mode == "failure-recovery" {
				wantCalls = 4
			}
			if code != wantCode || int(calls.Load()) != wantCalls {
				t.Fatalf("mode %s: exit=%d calls=%d want=%d/%d stderr=%s", mode, code, calls.Load(), wantCode, wantCalls, stderr.String())
			}
			if mode == "denied" && backend.calls.Load() != 0 {
				t.Fatal("denied reader reached Store")
			}
			var events []production.ObserverEvent
			decoder := json.NewDecoder(&stdout)
			for {
				var e production.ObserverEvent
				if err := decoder.Decode(&e); err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				events = append(events, e)
			}
			wantEvents := 3
			if mode == "failure-recovery" {
				wantEvents = 4
			}
			if len(events) != wantEvents || events[0].Kind != "STARTED" || !events[len(events)-1].Stopped {
				t.Fatal("incomplete observer journal", events)
			}
			if mode == "failure-recovery" && (events[1].Kind != "FAILED" || events[2].Kind != "RECOVERED" || !events[3].HadAlert) {
				t.Fatal("failure erased by recovery", events)
			}
			if mode == "denied" && events[1].Reason != "SOURCE_UNAUTHORIZED" {
				t.Fatal("authorization failure omitted", events)
			}
			if mode == "sigterm" && events[len(events)-1].Reason != "MONITOR_CANCELLED" {
				t.Fatal("cancellation omitted", events)
			}
			digest := sha256.Sum256([]byte(server.URL))
			endpoint := "sha256:" + hex.EncodeToString(digest[:])
			for _, e := range events {
				if _, err := production.EvaluateObserverFreshness(e, events[0].SessionID, endpoint, e.RecordedAt); err != nil {
					t.Fatal("retained CLI event invalid", err)
				}
			}
			combined := append(append([]byte{}, stdout.Bytes()...), stderr.Bytes()...)
			// stdout was consumed by the decoder; marshal retained event structures
			// to check the actual decoded content as well as the stderr stream.
			raw, _ := json.Marshal(events)
			combined = append(combined, raw...)
			for _, secret := range [][]byte{inputs["client.key"], []byte(dir), []byte("PRIVATE KEY"), []byte("PRIVATE_MONITOR_FAILURE"), []byte(server.URL)} {
				if bytes.Contains(combined, secret) {
					t.Fatal("private input or raw endpoint leaked")
				}
			}
		})
	}
}
