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
	"strconv"
	"strings"
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

func monitorPipeFlags(t *testing.T, file *os.File) uintptr {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags uintptr
	var errno syscall.Errno
	if err := raw.Control(func(fd uintptr) {
		flags, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
	}); err != nil || errno != 0 {
		t.Fatal(err, errno)
	}
	return flags
}

// A blocking inherited write end models stdout without relying on Go's
// pollable os.Pipe wrapper. Changes to this OFD would affect the parent too.
func monitorBlockingPipe(t *testing.T, fill int) (*os.File, *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close(); writer.Close() })
	raw, err := writer.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var setupErr error
	if err := raw.Control(func(fd uintptr) {
		if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETPIPE_SZ, 4096); errno != 0 {
			setupErr = errno
			return
		}
		setupErr = syscall.SetNonblock(int(fd), false)
	}); err != nil || setupErr != nil {
		t.Fatal(err, setupErr)
	}
	if fill != 0 {
		if n, err := writer.Write(bytes.Repeat([]byte("x"), fill)); err != nil || n != fill {
			t.Fatal(n, err)
		}
	}
	return reader, writer
}

func TestProductionMonitorPipeOutputEndsBlockedInitialWrite(t *testing.T) {
	for _, mode := range []string{"deadline", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			_, writer := monitorBlockingPipe(t, 4096)
			before := monitorPipeFlags(t, writer)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer deadlineCancel()
			} else {
				timer := time.AfterFunc(100*time.Millisecond, cancel)
				defer timer.Stop()
			}
			var calls int
			reader := observationReaderFunc(func(context.Context, string, string, []byte) ([]byte, error) {
				calls++
				return nil, fmt.Errorf("unexpected source read")
			})
			p := production.ObserverPolicy{Progress: production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60}, DurationSeconds: 3, StaleAfterSeconds: 4}
			start := time.Now()
			hadAlert, err := runProductionMonitor(ctx, reader, p, "https://synthetic.invalid", "", writer)
			if err == nil || hadAlert || calls != 0 || time.Since(start) > 2*time.Second {
				t.Fatalf("blocked output did not end promptly: calls=%d alert=%v err=%v elapsed=%s", calls, hadAlert, err, time.Since(start))
			}
			if after := monitorPipeFlags(t, writer); after != before || after&syscall.O_NONBLOCK != 0 {
				t.Fatal("caller/shared stdout flags changed")
			}
		})
	}
}

func TestProductionMonitorPipeOutputCancellationMakesOneNonblockingAttempt(t *testing.T) {
	reader, writer := monitorBlockingPipe(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, cleanup, err := prepareProductionMonitorOutput(ctx, writer)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cancel()
	if n, err := output.Write([]byte("stopped\n")); err != nil || n != 8 {
		t.Fatal("drained sink lost its terminal write", n, err)
	}
	if n, err := reader.Read(make([]byte, 8)); err != nil || n != 8 {
		t.Fatal(n, err)
	}
	if n, err := writer.Write(bytes.Repeat([]byte("x"), 4096)); err != nil || n != 4096 {
		t.Fatal(n, err)
	}
	start := time.Now()
	if n, err := output.Write([]byte("stopped\n")); err == nil || n != 0 || time.Since(start) > time.Second {
		t.Fatal("cancelled output waited/retried or invented terminal bytes", n, err)
	}
	cleanup()
	cleanup() // Cancellation callbacks and descriptor cleanup are joined once.
	if _, err := writer.Stat(); err != nil {
		t.Fatal("caller descriptor was closed", err)
	}
}

func TestProductionMonitorPipeOutputRejectsNoReaderAndPreservesOtherWriters(t *testing.T) {
	reader, writer := monitorBlockingPipe(t, 0)
	before := monitorPipeFlags(t, writer)
	if _, cleanup, err := prepareProductionMonitorOutput(context.Background(), reader); err == nil {
		cleanup()
		t.Fatal("read-only descriptor was reopened for writing")
	}
	reader.Close()
	if output, cleanup, err := prepareProductionMonitorOutput(context.Background(), writer); err == nil {
		// Anonymous pipefs can reopen a writer without readers; named FIFOs
		// can instead return ENXIO. Neither case may yield successful output.
		if n, err := output.Write([]byte("unread\n")); err == nil || n != 0 {
			t.Fatal("pipe with no reader accepted output", n, err)
		}
		cleanup()
	}
	if after := monitorPipeFlags(t, writer); after != before {
		t.Fatal("failed reopen changed original flags")
	}
	var buffer bytes.Buffer
	file, err := os.Create(filepath.Join(t.TempDir(), "regular-output"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, original := range []io.Writer{&buffer, file} {
		output, cleanup, err := prepareProductionMonitorOutput(context.Background(), original)
		if err != nil || output != original {
			t.Fatal("non-pipe output semantics changed", err)
		}
		cleanup()
		if _, err := original.Write([]byte("still open")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionMonitorPipeOutputRejectsPartialTerminalRecord(t *testing.T) {
	reader, writer := monitorBlockingPipe(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, cleanup, err := prepareProductionMonitorOutput(ctx, writer)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cancel()
	// A terminal record larger than available pipe space may be partially
	// written by the single syscall. It must never be retried or acknowledged.
	emit := observerJSONEmitter(output)
	if err := emit(production.ObserverEvent{Kind: "STOPPED", Reason: strings.Repeat("x", 8192)}); err == nil {
		t.Fatal("partial cancelled terminal record accepted")
	}
	data := make([]byte, 8192)
	n, err := reader.Read(data)
	if err != nil || n != 4096 || bytes.Contains(data[:n], []byte("\n")) {
		t.Fatal("unexpected partial output or fabricated complete line", n, err)
	}
}

func waitForMonitorOwnedPipe(t *testing.T, pid int, pipe os.FileInfo) {
	t.Helper()
	base := "/proc/" + strconv.Itoa(pid)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(base + "/fd")
		for _, entry := range entries {
			if entry.Name() == "1" {
				continue
			}
			info, err := os.Stat(base + "/fd/" + entry.Name())
			if err != nil || !os.SameFile(pipe, info) {
				continue
			}
			data, _ := os.ReadFile(base + "/fdinfo/" + entry.Name())
			for _, line := range strings.Split(string(data), "\n") {
				if value, ok := strings.CutPrefix(line, "flags:\t"); ok {
					flags, err := strconv.ParseUint(value, 8, 64)
					if err == nil && flags&syscall.O_NONBLOCK != 0 {
						return
					}
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("CLI did not own an independent nonblocking pipe output")
}

// Real subprocesses exercise inherited stdout flags, actual pipe backpressure,
// signal handling and the normal authenticated Control client. Synthetic Store
// observations remain test inputs, never production qualification evidence.
func TestProductionMonitorCLIBlockedPipeDeadlineAndSIGTERM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	bin := filepath.Join(t.TempDir(), "eng")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v %s", err, out)
	}
	for _, mode := range []string{"start-deadline", "start-sigterm", "window-deadline", "window-sigterm"} {
		t.Run(mode, func(t *testing.T) {
			pki := testsupport.NewPKI(t)
			subject := "urn:engineering-platform:operator:blocked-output"
			policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: subject, Scope: "platform", Capabilities: []string{access.Read}}}})
			if err != nil {
				t.Fatal(err)
			}
			backend := &monitorCLIStore{Memory: store.NewMemory()}
			handler, err := api.NewAuthenticatedHandler(backend, nil, policy, api.AuthenticatedOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/status" {
					t.Error("unexpected source mutation or endpoint")
				}
				handler.ServeHTTP(w, r)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			cert := pki.ClientCertificate(t, subject)
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for name, raw := range map[string][]byte{"ca.pem": pki.CAPEM, "client.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), "client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})} {
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			signal := strings.HasSuffix(mode, "sigterm")
			window := strings.HasPrefix(mode, "window")
			duration, seconds := "3s", 3
			if signal {
				duration, seconds = "6s", 6
			}
			fill := 4096
			if window {
				p := production.ObserverPolicy{Progress: production.ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60}, DurationSeconds: seconds, StaleAfterSeconds: 4}
				at := time.Date(2026, 10, 22, 0, 0, 0, 999999999, time.UTC)
				o, err := production.NewObserver(p, monitorTestSession, monitorTestEndpoint, at)
				if err != nil {
					t.Fatal(err)
				}
				started, err := o.Start(at)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(started)
				fill -= len(raw) + 1 + 64 // STARTED fits; a completed window cannot.
			}
			pipeReader, pipeWriter := monitorBlockingPipe(t, fill)
			beforeFlags := monitorPipeFlags(t, pipeWriter)
			pipeInfo, err := pipeWriter.Stat()
			if err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(ctx, bin, "production-status", "--monitor-for", duration, "--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m", "--stale-after", "4s", "--require-no-alert")
			command.Env = []string{"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + server.URL, "CONTROL_CLIENT_CERT_FILE=" + filepath.Join(dir, "client.pem"), "CONTROL_CLIENT_KEY_FILE=" + filepath.Join(dir, "client.key"), "CONTROL_SERVER_CA_FILE=" + filepath.Join(dir, "ca.pem")}
			command.Stdout = pipeWriter
			var stderr bytes.Buffer
			command.Stderr = &stderr
			start := time.Now()
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			exited := false
			t.Cleanup(func() {
				if !exited {
					command.Process.Kill()
					<-done
				}
			})
			waitForMonitorOwnedPipe(t, command.Process.Pid, pipeInfo)
			if signal {
				if window {
					limit := time.Now().Add(4 * time.Second)
					for calls.Load() < 3 && time.Now().Before(limit) {
						time.Sleep(5 * time.Millisecond)
					}
					if calls.Load() != 3 {
						t.Fatal("first window did not complete before SIGTERM", calls.Load())
					}
				}
				if err := command.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
			}
			waitBudget := time.Duration(seconds)*time.Second + 2*time.Second
			if signal {
				waitBudget = 2 * time.Second
			}
			select {
			case err = <-done:
				exited = true
			case <-time.After(waitBudget):
				t.Fatal("blocked stdout survived monitor deadline/SIGTERM")
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("output failure did not produce normal nonzero exit: %v %s", err, stderr.String())
			}
			wantCalls := int32(0)
			if window {
				wantCalls = 3
			}
			if calls.Load() != wantCalls || monitorPipeFlags(t, pipeWriter) != beforeFlags {
				t.Fatal("output failure sent additional GETs or changed shared stdout flags", calls.Load(), wantCalls)
			}
			pipeWriter.Close()
			retained, readErr := io.ReadAll(pipeReader)
			if readErr != nil {
				t.Fatal(readErr)
			}
			retained = retained[fill:]
			if bytes.Contains(retained, []byte(`"kind":"STOPPED"`)) {
				t.Fatal("blocked output fabricated a complete terminal record")
			}
			if window && !bytes.Contains(retained, []byte(`"kind":"STARTED"`)) {
				t.Fatal("initial event did not fit before window output blocked")
			}
			t.Logf("%s: exit=1 source GETs=%d elapsed=%s", mode, calls.Load(), time.Since(start))
		})
	}
}
