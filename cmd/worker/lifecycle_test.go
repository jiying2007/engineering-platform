package main

import (
	"bytes"
	"context"
	"crypto/x509"
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
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

type workerSemanticStopError struct {
	cause   error
	meaning error
}

func (e *workerSemanticStopError) Error() string        { return "unobserved operation" }
func (e *workerSemanticStopError) Unwrap() error        { return e.cause }
func (e *workerSemanticStopError) Is(target error) bool { return target == e.meaning }

func TestWorkerLoopPreservesMixedAndUnrequestedStopErrors(t *testing.T) {
	fault := errors.New("preparation cleanup failed")
	for _, tc := range []struct {
		name      string
		cancel    bool
		failure   error
		wantNil   bool
		wantCause error
	}{
		{"requested-cancel", true, context.Canceled, true, nil},
		{"requested-wrapped-cancel", true, fmt.Errorf("step stopped: %w", context.Canceled), true, nil},
		{"requested-joined-cancel", true, errors.Join(context.Canceled, fmt.Errorf("lease stopped: %w", context.Canceled)), true, nil},
		{"requested-cancel-and-cleanup-failure", true, errors.Join(context.Canceled, fault), false, fault},
		{"unrequested-cancel", false, context.Canceled, false, context.Canceled},
		{"unrequested-wrapped-cancel", false, fmt.Errorf("child stopped independently: %w", context.Canceled), false, context.Canceled},
		{"deadline-remains-failure", true, context.DeadlineExceeded, false, context.DeadlineExceeded},
		{"cancel-after-complete-step", true, nil, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			err := runWorkerLoop(ctx, 10*time.Millisecond, func(context.Context) error {
				calls++
				if tc.cancel {
					cancel()
				}
				return tc.failure
			})
			if calls != 1 || (err == nil) != tc.wantNil {
				t.Fatalf("unexpected stop classification: calls=%d err=%v", calls, err)
			}
			if tc.wantCause != nil && (!errors.Is(err, tc.wantCause) || err != tc.failure) {
				t.Fatal("original failure tree was replaced or lost", err)
			}
			if !tc.cancel && ctx.Err() != nil {
				t.Fatal("worker invented parent cancellation")
			}
		})
	}
}

func TestWorkerLoopPreservesAuthorityAmbiguityAfterCancel(t *testing.T) {
	for _, meaning := range []error{
		controlclient.ErrPostOutcomeUnobserved,
		controlclient.ErrCommitOutcomeUnknown,
		controlclient.ErrExternalEffectUnresolved,
		workeragent.ErrFailureReportUnobserved,
	} {
		t.Run(meaning.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			var original error
			err := runWorkerLoop(ctx, 10*time.Millisecond, func(stepCtx context.Context) error {
				calls++
				cancel()
				if meaning == controlclient.ErrPostOutcomeUnobserved {
					// Exercise the real client's pre-cancelled POST classification.
					// No endpoint or network operation is available in this fixture.
					_, original = (&controlclient.Client{}).Raw(stepCtx, http.MethodPost, "/api/v1/worker/claim", []byte(`{}`))
				} else {
					// Model the other existing dual-semantics wrappers without
					// needing a state-changing publication or execution fixture.
					original = fmt.Errorf("retained failure: %w", &workerSemanticStopError{cause: stepCtx.Err(), meaning: meaning})
				}
				return original
			})
			if calls != 1 || err == nil || err != original || !errors.Is(err, meaning) || !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation erased exact-readback authority or replayed the step", calls, err)
			}
		})
	}
}

func TestWorkerLoopRetainsOnlyCompleteConflictDeferral(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := runWorkerLoop(ctx, 10*time.Millisecond, func(context.Context) error {
		calls++
		if calls == 1 {
			return &controlclient.HTTPError{Status: http.StatusConflict}
		}
		cancel()
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatal("complete HTTP conflict no longer defers one polling cycle", calls, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	calls = 0
	original := errors.Join(&controlclient.HTTPError{Status: http.StatusConflict}, context.Canceled)
	err = runWorkerLoop(ctx, 10*time.Millisecond, func(context.Context) error {
		calls++
		cancel()
		return original
	})
	if err != original || calls != 1 {
		t.Fatal("damaged conflict was swallowed or caused another poll", calls, err)
	}
}

func TestWorkerLoopPreCancelledOwnerDoesNotCallStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runWorkerLoop(ctx, 10*time.Millisecond, func(context.Context) error {
		t.Fatal("pre-cancelled owner admitted a step")
		return nil
	})
	if err != nil {
		t.Fatal("idle requested stop became a failure", err)
	}
}

// This uses the actual compiled Worker and its mTLS Control client. The test
// handler observes one claim POST but returns no acknowledgement before the
// process receives SIGTERM. It proves exit/error propagation, not SQL commit,
// production-host stop latency or safe replay of an unobserved operation.
func TestWorkerCLIStoppedAfterUnobservedClaimRequiresReadback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	bin := filepath.Join(t.TempDir(), "worker")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual Worker: %v %s", err, out)
	}
	pki := testsupport.NewPKI(t)
	var requests atomic.Int32
	received, release := make(chan struct{}), make(chan struct{})
	var observed sync.Once
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/worker/claim" {
			t.Error("unexpected request or mutation endpoint")
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 4096))
		observed.Do(func() { close(received) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	defer close(release)
	cert := pki.ClientCertificate(t, "urn:engineering-platform:worker:cancel-readback-test")
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{
		"ca.pem":     pki.CAPEM,
		"client.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}),
		"client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.CommandContext(ctx, bin, "--admission-only", "--profile", "input-admission-v1")
	command.Env = []string{
		"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + server.URL,
		"CONTROL_CLIENT_CERT_FILE=" + filepath.Join(dir, "client.pem"),
		"CONTROL_CLIENT_KEY_FILE=" + filepath.Join(dir, "client.key"),
		"CONTROL_SERVER_CA_FILE=" + filepath.Join(dir, "ca.pem"),
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = command.Process.Kill()
			<-done
		}
	}()
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("claim did not reach the authenticated handler")
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		finished = true
	case <-time.After(5 * time.Second):
		t.Fatal("Worker did not join its cancelled claim and exit")
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || requests.Load() != 1 || stdout.Len() != 0 {
		t.Fatalf("unobserved claim became success or was replayed: exit=%v calls=%d stdout=%q stderr=%q", err, requests.Load(), stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "control POST outcome unobserved") || !strings.Contains(stderr.String(), "read back exact original resource") || !strings.Contains(stderr.String(), "before any retry") {
		t.Fatalf("Worker discarded its exact-readback diagnostic: %s", stderr.String())
	}
	for _, secret := range []string{server.URL, dir, "PRIVATE KEY"} {
		if strings.Contains(stderr.String(), secret) {
			t.Fatal("Worker leaked private transport or credential details")
		}
	}
}
