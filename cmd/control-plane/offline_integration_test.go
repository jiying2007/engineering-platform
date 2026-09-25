package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io/fs"
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
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sandbox/testutil"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// Actual local Git, mTLS, PostgreSQL, compiled Worker/eng and real container.
// The computation is a fixture, not Codex/model evidence.
func TestOfflineCommandPreparedBytesContainerAndDurableReceipt(t *testing.T) {
	image := testutil.Build(t)
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Fatal("real offline command suite requires POSTGRES_TEST_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	guard, err := os.ReadFile(image.Guard)
	commandOK(t, err)
	profile := sandbox.Profile{Image: image.Image, GuardDigest: sandbox.Hash(guard), Argv: []string{"/probe"}, Seconds: 10}
	pd, err := profile.Digest()
	commandOK(t, err)
	admin, err := pgxpool.New(ctx, url)
	commandOK(t, err)
	defer admin.Close()
	nonce := make([]byte, 16)
	_, err = rand.Read(nonce)
	commandOK(t, err)
	schema := "ep_offline_" + hex.EncodeToString(nonce)
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	commandOK(t, err)
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, err := admin.Exec(clean, "DROP SCHEMA "+quoted+" CASCADE")
		if err != nil {
			t.Error(err)
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
	const engineer = "urn:engineering-platform:engineer:offline"
	const worker = "urn:engineering-platform:worker:offline"
	const workerProfile = "worker/offline"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: engineer, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart}},
		{Subject: worker, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport, access.WorkerPrepare, access.ActionExecute}, WorkerProfiles: []string{workerProfile}, Actions: []access.ActionGrant{{Action: offline.Action, RiskClass: "CONTROLLED_MUTATION", Capability: pd}}},
	}})
	commandOK(t, err)
	pki := testsupport.NewPKI(t)
	server, err := assembleServer(configuration{policy: policy, tls: pki.ServerTLS()}, store)
	commandOK(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	commandOK(t, err)
	endpoint := "https://" + listener.Addr().String()
	serving, stopServer := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- runControlServer(serving, server, listener, false, func(ctx context.Context) error { _, err := store.RelayRunStarts(ctx, "offline-relay", 16); return err })
	}()
	defer func() {
		stopServer()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			_ = server.Close()
			t.Error("serving loop did not join")
		}
	}()
	client, err := controlclient.New(endpoint, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, engineer)}})
	commandOK(t, err)
	defer client.Close()
	base := t.TempDir()
	repo, source, root := filepath.Join(base, "repo"), filepath.Join(base, "source"), filepath.Join(base, "prepared")
	for _, dir := range []string{repo, source, root} {
		commandOK(t, os.Mkdir(dir, 0o700))
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return err
		})
	})
	git, err := exec.LookPath("git")
	commandOK(t, err)
	git, err = filepath.EvalSymlinks(git)
	commandOK(t, err)
	gitRun := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, git, append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	gitRun("init")
	gitRun("config", "user.name", "Offline Fixture")
	gitRun("config", "user.email", "test@example.invalid")
	commandOK(t, os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("real approved source\n"), 0o600))
	gitRun("add", "hello.txt")
	gitRun("commit", "-m", "base")
	commit := gitRun("rev-parse", "HEAD")
	contextBytes := []byte("real approved context\n")
	digest := sandbox.Hash(contextBytes)
	commandOK(t, os.WriteFile(filepath.Join(source, digest[7:]+".bin"), contextBytes, 0o400))
	ref := core.ContextRef{Source: "docs:offline", Type: "DOCUMENT", Version: "v1", Digest: digest, Trust: core.ContextApproved}
	post := func(path string, body any) map[string]json.RawMessage {
		t.Helper()
		var out map[string]json.RawMessage
		commandOK(t, client.Call(ctx, http.MethodPost, path, body, &out))
		return out
	}
	post("/api/v1/work-items", map[string]any{"work_item_id": "offline-work", "title": "offline command fixture", "human_owner": engineer})
	task := post("/api/v1/task-contracts", map[string]any{
		"contract":  map[string]any{"task_contract_id": "offline-task", "work_item_id": "offline-work", "task_type": "FEATURE", "allowed_actions": []string{offline.Action}},
		"material":  map[string]any{"repository": "fixture", "base_commit": commit, "target_id": "target", "acceptance_criteria": []string{"offline check"}},
		"subsystem": "driver", "verification_plan": map[string]any{"verification_plan_id": "offline-plan", "criteria": []any{map[string]any{"criterion_id": "ac", "statement": "offline check", "evidence_requirements": []any{map[string]any{"requirement_id": "req", "procedure": "ci.test"}}}}},
	})
	var td string
	commandOK(t, json.Unmarshal(task["digest"], &td))
	input := core.RunInputManifest{RunID: "offline-run", TaskContractDigest: td, ContextRefs: []core.ContextRef{ref}, RuntimeProfile: "offline-check", ToolProfile: "offline/" + pd, WorkerProfile: workerProfile, PolicyProfile: "policy"}
	inputDigest, err := input.Digest()
	commandOK(t, err)
	post("/api/v1/runs", map[string]any{"run_id": input.RunID, "task_contract_digest": td, "attempt_id": "attempt", "run_input": input})
	prep := preparation.Configuration{Version: 1, Worker: worker, Root: root, Git: git, ContextSource: source, Approvals: []preparation.Approval{{RunID: input.RunID, TaskDigest: td, InputDigest: inputDigest, Repository: "fixture", RepositoryPath: repo, Refs: []core.ContextRef{ref}}}}
	data, _ := json.Marshal(prep)
	configFile := filepath.Join(base, "prepare.json")
	commandOK(t, os.WriteFile(configFile, data, 0o600))
	bin := t.TempDir()
	for _, name := range []string{"worker", "eng"} {
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, name), "../"+name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	env := func(subject string) []string {
		t.Helper()
		dir := t.TempDir()
		cert := pki.ClientCertificate(t, subject)
		key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		commandOK(t, err)
		cp, kp, ca := filepath.Join(dir, "client.pem"), filepath.Join(dir, "client.key"), filepath.Join(dir, "ca.pem")
		commandOK(t, os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0o600))
		commandOK(t, os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0o600))
		commandOK(t, os.WriteFile(ca, pki.CAPEM, 0o600))
		return []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + endpoint, "CONTROL_CLIENT_CERT_FILE=" + cp, "CONTROL_CLIENT_KEY_FILE=" + kp, "CONTROL_SERVER_CA_FILE=" + ca}
	}
	workerEnv := append(env(worker), "WORKER_PREPARATION_CONFIG="+configFile)
	for {
		status, err := store.GetInbox(ctx, input.RunID)
		if err == nil && status.State == "PENDING" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("no relay transfer")
		case <-time.After(20 * time.Millisecond):
		}
	}
	prepareCmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--prepare-only", "--profile", workerProfile, "--once")
	prepareCmd.Env = workerEnv
	out, err := prepareCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare: %v %s", err, out)
	}
	config := map[string]any{"version": 1, "engine_socket": image.Socket, "guard_executable": image.Guard, "profile": profile}
	data, _ = json.Marshal(config)
	offlineFile := filepath.Join(base, "offline.json")
	commandOK(t, os.WriteFile(offlineFile, data, 0o600))
	execEnv := append(workerEnv, "WORKER_OFFLINE_CONFIG="+offlineFile)
	command := func() ([]byte, error) {
		cmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--execute-offline", "--profile", workerProfile, "--run", input.RunID, "--once")
		cmd.Env = execEnv
		return cmd.CombinedOutput()
	}
	denied := profile
	denied.Argv = []string{"/probe", "sleep"}
	config["profile"] = denied
	data, _ = json.Marshal(config)
	commandOK(t, os.WriteFile(offlineFile, data, 0o600))
	if _, err := command(); err == nil {
		t.Fatal("ungranted command allowed")
	}
	config["profile"] = profile
	data, _ = json.Marshal(config)
	commandOK(t, os.WriteFile(offlineFile, data, 0o600))
	out, err = command()
	if err != nil {
		t.Fatalf("offline worker: %v %s", err, out)
	}
	var receipt offline.Receipt
	commandOK(t, json.Unmarshal(out, &receipt))
	if receipt.Kind != offline.Kind || receipt.Result.ExitCode != 0 || !strings.Contains(string(receipt.Result.Stdout), "OFFLINE_PROBE_PASS") || receipt.Worker != worker {
		t.Fatalf("invalid actual execution receipt: %#v", receipt)
	}
	query := exec.CommandContext(ctx, filepath.Join(bin, "eng"), "api", "GET", "/api/v1/runs/"+input.RunID+"/offline")
	query.Env = env(engineer)
	out, err = query.CombinedOutput()
	if err != nil {
		t.Fatalf("eng: %v %s", err, out)
	}
	var status offline.Status
	commandOK(t, json.Unmarshal(out, &status))
	want, _ := json.Marshal(receipt)
	got, _ := json.Marshal(status.Receipt)
	if status.State != offline.Finished || string(want) != string(got) {
		t.Fatal("stored execution changed")
	}
	if _, err = command(); err == nil {
		t.Fatal("actual execution ran twice")
	}
	value, _, err := store.GetExecution(input.RunID)
	commandOK(t, err)
	if value.State != "RUNNING" {
		t.Fatal("offline check completed engineering Run")
	}
	var count int
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM evidence").Scan(&count))
	if count != 0 {
		t.Fatal("offline fixture manufactured Evidence")
	}
	t.Logf("actual offline receipt profile=%s stdout=%s", receipt.Result.ProfileDigest, receipt.Result.StdoutDigest)
}
