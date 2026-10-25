//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/x509"
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
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func observerJournalTestDirectory(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeObserverJournalTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProductionObserverJournalLinuxPrivateFileReadback(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0400} {
		t.Run(fmt.Sprintf("%04o", mode), func(t *testing.T) {
			path := filepath.Join(observerJournalTestDirectory(t), "observer.jsonl")
			data := []byte("file-boundary fixture\n")
			writeObserverJournalTestFile(t, path, data)
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			journal, err := openProductionObserverJournal(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			got, err := io.ReadAll(journal)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatal("private journal bytes changed", string(got), err)
			}
			if err := journal.Verify(t.Context()); err != nil {
				t.Fatal("unchanged private journal rejected", err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			if err := journal.Verify(t.Context()); err == nil {
				t.Fatal("closed journal verified")
			}
		})
	}
}

func TestProductionObserverJournalLinuxRejectsUnsafeFiles(t *testing.T) {
	for _, name := range []string{"world-readable", "executable", "hardlink", "leaf-symlink", "parent-symlink", "fifo", "empty", "oversized-sparse", "directory"} {
		t.Run(name, func(t *testing.T) {
			dir := observerJournalTestDirectory(t)
			path := filepath.Join(dir, "observer.jsonl")
			writeObserverJournalTestFile(t, path, []byte("fixture\n"))
			var err error
			switch name {
			case "world-readable":
				err = os.Chmod(path, 0644)
			case "executable":
				err = os.Chmod(path, 0700)
			case "hardlink":
				err = os.Link(path, filepath.Join(dir, "second-name"))
			case "leaf-symlink":
				link := filepath.Join(dir, "linked.jsonl")
				err = os.Symlink(path, link)
				path = link
			case "parent-symlink":
				link := filepath.Join(observerJournalTestDirectory(t), "linked-parent")
				err = os.Symlink(dir, link)
				path = filepath.Join(link, "observer.jsonl")
			case "fifo":
				path = filepath.Join(dir, "unconnected-fifo")
				err = syscall.Mkfifo(path, 0600)
			case "empty":
				err = os.Truncate(path, 0)
			case "oversized-sparse":
				// Truncate sets only logical length; it does not allocate or read
				// a 128 MiB fixture just to exercise the pre-read size boundary.
				err = os.Truncate(path, production.ObserverMaxJournalBytes+1)
			case "directory":
				path = dir
			}
			if err != nil {
				t.Fatal("construct actual filesystem boundary", err)
			}
			if name != "fifo" {
				journal, err := openProductionObserverJournal(t.Context(), path)
				if journal != nil {
					journal.Close()
				}
				if err == nil {
					t.Fatal("unsafe journal accepted", name)
				}
				return
			}
			// A FIFO without a writer would block an ordinary read-only open.
			// Release such a regressed open on failure so the test cannot leak
			// its goroutine or leave filesystem cleanup waiting on a reader.
			done := make(chan error, 1)
			go func() {
				journal, err := openProductionObserverJournal(t.Context(), path)
				if journal != nil {
					journal.Close()
				}
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("FIFO accepted")
				}
			case <-time.After(2 * time.Second):
				unblock, openErr := os.OpenFile(path, os.O_RDWR|syscall.O_NONBLOCK, 0)
				if openErr == nil {
					defer unblock.Close()
				}
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				t.Fatal("FIFO inspection blocked waiting for a writer", openErr)
			}
		})
	}
}

func TestProductionObserverJournalLinuxRejectsChangedFiles(t *testing.T) {
	for _, name := range []string{"append", "truncate", "same-size-rewrite", "chmod", "replacement", "ancestor-changed"} {
		t.Run(name, func(t *testing.T) {
			base := observerJournalTestDirectory(t)
			ancestor := filepath.Join(base, "ancestor")
			parent := filepath.Join(ancestor, "parent")
			if err := os.MkdirAll(parent, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "observer.jsonl")
			data := []byte("first\n")
			writeObserverJournalTestFile(t, path, data)
			// A known older mtime makes a real same-size write observable even
			// on filesystems with coarse timestamp resolution.
			old := time.Now().Add(-time.Hour)
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
			beforeFile, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			beforeParent, err := os.Stat(parent)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := openProductionObserverJournal(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			if got, err := io.ReadAll(journal); err != nil || !bytes.Equal(got, data) {
				t.Fatal("initial read failed", err)
			}
			switch name {
			case "append":
				var f *os.File
				f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
				if err == nil {
					_, err = f.Write([]byte("next\n"))
					err = errors.Join(err, f.Close())
				}
			case "truncate":
				err = os.Truncate(path, int64(len(data)-1))
			case "same-size-rewrite":
				var f *os.File
				f, err = os.OpenFile(path, os.O_WRONLY, 0)
				if err == nil {
					_, err = f.WriteAt([]byte("other\n"), 0)
					err = errors.Join(err, f.Close())
				}
			case "chmod":
				// Both modes are permitted at open, but changing between them
				// during one read must still reject that observation.
				err = os.Chmod(path, 0400)
			case "replacement":
				replacement := filepath.Join(parent, "replacement")
				writeObserverJournalTestFile(t, replacement, data)
				err = os.Rename(replacement, path)
			case "ancestor-changed":
				retired := filepath.Join(base, "retired")
				if err = os.Rename(ancestor, retired); err == nil {
					err = os.Mkdir(ancestor, 0700)
				}
				if err == nil {
					err = os.Rename(filepath.Join(retired, "parent"), parent)
				}
				if err == nil {
					afterFile, fileErr := os.Stat(path)
					afterParent, parentErr := os.Stat(parent)
					if fileErr != nil || parentErr != nil || !os.SameFile(beforeFile, afterFile) || !os.SameFile(beforeParent, afterParent) {
						t.Fatal("ancestor fixture unexpectedly replaced the leaf or its direct parent", fileErr, parentErr)
					}
				}
			}
			if err != nil {
				t.Fatal("mutate actual opened journal", err)
			}
			if err := journal.Verify(t.Context()); err == nil {
				t.Fatal("changed journal verified", name)
			}
		})
	}
}

func TestProductionObserverJournalLinuxCancellation(t *testing.T) {
	path := filepath.Join(observerJournalTestDirectory(t), "observer.jsonl")
	writeObserverJournalTestFile(t, path, []byte("fixture\n"))
	journal, err := openProductionObserverJournal(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	for _, deadline := range []bool{false, true} {
		var ctx context.Context
		var cancel context.CancelFunc
		want := error(context.Canceled)
		if deadline {
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			ctx, cancel = context.WithCancel(t.Context())
			cancel()
		}
		opened, err := openProductionObserverJournal(ctx, path+"-absent")
		if opened != nil {
			opened.Close()
		}
		if !errors.Is(err, want) {
			t.Fatal("cancelled opener reached path lookup or lost caller error", err)
		}
		if err := journal.Verify(ctx); !errors.Is(err, want) {
			t.Fatal("cancelled verification lost caller error", err)
		}
		cancel()
	}
	if err := journal.Verify(t.Context()); err != nil {
		t.Fatal("cancelled verification damaged the retained file", err)
	}
}

func TestProductionObserverCheckRejectsExpiryDuringOutput(t *testing.T) {
	// These explicitly synthetic snapshots exercise only the local journal and
	// output boundary. They are not authenticated service or deployment facts.
	end := time.Now().UTC()
	start := end.Add(-2 * time.Second)
	policy := production.ObserverPolicy{Progress: production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60}, DurationSeconds: 30, StaleAfterSeconds: 3}
	observer, err := production.NewObserver(policy, monitorTestSession, monitorTestEndpoint, start)
	if err != nil {
		t.Fatal(err)
	}
	started, err := observer.Start(start)
	if err != nil {
		t.Fatal(err)
	}
	var samples []production.ProgressObservation
	for i := 0; i < 3; i++ {
		at := start.Add(time.Duration(i) * time.Second)
		status, err := production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: at, RecoveryEpoch: 3, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}})
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, production.ProgressObservation{RequestedAt: at, ReceivedAt: at, Status: status})
	}
	report, err := production.EvaluateProgress(policy.Progress, samples)
	if err != nil {
		t.Fatal(err)
	}
	window, err := observer.Window(report, end)
	if err != nil {
		t.Fatal(err)
	}
	var journal bytes.Buffer
	encoder := json.NewEncoder(&journal)
	for _, event := range []production.ObserverEvent{started, window} {
		if err := encoder.Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(observerJournalTestDirectory(t), "observer.jsonl")
	writeObserverJournalTestFile(t, path, journal.Bytes())
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var output bytes.Buffer
	writes := 0
	writer := jsonOutputWriterFunc(func(data []byte) (int, error) {
		writes++
		timer := time.NewTimer(time.Until(window.ExpiresAt.Add(25 * time.Millisecond)))
		defer timer.Stop()
		select {
		case <-timer.C:
			return output.Write(data)
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})
	o := productionObserverCheckOptions{journal: path, session: monitorTestSession, endpoint: monitorTestEndpoint, requireCurrentClear: true}
	if err := checkProductionObserverJournal(ctx, o, writer); err == nil || !strings.Contains(err.Error(), "not currently clear") || writes != 1 {
		t.Fatal("slow successful output retained an expired clear exit", writes, err)
	}
	var printed production.ObserverJournalCheck
	if err := json.Unmarshal(output.Bytes(), &printed); err != nil {
		t.Fatal(err)
	}
	if !printed.CurrentClear || !printed.CheckedAt.Before(window.ExpiresAt) || !time.Now().After(window.ExpiresAt) || printed.Ready || printed.ExecutionAuthorized || printed.ProductionQualified {
		t.Fatal("printed snapshot was relabelled after output or promoted authority", printed)
	}
}

// This runs the compiled CLI and the unchanged mTLS/capability middleware. The
// monitor Store is explicitly synthetic in-memory data, not PostgreSQL or a
// production deployment. No Provider or person-directed notifier is invoked.
func TestProductionObserverCheckCLIThroughAuthenticatedMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	bin := filepath.Join(t.TempDir(), "eng")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-trimpath", "-o", bin, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v %s", err, output)
	}
	pki := testsupport.NewPKI(t)
	subject := "urn:engineering-platform:operator:observer-journal-test"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: subject, Scope: "platform", Capabilities: []string{access.Read}}}})
	if err != nil {
		t.Fatal(err)
	}
	backend := &monitorCLIStore{Memory: store.NewMemory()}
	handler, err := api.NewAuthenticatedHandler(backend, nil, policy, api.AuthenticatedOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/status" {
			t.Error("unexpected mutation or endpoint", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	endpoint := canonical.BytesDigest([]byte(server.URL))
	cert := pki.ClientCertificate(t, subject)
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	credentials := observerJournalTestDirectory(t)
	for name, data := range map[string][]byte{
		"ca.pem":     pki.CAPEM,
		"client.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}),
		"client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}),
	} {
		writeObserverJournalTestFile(t, filepath.Join(credentials, name), data)
	}
	dir := observerJournalTestDirectory(t)
	path := filepath.Join(dir, "observer.jsonl")
	journal, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	producer := exec.CommandContext(ctx, bin, "production-status", "--monitor-for", "30s", "--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m", "--stale-after", "5s", "--session", monitorTestSession, "--require-no-alert")
	producer.Env = []string{"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + server.URL, "CONTROL_CLIENT_CERT_FILE=" + filepath.Join(credentials, "client.pem"), "CONTROL_CLIENT_KEY_FILE=" + filepath.Join(credentials, "client.key"), "CONTROL_SERVER_CA_FILE=" + filepath.Join(credentials, "ca.pem")}
	var producerStderr bytes.Buffer
	producer.Stdout, producer.Stderr = journal, &producerStderr
	if err := producer.Start(); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		producer.Process.Kill()
		producer.Wait()
		t.Fatal(err)
	}
	done := make(chan struct{})
	var producerErr error
	go func() {
		producerErr = producer.Wait()
		close(done)
	}()
	defer func() { producer.Process.Kill(); <-done }()
	var latest production.ObserverEvent
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	startedWindow := time.NewTimer(12 * time.Second)
	defer startedWindow.Stop()
waitForWindow:
	for {
		select {
		case <-done:
			t.Fatal("producer ended before a clear window", producerErr, producerStderr.String())
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-startedWindow.C:
			t.Fatal("producer did not emit a complete clear window")
		case <-poll.C:
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// This loop waits for production to finish a line. The checker
			// itself must reject an incomplete trailing record, tested below.
			lines := bytes.Split(raw, []byte{'\n'})
			for _, line := range lines[:len(lines)-1] {
				var event production.ObserverEvent
				if err := json.Unmarshal(line, &event); err != nil {
					t.Fatal("producer emitted invalid JSON", err)
				}
				if event.Kind == "WINDOW" && event.State == production.ObserverClear {
					latest = event
					break waitForWindow
				}
			}
		}
	}
	// SIGKILL deliberately leaves a complete live prefix without a STOPPED
	// record, the failure an independently scheduled consumer must catch later.
	if err := producer.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	var killed *exec.ExitError
	if !errors.As(producerErr, &killed) {
		t.Fatal("producer did not terminate from the test's kill", producerErr)
	}
	status, ok := killed.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatal("producer did not terminate by SIGKILL", producerErr)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(raw, []byte{'\n'})
	if len(lines) != 3 || len(lines[2]) != 0 {
		t.Fatal("expected exactly STARTED and one complete WINDOW", string(raw))
	}
	for i, line := range lines[:2] {
		var event production.ObserverEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if event.Sequence != i+1 || event.SessionID != monitorTestSession || event.EndpointDigest != endpoint || event.Stopped || event.Ready || event.CapacityObserved || event.ExecutionAuthorized || event.ProductionQualified {
			t.Fatal("producer changed pins, appended STOP, or promoted authority", event)
		}
	}
	if requests.Load() != 3 || backend.calls.Load() != 3 {
		t.Fatal("monitor did not make exactly one authenticated read-only window", requests.Load(), backend.calls.Load())
	}
	if err := os.RemoveAll(credentials); err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, journalPath, session, expectedEndpoint string, wantCode int, wantReport bool) production.ObserverJournalCheck {
		t.Helper()
		command := exec.CommandContext(ctx, bin, "production-observer-check", "--journal", journalPath, "--session", session, "--endpoint-digest", expectedEndpoint, "--require-current-clear")
		// No Core endpoint, certificate, key or CA environment; the former
		// producer credential files have also been removed above.
		command.Env = []string{"PATH=/usr/bin:/bin"}
		var output, diagnostics bytes.Buffer
		command.Stdout, command.Stderr = &output, &diagnostics
		err := command.Run()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatal(err)
			}
			code = exit.ExitCode()
		}
		if code != wantCode || requests.Load() != 3 || backend.calls.Load() != 3 {
			t.Fatalf("independent checker exit=%d want=%d Core requests=%d store=%d stderr=%q", code, wantCode, requests.Load(), backend.calls.Load(), diagnostics.String())
		}
		var check production.ObserverJournalCheck
		if !wantReport {
			if output.Len() != 0 {
				t.Fatal("invalid identity or journal emitted a report", output.String())
			}
			return check
		}
		if err := json.Unmarshal(output.Bytes(), &check); err != nil {
			t.Fatal("checker did not return valid JSON", err, diagnostics.String())
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"ready", "execution_authorized", "production_qualified"} {
			if !bytes.Equal(fields[name], []byte("false")) {
				t.Fatal("checker omitted or promoted authority", name, string(fields[name]))
			}
		}
		if check.SessionID != monitorTestSession || check.EndpointDigest != endpoint || check.JournalDigest != canonical.BytesDigest(raw) || check.JournalBytes != int64(len(raw)) || check.RecordCount != 2 || check.LastSequence != 2 || check.Stopped || check.HadAlert {
			t.Fatal("checker changed retained byte or event identity", check)
		}
		return check
	}
	t.Run("fresh-independent-clear", func(t *testing.T) {
		check := run(t, path, monitorTestSession, endpoint, 0, true)
		if !check.CurrentClear || !check.ObserverFresh || !check.SourceFresh {
			t.Fatal("fresh complete clear window was not observed", check)
		}
	})
	t.Run("wrong-session", func(t *testing.T) {
		run(t, path, strings.Repeat("f", 32), endpoint, 1, false)
	})
	t.Run("wrong-endpoint", func(t *testing.T) {
		run(t, path, monitorTestSession, "sha256:"+strings.Repeat("f", 64), 1, false)
	})
	for _, trim := range []int{1, 2} {
		t.Run(fmt.Sprintf("truncated-tail-%d", trim), func(t *testing.T) {
			truncated := filepath.Join(dir, fmt.Sprintf("truncated-%d.jsonl", trim))
			writeObserverJournalTestFile(t, truncated, raw[:len(raw)-trim])
			run(t, truncated, monitorTestSession, endpoint, 1, false)
		})
	}
	timer := time.NewTimer(time.Until(latest.ExpiresAt.Add(25 * time.Millisecond)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	t.Run("killed-producer-expires", func(t *testing.T) {
		if !time.Now().After(latest.ExpiresAt) {
			t.Fatal("expiry check ran before the actual retained deadline")
		}
		check := run(t, path, monitorTestSession, endpoint, 1, true)
		if check.CurrentClear || check.ObserverFresh || check.SourceFresh || !check.CheckedAt.After(latest.ExpiresAt) {
			t.Fatal("killed observer remained fresh or granted a successful gate", check)
		}
	})
}
