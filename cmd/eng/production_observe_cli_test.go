package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type observationStore struct {
	*store.Memory
	mode         string
	calls        atomic.Int32
	initial, old time.Time
}

func (s *observationStore) ReadOperationalStatus(context.Context) (production.OperationalStatus, error) {
	i := s.calls.Add(1)
	snapshot := production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: time.Now().UTC(), RecoveryMode: "NORMAL", RecoveryEpoch: 1, WorkerPolls: []production.WorkerPollObservation{}}
	if s.mode != "idle" {
		snapshot.PendingWorkerIntents = 1000
		snapshot.OldestPendingWorkerAt = &s.old
	}
	if s.mode == "epoch-change" && i > 1 {
		snapshot.RecoveryEpoch++
	}
	if s.mode == "repeated" {
		snapshot.CapturedAt = s.initial
	}
	return production.EvaluateSnapshot(snapshot)
}

// Real compiled CLI, actual direct mTLS and unchanged Core capability middleware.
// The snapshot Store is an explicit in-memory TEST source, not PostgreSQL or a
// production service heartbeat. No certificate/key or snapshot file is retained.
func TestProductionObservationCLIThroughAuthenticatedCore(t *testing.T) {
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
	window := []string{"production-status", "--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m"}
	cases := []struct {
		name, mode           string
		gate, denied, signal bool
		args                 []string
		wantCode, calls      int
		window               bool
		alert                bool
	}{
		{name: "aged-report", mode: "aged", wantCode: 0, calls: 3, window: true, alert: true},
		{name: "aged-gate", mode: "aged", gate: true, wantCode: 1, calls: 3, window: true, alert: true},
		{name: "idle-gate", mode: "idle", gate: true, wantCode: 0, calls: 3, window: true},
		{name: "epoch-change", mode: "epoch-change", wantCode: 1, calls: 2},
		{name: "repeated-snapshot", mode: "repeated", wantCode: 1, calls: 2},
		{name: "sigterm", mode: "aged", wantCode: 1, calls: 1, signal: true},
		{name: "read-permission-denied", mode: "idle", denied: true, wantCode: 1, calls: 1},
		{name: "single-authority", mode: "aged", args: []string{"production-status", "--require-authority-clear"}, wantCode: 0, calls: 1},
		{name: "single-readiness-fails", mode: "idle", args: []string{"production-status", "--require-ready"}, wantCode: 1, calls: 1},
		{name: "forbidden-repair", mode: "idle", args: append(append([]string{}, window...), "--repair"), wantCode: 1, calls: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pki := testsupport.NewPKI(t)
			readSubject, deniedSubject := "urn:engineering-platform:operator:progress-reader", "urn:engineering-platform:operator:progress-no-read"
			policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
				{Subject: readSubject, Scope: "platform", Capabilities: []string{access.Read}},
				{Subject: deniedSubject, Scope: "platform", Capabilities: []string{access.WorkCreate}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			backend := &observationStore{Memory: store.NewMemory(), mode: c.mode, initial: now, old: now.Add(-time.Hour)}
			handler, err := api.NewAuthenticatedHandler(backend, nil, policy, api.AuthenticatedOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/status" {
					t.Error("unexpected state-changing or unrelated request")
				}
				handler.ServeHTTP(w, r)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			subject := readSubject
			if c.denied {
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
			args := append([]string{}, window...)
			if c.gate {
				args = append(args, "--require-no-alert")
			}
			if c.args != nil {
				args = c.args
			}
			command := exec.CommandContext(ctx, bin, args...)
			command.Env = []string{"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + server.URL, "CONTROL_CLIENT_CERT_FILE=" + filepath.Join(dir, "client.pem"), "CONTROL_CLIENT_KEY_FILE=" + filepath.Join(dir, "client.key"), "CONTROL_SERVER_CA_FILE=" + filepath.Join(dir, "ca.pem"), "HTTPS_PROXY=http://127.0.0.1:1"}
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if c.signal {
				if err = command.Start(); err != nil {
					t.Fatal(err)
				}
				defer command.Process.Kill()
				limit := time.Now().Add(2 * time.Second)
				for requests.Load() == 0 && time.Now().Before(limit) {
					time.Sleep(5 * time.Millisecond)
				}
				if requests.Load() == 0 {
					t.Fatal("CLI did not start observation")
				}
				if err = command.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
				err = command.Wait()
			} else {
				err = command.Run()
			}
			gotCode := 0
			if err != nil {
				if e, ok := err.(*exec.ExitError); ok {
					gotCode = e.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if gotCode != c.wantCode || int(requests.Load()) != c.calls {
				t.Fatalf("exit=%d calls=%d; want %d/%d, stderr=%s", gotCode, requests.Load(), c.wantCode, c.calls, stderr.String())
			}
			if c.denied && backend.calls.Load() != 0 {
				t.Fatal("denied reader reached Store")
			}
			if c.window {
				var r production.ProgressReport
				if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
					t.Fatal(err, stdout.String())
				}
				if !r.Complete || len(r.Observations) != 3 || r.DiagnosticAlert != c.alert || r.Ready || r.ProductionQualified || r.ExecutionAuthorized || r.CapacityObserved {
					t.Fatal("CLI promoted a diagnostic", r)
				}
			} else if c.args != nil && c.calls == 1 {
				if _, err := decodeProductionStatus(stdout.Bytes(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
			} else if stdout.Len() != 0 {
				t.Fatal("incomplete/denied collection printed successful data", stdout.String())
			}
			for _, value := range [][]byte{[]byte("PRIVATE KEY"), []byte(dir), inputs["client.key"]} {
				if bytes.Contains(stdout.Bytes(), value) {
					t.Fatal("private TLS input disclosed")
				}
			}
		})
	}
}

func TestObservationCancellationPreventsFirstGET(t *testing.T) {
	// An already cancelled observation must not contact the API. Actual OS
	// signal delivery is tested against the compiled CLI in the sigterm case.
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	reader := observationReaderFunc(func(context.Context, string, string, []byte) ([]byte, error) { calls++; return nil, syscall.EINTR })
	cancel()
	_, err := collectProductionProgress(ctx, reader, production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60})
	if err == nil || calls != 0 {
		t.Fatal("cancellation allowed a network operation", calls, err)
	}
}
