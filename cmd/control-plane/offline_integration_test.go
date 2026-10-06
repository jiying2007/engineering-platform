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
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/distribution"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sandbox/testutil"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

// Actual local Git, mTLS, PostgreSQL, compiled Worker/eng and real container.
// The computation is a fixture, not Codex/model evidence.
func TestOfflineCommandPreparedBytesContainerAndDurableReceipt(t *testing.T) {
	offlineCommandCapture(t, "probe")
}

// The compiler, actual Worker/eng, mTLS/Core, raw report and retained artifacts
// are now one Run, rather than separate compiler and synthetic-record proofs.
func TestOfflineCommandCCompilerCaptureRestore(t *testing.T) {
	offlineCommandCapture(t, "compiler")
}

func TestOfflineCommandSIGKILLLeavesUnreconciledWithoutReplay(t *testing.T) {
	offlineCommandCapture(t, "crash")
}

func offlineCommandCapture(t *testing.T, mode string) {
	compiler := mode == "compiler"
	crash := mode == "crash"
	if mode != "probe" && !compiler && !crash {
		t.Fatal("unknown offline command test mode", mode)
	}
	var image testutil.Fixture
	if compiler {
		image = testutil.BuildCompiler(t)
	} else {
		image = testutil.Build(t)
	}
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Fatal("real offline command suite requires POSTGRES_TEST_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	bin := installedOfflineDistribution(t, ctx)
	image.Guard = filepath.Join(bin, "sandbox-guard")
	guard, err := os.ReadFile(image.Guard)
	commandOK(t, err)
	profile := sandbox.Profile{Image: image.Image, GuardDigest: sandbox.Hash(guard), Argv: []string{"/probe", "build-output", "good"}, Seconds: 10, Outputs: []sandbox.OutputSpec{{Name: "app.bin", MaxBytes: 64}, {Name: "app.map", MaxBytes: 64}}}
	if compiler {
		profile.Argv = []string{"/usr/bin/gcc", "-nostdlib", "-ffreestanding", "-fno-pie", "-no-pie", "/workspace/main.c", "-Wl,-e,entry,-Map=/tmp/ep-output/app.map", "-o", "/tmp/ep-output/app.elf"}
		profile.Seconds = 15
		profile.Outputs = []sandbox.OutputSpec{{Name: "app.elf", MaxBytes: 128 << 10}, {Name: "app.map", MaxBytes: 64 << 10}}
	}
	if crash {
		profile = sandbox.Profile{Image: image.Image, GuardDigest: sandbox.Hash(guard), Argv: []string{"/probe", "sleep"}, Seconds: 10}
	}
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
	if compiler {
		commandOK(t, os.WriteFile(filepath.Join(repo, "main.c"), []byte("int entry(void) { return 42; }\n"), 0600))
	}
	gitRun("add", ".")
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
	if crash {
		offlineCrashAfterPermit(t, ctx, store, pool, filepath.Join(bin, "worker"), execEnv, worker, workerProfile, input.RunID, root, profile, image)
		return
	}
	denied := profile
	denied.Argv = []string{"/probe", "sleep"}
	config["profile"] = denied
	data, _ = json.Marshal(config)
	commandOK(t, os.WriteFile(offlineFile, data, 0o600))
	if _, err := command(); err == nil {
		t.Fatal("ungranted command allowed")
	}
	changedOutputs := profile
	changedOutputs.Outputs = append([]sandbox.OutputSpec(nil), profile.Outputs...)
	changedOutputs.Outputs[0].MaxBytes++
	config["profile"] = changedOutputs
	data, _ = json.Marshal(config)
	commandOK(t, os.WriteFile(offlineFile, data, 0600))
	if _, err := command(); err == nil {
		t.Fatal("ungranted output contract allowed")
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
	if receipt.Kind != offline.Kind || receipt.Result.ExitCode != 0 || (!compiler && !strings.Contains(string(receipt.Result.Stdout), "OFFLINE_BUILD_PASS")) || receipt.Worker != worker {
		t.Fatalf("invalid actual execution receipt: %#v", receipt)
	}

	if receipt.Result.BuildOutputs == nil || len(receipt.Result.BuildOutputs.Files) != 2 || receipt.Result.Validate(profile) != nil {
		t.Fatal("Worker lost frozen output bytes")
	}
	if compiler {
		if !strings.HasPrefix(string(receipt.Result.BuildOutputs.Files[0].Bytes), "\x7fELF") || !strings.Contains(string(receipt.Result.BuildOutputs.Files[1].Bytes), "entry") {
			t.Fatal("actual compiler output missing")
		}
	} else if string(receipt.Result.BuildOutputs.Files[0].Bytes) != "actual output\x00\xff" {
		t.Fatal("probe output mismatch")
	}
	// The fsynced existing local report must retain exactly the same raw outputs
	// before network reporting. No extra write-capable mount or new ledger exists.
	recordCount := 0
	records := ""
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "offline-") && strings.HasSuffix(d.Name(), ".json") {
			raw, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			var report offline.Report
			if json.Unmarshal(raw, &report) == nil && report.Result.BuildOutputs != nil {
				a, _ := json.Marshal(report.Result)
				b, _ := json.Marshal(receipt.Result)
				if string(a) != string(b) {
					t.Fatal("local result differs from Core receipt")
				}
				recordCount++
				records = filepath.Dir(path)
			}
		}
		return nil
	})
	commandOK(t, err)
	if recordCount != 1 {
		t.Fatal("actual output report not retained", recordCount)
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
	// Capture the actual producer bytes without configuring network credentials.
	// The external anchors here come from this authorized test's known files.
	permitName := "offline-" + receipt.Token.ID + ".json"
	reportName := "offline-" + sandbox.Hash([]byte(receipt.Token.ID + ":report"))[7:] + ".json"
	permitRaw, err := os.ReadFile(filepath.Join(records, permitName))
	commandOK(t, err)
	reportRaw, err := os.ReadFile(filepath.Join(records, reportName))
	commandOK(t, err)
	retained := t.TempDir()
	commandOK(t, os.Chmod(retained, 0700))
	archive := filepath.Join(retained, "offline.tar")
	capture := exec.CommandContext(ctx, filepath.Join(bin, "eng"), "artifact-set", "capture-offline", "--records", records, "--run", input.RunID, "--execution", receipt.Token.ID, "--permit-digest", sandbox.Hash(permitRaw), "--report-digest", sandbox.Hash(reportRaw), "--out", archive)
	capture.Env = []string{"PATH=/usr/bin:/bin"}
	out, err = capture.CombinedOutput()
	if err != nil {
		t.Fatalf("producer capture: %v %s", err, out)
	}
	var captured workeragent.OfflineCaptureReport
	commandOK(t, json.Unmarshal(out, &captured))
	if captured.OutputCount != 2 || captured.Archive.Members != 4 || captured.CoreObservation != "NOT_OBSERVED" || captured.ExecutionAuthorized || captured.ProductionQualified || captured.FullRunBackup {
		t.Fatal("invalid capture scope", captured)
	}
	// Source/Git, Context and producer workspace are really gone. This proves
	// output independence, not a claim that those excluded inputs are backed up.
	for _, dir := range []string{root, repo, source} {
		commandOK(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		}))
		commandOK(t, os.RemoveAll(dir))
		if _, e := os.Stat(dir); !os.IsNotExist(e) {
			t.Fatal("original remains", dir)
		}
	}
	into := filepath.Join(retained, "restored")
	restore := exec.CommandContext(ctx, filepath.Join(bin, "eng"), "artifact-set", "restore", "--archive", archive, "--archive-digest", captured.Archive.ArchiveDigest, "--run", input.RunID, "--into", into)
	restore.Env = []string{"PATH=/usr/bin:/bin"}
	out, err = restore.CombinedOutput()
	if err != nil {
		t.Fatalf("producer restore: %v %s", err, out)
	}
	var restored artifactset.Report
	commandOK(t, json.Unmarshal(out, &restored))
	if restored.ExecutionAuthorized || restored.ProductionQualified || restored.ProducerSemanticsVerified {
		t.Fatal("restore grants authority")
	}
	files := filepath.Join(into, "files")
	read := func(name string) []byte {
		t.Helper()
		raw, e := os.ReadFile(filepath.Join(files, name))
		commandOK(t, e)
		return raw
	}
	if string(read(permitName)) != string(permitRaw) || string(read(reportName)) != string(reportRaw) {
		t.Fatal("producer records changed")
	}
	var restoredPermit offline.Permit
	var restoredReport offline.Report
	commandOK(t, json.Unmarshal(read(permitName), &restoredPermit))
	commandOK(t, json.Unmarshal(read(reportName), &restoredReport))
	commandOK(t, restoredPermit.Check(worker, offline.Start{RunID: input.RunID, WorkerProfile: workerProfile, Profile: profile}))
	commandOK(t, receipt.Verify(worker, restoredPermit, restoredReport.Result))
	for i, member := range []string{"output-000.bin", "output-001.bin"} {
		data := read(member)
		if string(data) != string(receipt.Result.BuildOutputs.Files[i].Bytes) {
			t.Fatal("restored native output drift")
		}
		st, e := os.Stat(filepath.Join(files, member))
		commandOK(t, e)
		if st.Mode().Perm() != 0600 {
			t.Fatal("output automatically executable")
		}
	}
	observed, err := store.GetOffline(ctx, input.RunID)
	commandOK(t, err)
	got, _ = json.Marshal(observed.Receipt)
	if observed.State != offline.Finished || string(got) != string(want) {
		t.Fatal("capture changed Core result")
	}
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM evidence").Scan(&count))
	if count != 0 {
		t.Fatal("capture manufactured Evidence")
	}
	t.Logf("same-Run compiler=%t Worker/Core output -> capture -> original roots removed -> raw/semantic restore PASS; profile=%s", compiler, receipt.Result.ProfileDigest)
}

func offlineCrashAfterPermit(t *testing.T, ctx context.Context, store *pgstore.Store, pool *pgxpool.Pool, workerBin string, env []string, subject, profileName, runID, preparedRoot string, profile sandbox.Profile, image testutil.Fixture) {
	t.Helper()
	cmd := exec.Command(workerBin, "--execute-offline", "--profile", profileName, "--run", runID, "--once")
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	commandOK(t, cmd.Start())
	cleaned := false
	cleanupContainer := func() {
		if cleaned {
			return
		}
		cleaned = true
		list := exec.Command("docker", "ps", "-aq", "--filter", "ancestor="+image.Image, "--filter", "label=engineering-platform.offline")
		raw, err := list.Output()
		if err != nil {
			t.Errorf("list crash fixture containers: %v", err)
			return
		}
		ids := strings.Fields(string(raw))
		if len(ids) != 0 {
			args := append([]string{"rm", "-f"}, ids...)
			if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
				t.Errorf("remove crash fixture containers: %v %s", err, out)
			}
		}
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		cleaned = false
		cleanupContainer()
	})

	var status offline.Status
	var permitPath string
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.GetOffline(ctx, runID)
		if err == nil && current.State == offline.Authorized && current.Token.ID != "" {
			status = current
			_ = filepath.WalkDir(preparedRoot, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr == nil && !d.IsDir() && d.Name() == "offline-"+current.Token.ID+".json" {
					permitPath = path
				}
				return nil
			})
			if permitPath != "" {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if permitPath == "" {
		t.Fatalf("actual Worker never retained authorized permit; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	permitRaw, err := os.ReadFile(permitPath)
	commandOK(t, err)
	var permit offline.Permit
	commandOK(t, json.Unmarshal(permitRaw, &permit))
	request := offline.Start{RunID: runID, WorkerProfile: profileName, Profile: profile}
	commandOK(t, permit.Check(subject, request))
	if permit.Token != status.Token {
		t.Fatal("local permit differs from Core authorization")
	}

	deadline = time.Now().Add(8 * time.Second)
	renewed := 0
	for time.Now().Before(deadline) {
		commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='worker.offline.renewed'").Scan(&renewed))
		if renewed > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if renewed == 0 {
		t.Fatal("actual Worker did not renew before crash injection")
	}
	commandOK(t, cmd.Process.Kill())
	err = cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("SIGKILL did not terminate Worker as expected: %v", err)
	}
	wait, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || wait.Signal() != syscall.SIGKILL {
		t.Fatalf("Worker termination was not SIGKILL: %v", err)
	}
	cleanupContainer()

	status, err = store.GetOffline(ctx, runID)
	commandOK(t, err)
	if status.State != offline.Authorized || status.Receipt != nil {
		t.Fatal("crashed Worker invented a terminal receipt", status)
	}
	var finished, unknown, executions, evidence int
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='worker.offline.finished'").Scan(&finished))
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='worker.offline.unknown'").Scan(&unknown))
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM worker_offline_executions WHERE run_id=$1", runID).Scan(&executions))
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM evidence").Scan(&evidence))
	if finished != 0 || unknown != 0 || executions != 1 || evidence != 0 {
		t.Fatal("SIGKILL created terminal/replayed facts", finished, unknown, executions, evidence)
	}

	second := exec.CommandContext(ctx, workerBin, "--execute-offline", "--profile", profileName, "--run", runID, "--once")
	second.Env = env
	if out, err := second.CombinedOutput(); err == nil {
		t.Fatalf("crashed execution automatically replayed: %s", out)
	}
	commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM worker_offline_executions WHERE run_id=$1", runID).Scan(&executions))
	if executions != 1 {
		t.Fatal("replay created another offline execution", executions)
	}

	recoveryState, err := store.BeginRecovery(status.Token.RecoveryEpoch)
	commandOK(t, err)
	if _, err := store.CreateRecoveryProof(ctx, recoveryState.Epoch, "urn:engineering-platform:operator:crash-reconciler"); !errors.Is(err, pgstore.ErrRecoveryFactsUnresolved) {
		t.Fatalf("unresolved crashed execution did not block recovery proof: %v", err)
	}
	observed, err := store.GetOffline(ctx, runID)
	commandOK(t, err)
	if observed.Receipt != nil || observed.Token != status.Token {
		t.Fatal("recovery observation rewrote crashed execution", observed)
	}
	t.Logf("actual Worker SIGKILL after permit+renewal retained one unresolved execution and blocked Recovery proof; token=%s", status.Token.ID)
}

// The native test runs installed Worker/eng/guard after the source distribution
// is deleted. The Core server stays the real in-process authenticated test host;
// this is not systemd, production configuration or installed publisher proof.
func installedOfflineDistribution(t *testing.T, ctx context.Context) string {
	t.Helper()
	parent := t.TempDir()
	dist := filepath.Join(parent, "distribution")
	script, err := filepath.Abs("../../scripts/build-distribution.sh")
	commandOK(t, err)
	cmd := exec.CommandContext(ctx, "bash", script, dist)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build complete distribution: %v %s", err, out)
	}
	verified, err := distribution.Verify(dist)
	commandOK(t, err)
	installed := filepath.Join(parent, "installed")
	cmd = exec.CommandContext(ctx, filepath.Join(dist, "eng"), "distribution-install", "--from", dist, "--into", installed, "--source-commit", verified.SourceCommit)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install actual distribution: %v %s", err, out)
	}
	commandOK(t, os.RemoveAll(dist))
	if _, err := os.Stat(dist); !os.IsNotExist(err) {
		t.Fatal("original distribution remains")
	}
	_, err = distribution.VerifyInstallation(ctx, installed, verified.SourceCommit)
	commandOK(t, err)
	t.Log("actual source-bound installation verified after removing original distribution")
	return filepath.Join(installed, "bin")
}
