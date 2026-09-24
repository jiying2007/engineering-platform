package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/run"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func commandOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// This uses the actual serving lifecycle, real PostgreSQL and compiled worker/eng
// subprocesses. Only certificates, task data and input identities are synthetic;
// no installed Codex, model call, provider credential or tool execution is used.
func TestWorkerCommandMTLSRelayAdmissionAndShutdown(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	commandOK(t, err)
	defer admin.Close()
	var nonce [16]byte
	_, err = rand.Read(nonce[:])
	commandOK(t, err)
	schema := "ep_cmd_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	commandOK(t, err)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("clean command test schema: %v", err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(url)
	commandOK(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	commandOK(t, err)
	store := pgstore.New(pool)
	defer store.Close()
	commandOK(t, store.ApplyCoreMigration(ctx))
	commandOK(t, store.CheckWorkerSchema(ctx))

	const engineer = "urn:engineering-platform:engineer:command"
	const worker = "urn:engineering-platform:worker:command"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: engineer, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart}},
		{Subject: worker, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{"worker/ubuntu"}},
	}})
	commandOK(t, err)
	pki := testsupport.NewPKI(t)
	server, err := assembleServer(configuration{policy: policy, tls: pki.ServerTLS()}, store)
	commandOK(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	commandOK(t, err)
	defer listener.Close()
	endpoint := "https://" + listener.Addr().String()
	serving, stopServer := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- runControlServer(serving, server, listener, false, func(ctx context.Context) error {
			_, err := store.RelayRunStarts(ctx, "command-relay", 16)
			return err
		})
	}()
	defer func() {
		stopServer()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("command lifecycle shutdown: %v", err)
			}
		case <-time.After(12 * time.Second):
			_ = server.Close()
			t.Error("command lifecycle failed to join HTTP/relay")
		}
	}()
	client, err := controlclient.New(endpoint, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, engineer)}})
	commandOK(t, err)
	defer client.Close()
	post := func(p string, value any) map[string]json.RawMessage {
		t.Helper()
		var response map[string]json.RawMessage
		commandOK(t, client.Call(ctx, http.MethodPost, p, value, &response))
		return response
	}
	post("/api/v1/work-items", map[string]any{"work_item_id": "cmd-work", "title": "command fixture", "human_owner": engineer})
	task := post("/api/v1/task-contracts", map[string]any{
		"contract":  map[string]any{"task_contract_id": "cmd-task", "work_item_id": "cmd-work", "task_type": "FEATURE"},
		"material":  map[string]any{"repository": "repo", "base_commit": strings.Repeat("a", 40), "target_id": "target", "acceptance_criteria": []string{"tests pass"}},
		"subsystem": "driver",
		"verification_plan": map[string]any{"verification_plan_id": "cmd-plan", "criteria": []any{
			map[string]any{"criterion_id": "ac", "statement": "tests pass", "evidence_requirements": []any{map[string]any{"requirement_id": "req", "procedure": "ci.test"}}},
		}},
	})
	var taskDigest string
	commandOK(t, json.Unmarshal(task["digest"], &taskDigest))
	post("/api/v1/runs", map[string]any{
		"run_id": "cmd-run", "task_contract_digest": taskDigest, "attempt_id": "attempt",
		"run_input": map[string]any{"runtime_profile": "codex", "tool_profile": "read", "worker_profile": "worker/ubuntu", "policy_profile": "policy"},
	})

	// Compile actual CLI entrypoints. go build uses only the repository modules
	// already supplied by CI; the runtime subprocess environments exclude DB keys.
	bin := t.TempDir()
	for _, name := range []string{"worker", "eng"} {
		build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, name), "../"+name)
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v: %s", name, err, output)
		}
	}
	clientEnv := func(subject string) []string {
		t.Helper()
		dir := t.TempDir()
		cert := pki.ClientCertificate(t, subject)
		key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		commandOK(t, err)
		certFile, keyFile, caFile := filepath.Join(dir, "client.pem"), filepath.Join(dir, "client.key"), filepath.Join(dir, "ca.pem")
		commandOK(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0o600))
		commandOK(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0o600))
		commandOK(t, os.WriteFile(caFile, pki.CAPEM, 0o600))
		return []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + endpoint, "CONTROL_CLIENT_CERT_FILE=" + certFile, "CONTROL_CLIENT_KEY_FILE=" + keyFile, "CONTROL_SERVER_CA_FILE=" + caFile}
	}
	workerEnv := clientEnv(worker)
	denied := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--admission-only", "--profile", "worker/unauthorized", "--once")
	denied.Env = workerEnv
	if _, err := denied.CombinedOutput(); err == nil {
		t.Fatal("compiled Worker accepted an unauthorized profile")
	}
	// Wait only for the real loop to transfer. No direct relay call is made here.
	for {
		status, err := store.GetInbox(ctx, "cmd-run")
		if err == nil && status.State == "PENDING" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("serving loop did not transfer the run intent")
		case <-time.After(20 * time.Millisecond):
		}
	}
	process := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--admission-only", "--profile", "worker/ubuntu", "--once")
	process.Env = workerEnv
	output, err := process.CombinedOutput()
	if err != nil {
		t.Fatalf("compiled Worker failed: %v: %s", err, output)
	}
	var receipt workerqueue.Receipt
	commandOK(t, json.Unmarshal(output, &receipt))
	if receipt.Worker != worker || receipt.Kind != workerqueue.Validated || receipt.Validation.ExecutionStarted || receipt.Validation.ContextBytesVerified {
		t.Fatalf("invalid process receipt: %#v", receipt)
	}
	query := exec.CommandContext(ctx, filepath.Join(bin, "eng"), "api", "GET", "/api/v1/runs/cmd-run/inbox")
	query.Env = clientEnv(engineer)
	output, err = query.CombinedOutput()
	if err != nil {
		t.Fatalf("compiled eng failed: %v: %s", err, output)
	}
	var status workerqueue.Status
	commandOK(t, json.Unmarshal(output, &status))
	got, _ := json.Marshal(status.Receipt)
	want, _ := json.Marshal(receipt)
	if status.State != workerqueue.Validated || string(got) != string(want) {
		t.Fatal("CLI did not return the exact durable receipt")
	}
	value, _, err := store.GetExecution("cmd-run")
	commandOK(t, err)
	if value.State != run.Running {
		t.Fatal("input admission completed a Run")
	}
	var count int
	commandOK(t, pool.QueryRow(ctx, `SELECT count(*) FROM evidence`).Scan(&count))
	if count != 0 {
		t.Fatal("input admission created engineering Evidence")
	}
}

func TestWorkerCommandRelayFailureStopsServing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	commandOK(t, err)
	defer listener.Close()
	expected := errors.New("fixture relay storage failure")
	server := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	err = runControlServer(ctx, server, listener, true, func(context.Context) error { return expected })
	if !errors.Is(err, expected) {
		t.Fatalf("serving lifecycle lost relay failure: %v", err)
	}
}
