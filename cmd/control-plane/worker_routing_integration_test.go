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
	"fmt"
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
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// This is actual installed Worker/Preparer + authenticated in-process Core and
// PostgreSQL. Separate mTLS actors do not claim distinct Unix/systemd identities.
// No model, build, publication, Delivery or engineering Evidence is executed.
func TestOfflineCommandProductionWorkerRouting(t *testing.T) {
	if os.Getenv("EP_SANDBOX_INTEGRATION") != "1" {
		t.Skip("explicit native host integration not requested")
	}
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Fatal("mandatory routing integration requires PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bin := installedOfflineDistribution(t, ctx)
	profile := func(role, mode string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "templates/systemd/engineering-worker-"+role+".service"))
		commandOK(t, err)
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(line, "ExecStart=") {
				continue
			}
			parts := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			if len(parts) != 3 || parts[0] != "/opt/engineering-platform/bin/worker" || !strings.HasPrefix(parts[1], "--profile=") || parts[2] != mode {
				t.Fatal("installed Worker invocation drift", role)
			}
			return strings.TrimPrefix(parts[1], "--profile=")
		}
		t.Fatal("missing installed Worker invocation", role)
		return ""
	}
	admissionProfile, preparationProfile := profile("admission", "--admission-only"), profile("preparation", "--prepare-only")
	if admissionProfile == preparationProfile {
		t.Fatal("production claimers compete for the same terminal inbox")
	}
	const engineer = "urn:engineering-platform:engineer:routing"
	const admit = "urn:engineering-platform:worker:routing-admission"
	const prepare = "urn:engineering-platform:worker:routing-preparation"
	document := access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: engineer, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart}},
		{Subject: admit, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{admissionProfile}},
		{Subject: prepare, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport, access.WorkerPrepare}, WorkerProfiles: []string{preparationProfile}},
	}}
	policy, err := access.New(document)
	commandOK(t, err)
	// The original configuration collision must fail BEFORE a service can publish
	// the policy. No existing Run, lease, schema or history is rewritten to repair it.
	document.Principals[1].WorkerProfiles = []string{preparationProfile}
	if p, e := access.New(document); e == nil || p != nil {
		t.Fatal("overlapping routing policy accepted")
	}
	admin, err := pgxpool.New(ctx, dsn)
	commandOK(t, err)
	defer admin.Close()
	nonce := make([]byte, 16)
	_, err = rand.Read(nonce)
	commandOK(t, err)
	schema := "ep_routing_" + hex.EncodeToString(nonce)
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	commandOK(t, err)
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, e := admin.Exec(clean, "DROP SCHEMA "+quoted+" CASCADE")
		if e != nil {
			t.Error("cleanup only this synthetic schema", e)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	commandOK(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	commandOK(t, err)
	store := pgstore.New(pool)
	defer store.Close()
	commandOK(t, store.ApplyCoreMigration(ctx))
	pki := testsupport.NewPKI(t)
	server, err := assembleServer(configuration{policy: policy, tls: pki.ServerTLS()}, store)
	commandOK(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	commandOK(t, err)
	endpoint := "https://" + listener.Addr().String()
	serving, stopServer := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- runControlServer(serving, server, listener, false, func(ctx context.Context) error { _, err := store.RelayRunStarts(ctx, "routing-relay", 16); return err })
	}()
	defer func() {
		stopServer()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(12 * time.Second):
			_ = server.Close()
			t.Error("routing Core did not stop")
		}
	}()
	client, err := controlclient.New(endpoint, &tls.Config{RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, engineer)}})
	commandOK(t, err)
	defer client.Close()
	base := t.TempDir()
	repo, source, root := filepath.Join(base, "repo"), filepath.Join(base, "context"), filepath.Join(base, "prepared")
	for _, d := range []string{repo, source, root} {
		commandOK(t, os.Mkdir(d, 0700))
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				return os.Chmod(path, 0700)
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
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatal("fixture Git failed", e)
		}
		return strings.TrimSpace(string(out))
	}
	gitRun("init")
	gitRun("config", "user.name", "Routing Fixture")
	gitRun("config", "user.email", "test@example.invalid")
	code := []byte("explicitly approved routing source\n")
	commandOK(t, os.WriteFile(filepath.Join(repo, "hello.txt"), code, 0600))
	gitRun("add", ".")
	gitRun("commit", "-m", "base")
	commit := gitRun("rev-parse", "HEAD")
	contextBytes := []byte("approved routing Context\n")
	digest := canonical.BytesDigest(contextBytes)
	commandOK(t, os.WriteFile(filepath.Join(source, digest[7:]+".bin"), contextBytes, 0400))
	ref := core.ContextRef{Source: "docs:routing", Type: "DOCUMENT", Version: "v1", Digest: digest, Trust: core.ContextApproved}
	post := func(path string, body any) map[string]json.RawMessage {
		t.Helper()
		var out map[string]json.RawMessage
		commandOK(t, client.Call(ctx, http.MethodPost, path, body, &out))
		return out
	}
	createRun := func(label, profile string) core.RunInputManifest {
		t.Helper()
		post("/api/v1/work-items", map[string]any{"work_item_id": label + "-work", "title": "routing fixture", "human_owner": engineer})
		task := post("/api/v1/task-contracts", map[string]any{
			"contract":  map[string]any{"task_contract_id": label + "-task", "work_item_id": label + "-work", "task_type": "FEATURE"},
			"material":  map[string]any{"repository": "fixture", "base_commit": commit, "target_id": "target", "acceptance_criteria": []string{"routing only"}},
			"subsystem": "driver", "verification_plan": map[string]any{"verification_plan_id": label + "-plan", "criteria": []any{map[string]any{"criterion_id": "ac", "statement": "routing only", "evidence_requirements": []any{map[string]any{"requirement_id": "req", "procedure": "ci.test"}}}}},
		})
		var td string
		commandOK(t, json.Unmarshal(task["digest"], &td))
		input := core.RunInputManifest{RunID: label + "-run", TaskContractDigest: td, ContextRefs: []core.ContextRef{ref}, RuntimeProfile: "codex", ToolProfile: "read", WorkerProfile: profile, PolicyProfile: "policy"}
		post("/api/v1/runs", map[string]any{"run_id": input.RunID, "task_contract_digest": td, "attempt_id": label + "-attempt", "run_input": input})
		deadline := time.Now().Add(5 * time.Second)
		for {
			st, e := store.GetInbox(ctx, input.RunID)
			if e == nil && st.State == "PENDING" {
				break
			}
			if ctx.Err() != nil || time.Now().After(deadline) {
				t.Fatal("routing intent not relayed")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return input
	}
	preparedInput := createRun("prepare", preparationProfile)
	inputDigest, err := preparedInput.Digest()
	commandOK(t, err)
	pc := preparation.Configuration{Version: 1, Worker: prepare, Root: root, Git: git, ContextSource: source, Approvals: []preparation.Approval{{RunID: preparedInput.RunID, TaskDigest: preparedInput.TaskContractDigest, InputDigest: inputDigest, Repository: "fixture", RepositoryPath: repo, Refs: []core.ContextRef{ref}}}}
	raw, err := json.Marshal(pc)
	commandOK(t, err)
	configFile := filepath.Join(base, "prepare.json")
	commandOK(t, os.WriteFile(configFile, raw, 0600))
	env := func(subject string) []string {
		t.Helper()
		dir := t.TempDir()
		cert := pki.ClientCertificate(t, subject)
		key, e := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		commandOK(t, e)
		cp, kp, ca := filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key"), filepath.Join(dir, "ca.crt")
		commandOK(t, os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600))
		commandOK(t, os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600))
		commandOK(t, os.WriteFile(ca, pki.CAPEM, 0600))
		return []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + endpoint, "CONTROL_CLIENT_CERT_FILE=" + cp, "CONTROL_CLIENT_KEY_FILE=" + kp, "CONTROL_SERVER_CA_FILE=" + ca}
	}
	admitEnv, prepEnv := env(admit), append(env(prepare), "WORKER_PREPARATION_CONFIG="+configFile)
	workerCmd := func(mode, profile string, environ []string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), mode, "--profile="+profile, "--once")
		cmd.Env = environ
		return cmd
	}
	noWork := func(cmd *exec.Cmd) {
		t.Helper()
		out, e := cmd.CombinedOutput()
		commandOK(t, e)
		if len(bytes.TrimSpace(out)) != 0 {
			t.Fatal("idle worker unexpectedly produced a receipt")
		}
	}
	pending := func() {
		t.Helper()
		st, e := store.GetInbox(ctx, preparedInput.RunID)
		commandOK(t, e)
		if st.State != "PENDING" || st.Generation != 0 || st.Receipt != nil {
			t.Fatal("admission consumed preparation work")
		}
	}
	// Deliberately run admission FIRST while only preparation work is available.
	noWork(workerCmd("--admission-only", admissionProfile, admitEnv))
	pending()
	for _, cmd := range []*exec.Cmd{workerCmd("--admission-only", preparationProfile, admitEnv), workerCmd("--prepare-only", admissionProfile, prepEnv)} {
		if out, e := cmd.CombinedOutput(); e == nil || !strings.Contains(string(out), "403") {
			t.Fatal("cross-profile claim did not fail with authorization denial", e)
		}
		pending()
	}
	admittedInput := createRun("admit", admissionProfile)
	ac, pcmd := workerCmd("--admission-only", admissionProfile, admitEnv), workerCmd("--prepare-only", preparationProfile, prepEnv)
	var ab, pb bytes.Buffer
	ac.Stdout = &ab
	ac.Stderr = &ab
	pcmd.Stdout = &pb
	pcmd.Stderr = &pb
	commandOK(t, ac.Start())
	commandOK(t, pcmd.Start())
	ae, pe := ac.Wait(), pcmd.Wait()
	commandOK(t, ae)
	commandOK(t, pe)
	var ar workerqueue.Receipt
	var pr preparation.Receipt
	commandOK(t, json.Unmarshal(ab.Bytes(), &ar))
	commandOK(t, json.Unmarshal(pb.Bytes(), &pr))
	admittedDigest, err := admittedInput.Digest()
	commandOK(t, err)
	if ar.Kind != workerqueue.Validated || ar.Worker != admit || ar.Token.Profile != admissionProfile || ar.Validation.InputDigest != admittedDigest || ar.Validation.ExecutionStarted {
		t.Fatal("wrong admission identity or execution claim")
	}
	if pr.Kind != preparation.Kind || pr.Admission.Worker != prepare || pr.Admission.Token.Profile != preparationProfile || pr.Facts.InputDigest != inputDigest || pr.Facts.BaseCommit != commit || pr.Facts.ExecutionStarted || len(pr.Facts.Context.Entries) != 1 {
		t.Fatal("wrong preparation identity or execution claim")
	}
	observed, err := store.GetPreparation(ctx, preparedInput.RunID)
	commandOK(t, err)
	if observed.FactsDigest != pr.FactsDigest {
		t.Fatal("preparation receipt not retained")
	}
	slots, err := filepath.Glob(filepath.Join(root, "workspaces", "*", "prepared.json"))
	commandOK(t, err)
	if len(slots) != 1 {
		t.Fatal("exactly one preparation slot required")
	}
	local, err := os.ReadFile(slots[0])
	commandOK(t, err)
	var result preparation.Result
	commandOK(t, json.Unmarshal(local, &result))
	got, err := os.ReadFile(filepath.Join(result.Workspace.WorktreePath, "hello.txt"))
	commandOK(t, err)
	if !bytes.Equal(got, code) || result.Facts.InputDigest != inputDigest {
		t.Fatal("actual prepared bytes drifted")
	}
	original, err := os.ReadFile(filepath.Join(repo, "hello.txt"))
	commandOK(t, err)
	if !bytes.Equal(original, code) || gitRun("rev-parse", "HEAD") != commit {
		t.Fatal("fixture base changed")
	}
	count := func(table string) int {
		t.Helper()
		var n int
		commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		return n
	}
	if count("worker_inbox") != 2 || count("worker_preparations") != 1 {
		t.Fatal("wrong materialization coverage")
	}
	for _, table := range []string{"worker_codex_executions", "worker_offline_executions", "external_operations", "evidence", "delivery_receipts"} {
		if n := count(table); n != 0 {
			t.Fatal("routing created execution/publication/evidence/delivery", table, n)
		}
	}
	audit := count("audit_events")
	noWork(workerCmd("--admission-only", admissionProfile, admitEnv))
	noWork(workerCmd("--prepare-only", preparationProfile, prepEnv))
	if count("audit_events") != audit || count("worker_preparations") != 1 {
		t.Fatal("polling terminal inputs replayed work")
	}
	st, err := store.GetInbox(ctx, admittedInput.RunID)
	commandOK(t, err)
	if st.State != workerqueue.Validated || st.Receipt == nil || st.Receipt.Worker != admit {
		t.Fatal("admission receipt missing")
	}
	t.Log(fmt.Sprintf("installed Worker claim routing: admission-first leaves preparation pending; cross-profile 403 precedes claim; concurrent actors retain separate input/preparation receipts; source/Context materialized once; no Runtime/Delivery/Evidence; profiles=%s,%s", admissionProfile, preparationProfile))
}
