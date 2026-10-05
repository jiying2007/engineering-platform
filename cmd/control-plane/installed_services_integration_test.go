//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/distribution"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/production"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// A test-only bounded sink extracts the literal loopback listener announcement.
// Receipt/health checks, not the log line, establish application availability.
// Neither configuration contents nor fixture keys are retained in test output.
type serviceOutput struct {
	mu      sync.Mutex
	pending string
	ready   chan string
}

func (w *serviceOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending)+len(p) > 64<<10 {
		return 0, fmt.Errorf("service output exceeded test bound")
	}
	w.pending += string(p)
	for {
		line, rest, found := strings.Cut(w.pending, "\n")
		if !found {
			break
		}
		w.pending = rest
		if _, endpoint, ok := strings.Cut(line, " listening on "); ok {
			endpoint, _, _ = strings.Cut(endpoint, ";")
			host, port, err := net.SplitHostPort(endpoint)
			if err == nil && host == "127.0.0.1" && port != "" && port != "0" {
				select {
				case w.ready <- "https://" + endpoint:
				default:
				}
			}
		}
	}
	return len(p), nil
}

type installedProcess struct {
	cmd   *exec.Cmd
	done  chan struct{}
	ready chan string
	err   error // read only after done closes
}

func startInstalledProcess(t *testing.T, ctx context.Context, binary string, args, env []string) *installedProcess {
	t.Helper()
	p := &installedProcess{done: make(chan struct{}), ready: make(chan string, 1)}
	p.cmd = exec.CommandContext(ctx, binary, args...)
	p.cmd.Env = append([]string{"PATH=/usr/bin:/bin"}, env...)
	p.cmd.Stdout = io.Discard
	p.cmd.Stderr = &serviceOutput{ready: p.ready}
	commandOK(t, p.cmd.Start())
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		select {
		case <-p.done:
			return
		default:
			_ = p.cmd.Process.Kill()
		}
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			t.Error("test service cleanup did not reap its process")
		}
	})
	return p
}

func (p *installedProcess) endpoint(t *testing.T, ctx context.Context) string {
	t.Helper()
	select {
	case endpoint := <-p.ready:
		return endpoint
	case <-p.done:
		t.Fatalf("installed service exited before binding: %v", p.err)
	case <-ctx.Done():
		t.Fatal("installed service startup context expired")
	case <-time.After(20 * time.Second):
		t.Fatal("installed service did not announce its bound listener")
	}
	return ""
}

func (p *installedProcess) stop(t *testing.T, signal syscall.Signal) {
	t.Helper()
	commandOK(t, p.cmd.Process.Signal(signal))
	select {
	case <-p.done:
		if signal == syscall.SIGKILL {
			var e *exec.ExitError
			if !errors.As(p.err, &e) || e.Sys().(syscall.WaitStatus).Signal() != signal {
				t.Fatalf("expected observed forced stop, got %v", p.err)
			}
		} else {
			commandOK(t, p.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("installed service failed to stop and join")
	}
}

func replaceServiceEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			out = append(out, entry)
		}
	}
	return append(out, key+"="+value)
}

func serviceWrite(t *testing.T, dir, name string, raw []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	commandOK(t, os.WriteFile(path, raw, 0600))
	return path
}

func serviceJSON(t *testing.T, dir, name string, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	commandOK(t, err)
	return serviceWrite(t, dir, name, raw)
}

func serviceClientFiles(t *testing.T, dir, prefix string, cert tls.Certificate) (string, string) {
	t.Helper()
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	commandOK(t, err)
	return serviceWrite(t, dir, prefix+".crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})),
		serviceWrite(t, dir, prefix+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
}

func serviceClient(t *testing.T, pki *testsupport.PKI, endpoint, subject string) *controlclient.Client {
	t.Helper()
	c, err := controlclient.New(endpoint, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, subject)}})
	commandOK(t, err)
	t.Cleanup(c.Close)
	return c
}

func assertServiceClosed(t *testing.T, endpoint string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", strings.TrimPrefix(endpoint, "https://"), 250*time.Millisecond)
	if err == nil {
		c.Close()
		t.Fatal("stopped service still accepts connections")
	}
}

func rejectInstalledProcess(t *testing.T, ctx context.Context, binary string, args, env []string) {
	t.Helper()
	p := startInstalledProcess(t, ctx, binary, args, env)
	select {
	case <-p.ready:
		t.Fatal("rejected service announced a listener")
	case <-p.done:
		if p.err == nil {
			t.Fatal("invalid startup returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("invalid startup did not reject promptly")
	}
}

// EP_TEST_INSTALLED_* is an explicit test-harness-only option used to re-run
// these tests against authenticated downloaded bytes. The normal CI builds and
// installs all six real roles, then deletes the original distribution first.
func serviceTestBinaries(t *testing.T, ctx context.Context) string {
	t.Helper()
	root, source := os.Getenv("EP_TEST_INSTALLED_ROOT"), os.Getenv("EP_TEST_INSTALLED_SOURCE")
	if root == "" && source == "" {
		return installedOfflineDistribution(t, ctx)
	}
	if root == "" || source == "" {
		t.Fatal("both delivered installation and exact source are required")
	}
	_, err := distribution.VerifyInstallation(ctx, root, source)
	commandOK(t, err)
	return filepath.Join(root, "bin")
}

// This starts the INSTALLED entrypoints, not assembleServer or an in-process
// API. Identities, policies, Tasks and the inert publisher token are synthetic.
// No valid publish/observe, model call, production DB, systemd or migration is
// performed by the running services. The test admin migrates only a new schema.
func TestOfflineCommandInstalledServicesLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	bin := serviceTestBinaries(t, ctx)
	config := t.TempDir()
	commandOK(t, os.Chmod(config, 0700))
	pki := testsupport.NewPKI(t)
	ca := serviceWrite(t, config, "ca.crt", pki.CAPEM)
	cert := serviceWrite(t, config, "server.crt", pki.ServerCertPEM)
	key := serviceWrite(t, config, "server.key", pki.ServerKeyPEM)
	const controller = "urn:engineering-platform:control:installed-test"
	const engineer = "urn:engineering-platform:engineer:installed-test"
	const worker = "urn:engineering-platform:worker:installed-test"
	const stranger = "urn:engineering-platform:stranger:installed-test"
	controlCert, controlKey := serviceClientFiles(t, config, "control-client", pki.ClientCertificate(t, controller))
	artifactRoot := filepath.Join(config, "artifacts")
	commandOK(t, os.Mkdir(artifactRoot, 0700))
	git, err := exec.LookPath("git")
	commandOK(t, err)
	git, err = filepath.EvalSymlinks(git)
	commandOK(t, err)
	targets := []githubpublish.TargetPolicy{{Repository: "example/lifecycle-fixture", BaseRef: "main", BranchPrefix: "test/"}}
	publisherConfig := serviceJSON(t, config, "publisher.json", githubpublish.Configuration{Version: 1, ArtifactRoot: artifactRoot, GitExecutable: git, TokenFile: serviceWrite(t, config, "inert-token", []byte("TEST_ONLY_NOT_A_REAL_CREDENTIAL")), Targets: targets})
	publisherEnv := []string{"PUBLISHER_CONFIG_FILE=" + publisherConfig, "PUBLISHER_TLS_CERT_FILE=" + cert, "PUBLISHER_TLS_KEY_FILE=" + key, "PUBLISHER_CLIENT_CA_FILE=" + ca, "PUBLISHER_CONTROL_SUBJECT=" + controller, "LISTEN_HOST=127.0.0.1", "PORT=0"}
	publisher := startInstalledProcess(t, ctx, filepath.Join(bin, "publisher-service"), nil, publisherEnv)
	publisherEndpoint := publisher.endpoint(t, ctx)
	_, publisherPort, err := net.SplitHostPort(strings.TrimPrefix(publisherEndpoint, "https://"))
	commandOK(t, err)
	publisherEnv = replaceServiceEnv(publisherEnv, "PORT", publisherPort)
	pubClient := serviceClient(t, pki, publisherEndpoint, controller)
	checkPublisher := func() {
		t.Helper()
		var health map[string]string
		commandOK(t, pubClient.Call(ctx, http.MethodGet, "/healthz", nil, &health))
		if health["service"] != "engineering-github-publisher" || health["status"] != "ok" {
			t.Fatal("installed Publisher health identity mismatch")
		}
	}
	{ // Installed Publisher is also verified without a local PostgreSQL service.
		checkPublisher()
		bad := serviceClient(t, pki, publisherEndpoint, stranger)
		var health map[string]string
		var denied *controlclient.HTTPError
		if err := bad.Call(ctx, http.MethodGet, "/healthz", nil, &health); !errors.As(err, &denied) || denied.Status != http.StatusUnauthorized {
			t.Fatalf("Publisher admitted wrong client URI: %v", err)
		}
		for _, path := range []string{"/v1/publish", "/v1/observe"} {
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, publisherEndpoint+path, strings.NewReader(`{"plan":{}}`))
			commandOK(t, err)
			request.Header.Set("Content-Type", "application/json")
			response, err := pki.Client(t, controller).Do(request)
			commandOK(t, err)
			response.Body.Close()
			if response.StatusCode != http.StatusForbidden {
				t.Fatal("invalid plan was not rejected before external operations", response.StatusCode)
			}
		}
		rejectInstalledProcess(t, ctx, filepath.Join(bin, "publisher-service"), []string{"--execute"}, publisherEnv)
		rejectInstalledProcess(t, ctx, filepath.Join(bin, "publisher-service"), nil, publisherEnv) // occupied listener
		checkPublisher()
		publisher.stop(t, syscall.SIGTERM)
		assertServiceClosed(t, publisherEndpoint)
		publisher = startInstalledProcess(t, ctx, filepath.Join(bin, "publisher-service"), nil, publisherEnv)
		if publisher.endpoint(t, ctx) != publisherEndpoint {
			t.Fatal("Publisher restart changed requested listener")
		}
		checkPublisher()
		// Idle crash/restart, not a claim about an in-flight external publication.
		publisher.stop(t, syscall.SIGKILL)
		assertServiceClosed(t, publisherEndpoint)
		publisher = startInstalledProcess(t, ctx, filepath.Join(bin, "publisher-service"), nil, publisherEnv)
		if publisher.endpoint(t, ctx) != publisherEndpoint {
			t.Fatal("Publisher crash restart changed listener")
		}
		checkPublisher()
		t.Log("installed Publisher: exact mTLS identity, invalid plans/arguments, port collision, SIGTERM and idle SIGKILL restart PASS; no external publish/observe")
	}

	{
		dsn := os.Getenv("POSTGRES_TEST_URL")
		if dsn == "" {
			if os.Getenv("EP_SANDBOX_INTEGRATION") == "1" {
				t.Fatal("mandatory installed service integration requires PostgreSQL")
			}
			t.Run("control-postgres-durable-restart", func(t *testing.T) {
				t.Skip("POSTGRES_TEST_URL unavailable; Publisher proof is not Core/DB proof")
			})
			publisher.stop(t, syscall.SIGTERM)
			assertServiceClosed(t, publisherEndpoint)
			return
		}
		admin, err := pgxpool.New(ctx, dsn)
		commandOK(t, err)
		defer admin.Close()
		var nonce [16]byte
		_, err = rand.Read(nonce[:])
		commandOK(t, err)
		schema := "ep_installed_" + hex.EncodeToString(nonce[:])
		quoted := pgx.Identifier{schema}.Sanitize()
		_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
		commandOK(t, err)
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE")
			if err != nil {
				t.Errorf("remove only this test schema: %v", err)
			}
		}()
		u, err := url.Parse(dsn)
		commandOK(t, err)
		if u.Scheme != "postgres" && u.Scheme != "postgresql" {
			t.Fatal("installed service test requires an explicit PostgreSQL URL")
		}
		query := u.Query()
		query.Set("search_path", schema)
		u.RawQuery = query.Encode()
		pool, err := pgxpool.New(ctx, u.String())
		commandOK(t, err)
		store := pgstore.New(pool)
		defer store.Close()
		policy := serviceJSON(t, config, "access.json", access.Document{Version: 1, Principals: []access.PrincipalSpec{
			{Subject: engineer, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart}},
			{Subject: worker, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{"worker/installed-test"}},
		}})
		plan := serviceJSON(t, config, "plan.json", githubpublish.PlanConfiguration{Version: 1, ArtifactRoot: artifactRoot, Targets: targets})
		remote := serviceJSON(t, config, "remote.json", githubpublish.RemoteConfiguration{Version: 1, Endpoint: publisherEndpoint, ClientCertFile: controlCert, ClientKeyFile: controlKey, ServerCAFile: ca})
		controlEnv := []string{"DATABASE_URL=" + u.String(), "AUTO_MIGRATE=0", "CONTROL_TLS_CERT_FILE=" + cert, "CONTROL_TLS_KEY_FILE=" + key, "CONTROL_CLIENT_CA_FILE=" + ca, "CONTROL_AUTH_POLICY_FILE=" + policy, "GITHUB_PUBLISHER_PLAN_FILE=" + plan, "GITHUB_PUBLISHER_REMOTE_FILE=" + remote, "LISTEN_HOST=127.0.0.1", "PORT=0"}
		controlBin := filepath.Join(bin, "control-plane")
		args := []string{"--production"}
		// No fallback to memory and no implicit service migration into a blank DB.
		rejectInstalledProcess(t, ctx, controlBin, args, controlEnv)
		var count int
		commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname=$1", schema).Scan(&count))
		if count != 0 {
			t.Fatal("production startup mutated the unprepared schema")
		}
		rejectInstalledProcess(t, ctx, controlBin, args, replaceServiceEnv(controlEnv, "AUTO_MIGRATE", "1"))
		rejectInstalledProcess(t, ctx, controlBin, args, append(controlEnv, "GITHUB_PUBLISHER_CONFIG_FILE="+publisherConfig))
		commandOK(t, store.ApplyCoreMigration(ctx)) // explicit ephemeral test-admin act
		commandOK(t, store.CheckWorkerSchema(ctx))
		control := startInstalledProcess(t, ctx, controlBin, args, controlEnv)
		endpoint := control.endpoint(t, ctx)
		defer func() { // reap BEFORE schema/pool teardown even on assertion failure
			select {
			case <-control.done:
			default:
				_ = control.cmd.Process.Kill()
				select {
				case <-control.done:
				case <-time.After(5 * time.Second):
					t.Error("Control teardown failed to reap before schema cleanup")
				}
			}
		}()
		_, port, err := net.SplitHostPort(strings.TrimPrefix(endpoint, "https://"))
		commandOK(t, err)
		controlEnv = replaceServiceEnv(controlEnv, "PORT", port)
		client := serviceClient(t, pki, endpoint, engineer)
		var health map[string]string
		commandOK(t, client.Call(ctx, http.MethodGet, "/healthz", nil, &health))
		if health["status"] != "ok" {
			t.Fatal("installed Control is not responding")
		}
		post := func(path string, value any) map[string]json.RawMessage {
			t.Helper()
			var response map[string]json.RawMessage
			commandOK(t, client.Call(ctx, http.MethodPost, path, value, &response))
			return response
		}
		post("/api/v1/work-items", map[string]any{"work_item_id": "installed-work", "title": "installed service fixture", "human_owner": engineer})
		task := post("/api/v1/task-contracts", map[string]any{
			"contract":  map[string]any{"task_contract_id": "installed-task", "work_item_id": "installed-work", "task_type": "FEATURE"},
			"material":  map[string]any{"repository": "repo", "base_commit": strings.Repeat("a", 40), "target_id": "target", "acceptance_criteria": []string{"tests pass"}},
			"subsystem": "driver", "verification_plan": map[string]any{"verification_plan_id": "installed-plan", "criteria": []any{
				map[string]any{"criterion_id": "ac", "statement": "tests pass", "evidence_requirements": []any{map[string]any{"requirement_id": "req", "procedure": "ci.test"}}},
			}},
		})
		var taskDigest string
		commandOK(t, json.Unmarshal(task["digest"], &taskDigest))
		post("/api/v1/runs", map[string]any{"run_id": "installed-run", "task_contract_digest": taskDigest, "attempt_id": "attempt", "run_input": map[string]any{"runtime_profile": "codex", "tool_profile": "read", "worker_profile": "worker/installed-test", "policy_profile": "policy"}})
		for {
			status, err := store.GetInbox(ctx, "installed-run")
			if err == nil && status.State == "PENDING" {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("installed Control relay did not produce admission work")
			case <-time.After(20 * time.Millisecond):
			}
		}
		wc, wk := serviceClientFiles(t, config, "worker-client", pki.ClientCertificate(t, worker))
		workerEnv := []string{"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + endpoint, "CONTROL_CLIENT_CERT_FILE=" + wc, "CONTROL_CLIENT_KEY_FILE=" + wk, "CONTROL_SERVER_CA_FILE=" + ca}
		cmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--admission-only", "--profile=worker/installed-test", "--once")
		cmd.Env = workerEnv // excludes DB and Publisher credentials
		raw, err := cmd.CombinedOutput()
		commandOK(t, err)
		var receipt workerqueue.Receipt
		commandOK(t, json.Unmarshal(raw, &receipt))
		if receipt.Kind != workerqueue.Validated || receipt.Worker != worker || receipt.Validation.ExecutionStarted {
			t.Fatal("installed Worker did not return pure admission proof")
		}
		expected, err := json.Marshal(receipt)
		commandOK(t, err)
		checkState := func() {
			t.Helper()
			var observed workerqueue.Status
			commandOK(t, client.Call(ctx, http.MethodGet, "/api/v1/runs/installed-run/inbox", nil, &observed))
			got, err := json.Marshal(observed.Receipt)
			commandOK(t, err)
			if observed.State != workerqueue.Validated || !bytes.Equal(got, expected) {
				t.Fatal("installed Control restart lost or changed admitted receipt")
			}
		}
		checkState()
		// All these counts/digests must remain stable across service restart.
		durable := func() string {
			t.Helper()
			var observed string
			commandOK(t, pool.QueryRow(ctx, `SELECT json_build_array(
 (SELECT json_agg(row(sequence,event_digest) ORDER BY sequence) FROM audit_events),
 (SELECT json_agg(row(outbox_id,state,attempt_count) ORDER BY outbox_id) FROM outbox_events),
 (SELECT count(*) FROM worker_inbox), (SELECT count(*) FROM worker_codex_executions),
 (SELECT count(*) FROM external_operations), (SELECT count(*) FROM evidence),
 (SELECT json_agg(row(recovery_epoch,recovery_mode)) FROM platform_state)
 )::text`).Scan(&observed))
			return observed
		}
		before := durable()
		var apiErr *controlclient.HTTPError
		unknown := serviceClient(t, pki, endpoint, stranger)
		if err := unknown.Call(ctx, http.MethodGet, "/api/v1/runs/installed-run/inbox", nil, &health); !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("Control accepted an unknown client identity: %v", err)
		}
		limited := serviceClient(t, pki, endpoint, worker)
		if err := limited.Call(ctx, http.MethodPost, "/api/v1/work-items", map[string]string{"work_item_id": "denied"}, &health); !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
			t.Fatalf("Control accepted a Worker-only identity for Work creation: %v", err)
		}
		rejectInstalledProcess(t, ctx, controlBin, args, controlEnv) // already-bound port
		for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
			control.stop(t, signal)
			assertServiceClosed(t, endpoint)
			control = startInstalledProcess(t, ctx, controlBin, args, controlEnv)
			if control.endpoint(t, ctx) != endpoint {
				t.Fatal("Control restart changed requested listener")
			}
			checkState()
			// A started daemon cannot manufacture an overall-ready claim, even
			// when another component (Publisher) is absent during this snapshot.
			publisher.stop(t, syscall.SIGTERM)
			assertServiceClosed(t, publisherEndpoint)
			var status production.OperationalStatus
			commandOK(t, client.Call(ctx, http.MethodGet, "/api/v1/operations/status", nil, &status))
			if status.Ready || status.ProductionQualified || status.ServiceReadiness != production.ServiceReadinessUnobserved {
				t.Fatal("Control asserted unobserved service readiness")
			}
			publisher = startInstalledProcess(t, ctx, filepath.Join(bin, "publisher-service"), nil, publisherEnv)
			if publisher.endpoint(t, ctx) != publisherEndpoint {
				t.Fatal("Publisher listener drift")
			}
			checkPublisher()
			// Observe beyond two real relay intervals, not just immediately
			// after the new listener opens. This is a test window, not an SLO.
			for i := 0; i < 3; i++ {
				select {
				case <-ctx.Done():
					t.Fatal("restart observation window expired")
				case <-time.After(500 * time.Millisecond):
				}
				if durable() != before {
					t.Fatal("service restart mutated authority/audit/outbox or replayed work")
				}
			}
		}
		commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM evidence").Scan(&count))
		if count != 0 {
			t.Fatal("service lifecycle created engineering Evidence")
		}
		control.stop(t, syscall.SIGTERM)
		assertServiceClosed(t, endpoint)
		t.Log("installed Control --production + Publisher + Worker: explicit test migration, actual mTLS/DB, admission, port conflict, SIGTERM and quiescent SIGKILL/restart preserve receipt/audit/outbox/epoch; no model/production qualification")
	}
	publisher.stop(t, syscall.SIGTERM)
	assertServiceClosed(t, publisherEndpoint)
}
