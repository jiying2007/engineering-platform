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
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/run"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// Actual Git bytes, compiled worker/eng processes, live mTLS Control Plane/relay
// and PostgreSQL receipt readback. This is a preparation test, not a model pilot.
func TestPreparationCommandRealBytesLeaseAndDurableReadback(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	commandOK(t, err)
	defer admin.Close()
	var nonce [16]byte
	_, err = rand.Read(nonce[:])
	commandOK(t, err)
	schema := "ep_prepare_" + hex.EncodeToString(nonce[:])
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
	commandOK(t, store.PreparationReady(ctx))
	const engineer = "urn:engineering-platform:engineer:prepare-command"
	const worker = "urn:engineering-platform:worker:prepare-command"
	const ordinary = "urn:engineering-platform:worker:input-only"
	const profile = "worker/preparation"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: engineer, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart}},
		{Subject: worker, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport, access.WorkerPrepare}, WorkerProfiles: []string{profile}},
		{Subject: ordinary, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{profile}},
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
		done <- runControlServer(serving, server, listener, false, func(ctx context.Context) error { _, err := store.RelayRunStarts(ctx, "prepare-relay", 16); return err })
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
			t.Error("preparation serving lifecycle did not stop")
		}
	}()
	client, err := controlclient.New(endpoint, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, engineer)}})
	commandOK(t, err)
	defer client.Close()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	source := filepath.Join(base, "context-source")
	root := filepath.Join(base, "prepared")
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
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture Git: %v %s", err, data)
		}
		return strings.TrimSpace(string(data))
	}
	gitRun("init")
	gitRun("config", "user.name", "Preparation Fixture")
	gitRun("config", "user.email", "test@example.invalid")
	commandOK(t, os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("exact source\n"), 0o600))
	gitRun("add", "hello.txt")
	gitRun("commit", "-m", "exact base")
	commit := gitRun("rev-parse", "HEAD")
	contextBytes := []byte("approved engineering context\n")
	digest := canonical.BytesDigest(contextBytes)
	commandOK(t, os.WriteFile(filepath.Join(source, strings.TrimPrefix(digest, "sha256:")+".bin"), contextBytes, 0o400))
	ref := core.ContextRef{Source: "docs:command-spec", Type: "DOCUMENT", Version: "v1", Digest: digest, Trust: core.ContextApproved}
	post := func(path string, body any) map[string]json.RawMessage {
		t.Helper()
		var out map[string]json.RawMessage
		commandOK(t, client.Call(ctx, http.MethodPost, path, body, &out))
		return out
	}
	post("/api/v1/work-items", map[string]any{"work_item_id": "prepare-work", "title": "actual preparation fixture", "human_owner": engineer})
	task := post("/api/v1/task-contracts", map[string]any{
		"contract":          map[string]any{"task_contract_id": "prepare-task", "work_item_id": "prepare-work", "task_type": "FEATURE"},
		"material":          map[string]any{"repository": "local-fixture", "base_commit": commit, "target_id": "target", "acceptance_criteria": []string{"real preparation"}},
		"subsystem":         "driver",
		"verification_plan": map[string]any{"verification_plan_id": "prepare-plan", "criteria": []any{map[string]any{"criterion_id": "ac", "statement": "real preparation", "evidence_requirements": []any{map[string]any{"requirement_id": "req", "procedure": "ci.test"}}}}},
	})
	var taskDigest string
	commandOK(t, json.Unmarshal(task["digest"], &taskDigest))
	input := core.RunInputManifest{RunID: "prepare-run", TaskContractDigest: taskDigest, ContextRefs: []core.ContextRef{ref}, RuntimeProfile: "codex", ToolProfile: "read", WorkerProfile: profile, PolicyProfile: "policy"}
	frozen := post("/api/v1/runs", map[string]any{"run_id": input.RunID, "task_contract_digest": taskDigest, "attempt_id": "attempt", "run_input": input})
	var started run.Run
	commandOK(t, json.Unmarshal(frozen["run"], &started))
	inputDigest, err := input.Digest()
	commandOK(t, err)
	if started.RunInputManifestDigest != inputDigest {
		t.Fatal("server changed frozen input")
	}
	prepConfig := preparation.Configuration{Version: 1, Worker: worker, Root: root, Git: git, ContextSource: source, Approvals: []preparation.Approval{{RunID: input.RunID, TaskDigest: taskDigest, InputDigest: inputDigest, Repository: "local-fixture", RepositoryPath: repo, Refs: []core.ContextRef{ref}}}}
	configBytes, err := json.Marshal(prepConfig)
	commandOK(t, err)
	configFile := filepath.Join(base, "preparation.json")
	commandOK(t, os.WriteFile(configFile, configBytes, 0o600))
	ordinaryClient, err := controlclient.New(endpoint, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, ordinary)}})
	commandOK(t, err)
	defer ordinaryClient.Close()
	if err := ordinaryClient.Call(ctx, http.MethodPost, "/api/v1/worker/prepare-claim", map[string]string{"worker_profile": profile}, nil); err == nil {
		t.Fatal("ordinary input worker obtained preparation authority")
	}
	bin := t.TempDir()
	for _, name := range []string{"worker", "eng"} {
		build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, name), "../"+name)
		if data, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v %s", name, err, data)
		}
	}
	environment := func(subject string) []string {
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
	for {
		status, err := store.GetInbox(ctx, input.RunID)
		if err == nil && status.State == "PENDING" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("relay did not transfer preparation input")
		case <-time.After(20 * time.Millisecond):
		}
	}
	process := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--prepare-only", "--profile", profile, "--once")
	process.Env = append(environment(worker), "WORKER_PREPARATION_CONFIG="+configFile, "GIT_DIR=/must-not-inherit", "GIT_CONFIG_PARAMETERS=malformed-parent-config")
	output, err := process.CombinedOutput()
	if err != nil {
		t.Fatalf("compiled preparation Worker: %v %s", err, output)
	}
	var receipt preparation.Receipt
	commandOK(t, json.Unmarshal(output, &receipt))
	if receipt.Kind != preparation.Kind || receipt.Admission.Worker != worker || receipt.Facts.ExecutionStarted || receipt.Facts.OSIsolated || len(receipt.Facts.Context.Entries) != 1 || receipt.Facts.Context.Entries[0].Ref.Digest != digest {
		t.Fatalf("unexpected preparation receipt: %#v", receipt)
	}
	query := exec.CommandContext(ctx, filepath.Join(bin, "eng"), "api", "GET", "/api/v1/runs/prepare-run/preparation")
	query.Env = environment(engineer)
	readback, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("compiled eng readback: %v %s", err, readback)
	}
	var stored preparation.Receipt
	commandOK(t, json.Unmarshal(readback, &stored))
	actual, _ := json.Marshal(stored)
	expected, _ := json.Marshal(receipt)
	if string(actual) != string(expected) {
		t.Fatal("retained preparation receipt changed")
	}
	slots, err := os.ReadDir(filepath.Join(root, "workspaces"))
	commandOK(t, err)
	if len(slots) != 1 {
		t.Fatal("expected one prepared workspace")
	}
	localBytes, err := os.ReadFile(filepath.Join(root, "workspaces", slots[0].Name(), "prepared.json"))
	commandOK(t, err)
	var local preparation.Result
	commandOK(t, json.Unmarshal(localBytes, &local))
	got, err := os.ReadFile(filepath.Join(local.BundlePath, receipt.Facts.Context.Entries[0].File))
	commandOK(t, err)
	if string(got) != string(contextBytes) {
		t.Fatal("prepared bundle did not contain actual approved bytes")
	}
	got, err = os.ReadFile(filepath.Join(local.Workspace.WorktreePath, "hello.txt"))
	commandOK(t, err)
	if string(got) != "exact source\n" {
		t.Fatal("workspace not exact base")
	}
	value, _, err := store.GetExecution(input.RunID)
	commandOK(t, err)
	if value.State != run.Running {
		t.Fatal("preparation completed Run")
	}
	var count int
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM evidence").Scan(&count))
	if count != 0 {
		t.Fatal("preparation fabricated engineering evidence")
	}
}
