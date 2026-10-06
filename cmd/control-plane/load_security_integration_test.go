//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// This is a bounded characterization, not a throughput SLO. It combines real
// PostgreSQL, mTLS Core and installed Worker processes while asserting authority
// and exactly-once admission invariants under concurrent legitimate and denied
// requests. No model, preparation, publication, Delivery or Evidence is run.
func TestSustainedAdmissionSecurityMatrix(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Fatal("sustained matrix requires POSTGRES_TEST_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	bin := installedOfflineDistribution(t, ctx)

	profile := func() string {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "templates/systemd/engineering-worker-admission.service"))
		commandOK(t, err)
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(line, "ExecStart=") {
				continue
			}
			parts := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			if len(parts) != 3 || parts[0] != "/opt/engineering-platform/bin/worker" || parts[2] != "--admission-only" || !strings.HasPrefix(parts[1], "--profile=") {
				t.Fatal("installed admission invocation drift")
			}
			return strings.TrimPrefix(parts[1], "--profile=")
		}
		t.Fatal("installed admission unit missing ExecStart")
		return ""
	}()
	const (
		runCount    = 32
		workerCount = 8
		perWorker   = runCount / workerCount
		engineer    = "urn:engineering-platform:engineer:load-matrix"
		denied      = "urn:engineering-platform:observer:load-matrix"
	)
	if runCount%workerCount != 0 {
		t.Fatal("invalid fixed matrix dimensions")
	}
	workerSubjects := make([]string, workerCount)
	principals := []access.PrincipalSpec{{Subject: engineer, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart}}, {Subject: denied, Scope: "platform", Capabilities: []string{access.Read}}}
	for i := range workerSubjects {
		workerSubjects[i] = fmt.Sprintf("urn:engineering-platform:worker:load-%02d", i)
		principals = append(principals, access.PrincipalSpec{Subject: workerSubjects[i], Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{profile}})
	}
	policy, err := access.New(access.Document{Version: 1, Principals: principals})
	commandOK(t, err)

	admin, err := pgxpool.New(ctx, dsn)
	commandOK(t, err)
	defer admin.Close()
	var nonce [16]byte
	_, err = rand.Read(nonce[:])
	commandOK(t, err)
	schema := "ep_load_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	commandOK(t, err)
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, e := admin.Exec(clean, "DROP SCHEMA "+quoted+" CASCADE"); e != nil {
			t.Errorf("cleanup load matrix schema: %v", e)
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
	commandOK(t, store.CheckWorkerSchema(ctx))

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
		done <- runControlServer(serving, server, listener, false, func(c context.Context) error {
			_, e := store.RelayRunStarts(c, "load-matrix-relay", 64)
			return e
		})
	}()
	defer func() {
		stopServer()
		select {
		case e := <-done:
			if e != nil {
				t.Errorf("load matrix Core shutdown: %v", e)
			}
		case <-time.After(12 * time.Second):
			_ = server.Close()
			t.Error("load matrix Core failed to join")
		}
	}()

	newClient := func(subject string) *controlclient.Client {
		t.Helper()
		client, e := controlclient.New(endpoint, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, subject)}})
		commandOK(t, e)
		t.Cleanup(client.Close)
		return client
	}
	envFor := func(subject string) []string {
		t.Helper()
		dir := t.TempDir()
		cert := pki.ClientCertificate(t, subject)
		key, e := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		commandOK(t, e)
		cp, kp, ca := filepath.Join(dir, "client.pem"), filepath.Join(dir, "client.key"), filepath.Join(dir, "ca.pem")
		commandOK(t, os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600))
		commandOK(t, os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600))
		commandOK(t, os.WriteFile(ca, pki.CAPEM, 0600))
		return []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + endpoint, "CONTROL_CLIENT_CERT_FILE=" + cp, "CONTROL_CLIENT_KEY_FILE=" + kp, "CONTROL_SERVER_CA_FILE=" + ca}
	}

	// Create frozen Work/Task/Run inputs concurrently through authenticated Core.
	intakeStart := time.Now()
	intakeErrs := make(chan error, runCount)
	var intakeWG sync.WaitGroup
	for lane := 0; lane < 4; lane++ {
		intakeWG.Add(1)
		go func(lane int) {
			defer intakeWG.Done()
			client := newClient(engineer)
			for index := lane; index < runCount; index += 4 {
				label := fmt.Sprintf("load-%02d", index)
				call := func(path string, in any, out any) error { return client.Call(ctx, http.MethodPost, path, in, out) }
				if e := call("/api/v1/work-items", map[string]any{"work_item_id": label + "-work", "title": "load matrix", "human_owner": engineer}, nil); e != nil {
					intakeErrs <- fmt.Errorf("%s work: %w", label, e)
					continue
				}
				var task map[string]json.RawMessage
				if e := call("/api/v1/task-contracts", map[string]any{
					"contract": map[string]any{"task_contract_id": label + "-task", "work_item_id": label + "-work", "task_type": "FEATURE"},
					"material": map[string]any{"repository": "fixture", "base_commit": strings.Repeat("a", 40), "target_id": "target", "acceptance_criteria": []string{"admission integrity"}},
					"subsystem": "driver",
					"verification_plan": map[string]any{"verification_plan_id": label + "-plan", "criteria": []any{map[string]any{"criterion_id": "ac", "statement": "admission integrity", "evidence_requirements": []any{map[string]any{"requirement_id": "req", "procedure": "ci.test"}}}}},
				}, &task); e != nil {
					intakeErrs <- fmt.Errorf("%s task: %w", label, e)
					continue
				}
				var digest string
				if e := json.Unmarshal(task["digest"], &digest); e != nil {
					intakeErrs <- fmt.Errorf("%s digest: %w", label, e)
					continue
				}
				if e := call("/api/v1/runs", map[string]any{
					"run_id": label + "-run", "task_contract_digest": digest, "attempt_id": label + "-attempt",
					"run_input": map[string]any{"runtime_profile": "codex", "tool_profile": "read", "worker_profile": profile, "policy_profile": "policy"},
				}, nil); e != nil {
					intakeErrs <- fmt.Errorf("%s run: %w", label, e)
				}
			}
		}(lane)
	}
	intakeWG.Wait()
	close(intakeErrs)
	for e := range intakeErrs {
		if e != nil {
			t.Fatal(e)
		}
	}
	intakeDuration := time.Since(intakeStart)

	deadline := time.Now().Add(12 * time.Second)
	for {
		var pending int
		commandOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM worker_inbox WHERE state='PENDING'").Scan(&pending))
		if pending == runCount {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay did not expose all pending inputs: %d/%d", pending, runCount)
		}
		time.Sleep(20 * time.Millisecond)
	}

	type processResult struct {
		receipt  workerqueue.Receipt
		duration time.Duration
		err      error
		output   string
	}
	validResults := make(chan processResult, runCount)
	deniedResults := make(chan processResult, workerCount*2)
	workerStart := time.Now()
	var workers sync.WaitGroup
	for i, subject := range workerSubjects {
		workers.Add(1)
		go func(index int, subject string) {
			defer workers.Done()
			env := envFor(subject)
			for n := 0; n < perWorker; n++ {
				start := time.Now()
				cmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--admission-only", "--profile="+profile, "--once")
				cmd.Env = env
				raw, e := cmd.CombinedOutput()
				result := processResult{duration: time.Since(start), err: e, output: string(raw)}
				if e == nil {
					e = json.Unmarshal(raw, &result.receipt)
					result.err = e
				}
				validResults <- result
			}
		}(i, subject)
	}
	// Security probes run while legitimate workers are active. Neither identity
	// nor profile mismatch is allowed to reach a successful claim.
	for i := 0; i < workerCount; i++ {
		workers.Add(2)
		go func() {
			defer workers.Done()
			start := time.Now()
			cmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--admission-only", "--profile="+profile, "--once")
			cmd.Env = envFor(denied)
			raw, e := cmd.CombinedOutput()
			deniedResults <- processResult{duration: time.Since(start), err: e, output: string(raw)}
		}()
		go func(subject string) {
			defer workers.Done()
			start := time.Now()
			cmd := exec.CommandContext(ctx, filepath.Join(bin, "worker"), "--admission-only", "--profile=worker/codex-production", "--once")
			cmd.Env = envFor(subject)
			raw, e := cmd.CombinedOutput()
			deniedResults <- processResult{duration: time.Since(start), err: e, output: string(raw)}
		}(workerSubjects[i])
	}
	workers.Wait()
	close(validResults)
	close(deniedResults)
	workerWall := time.Since(workerStart)

	seen := map[int64]bool{}
	durations := make([]time.Duration, 0, runCount)
	for result := range validResults {
		if result.err != nil {
			t.Fatalf("authorized Worker failed: %v: %s", result.err, result.output)
		}
		if result.receipt.Kind != workerqueue.Validated || result.receipt.Worker == "" ||
			result.receipt.Token.Profile != profile || result.receipt.Validation.ExecutionStarted ||
			seen[result.receipt.Token.InboxID] {
			t.Fatalf("duplicate or invalid admission receipt: %#v", result.receipt)
		}
		seen[result.receipt.Token.InboxID] = true
		durations = append(durations, result.duration)
	}
	if len(seen) != runCount {
		t.Fatalf("validated %d/%d unique inboxes", len(seen), runCount)
	}
	deniedCount := 0
	for result := range deniedResults {
		deniedCount++
		if result.err == nil || !strings.Contains(result.output, "403") {
			t.Fatalf("security probe did not fail with authorization denial: %v %q", result.err, result.output)
		}
	}
	if deniedCount != workerCount*2 {
		t.Fatalf("security probe count drift: %d", deniedCount)
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		commandOK(t, pool.QueryRow(ctx, query, args...).Scan(&n))
		return n
	}
	if n := count("SELECT count(*) FROM worker_inbox WHERE state='INPUT_VALIDATED' AND receipt_json IS NOT NULL"); n != runCount {
		t.Fatalf("terminal inbox count %d/%d", n, runCount)
	}
	if n := count("SELECT count(*) FROM worker_inbox WHERE lease_generation<>1"); n != 0 {
		t.Fatalf("duplicate claims observed: %d", n)
	}
	if n := count("SELECT count(*) FROM audit_events WHERE event_type='worker.input.claimed'"); n != runCount {
		t.Fatalf("claim audit count %d/%d", n, runCount)
	}
	if n := count("SELECT count(*) FROM audit_events WHERE event_type='worker.input.validated'"); n != runCount {
		t.Fatalf("validation audit count %d/%d", n, runCount)
	}
	if n := count("SELECT count(*) FROM runs WHERE state<>'RUNNING'"); n != 0 {
		t.Fatalf("admission changed Run state for %d rows", n)
	}
	for _, table := range []string{"worker_preparations", "worker_codex_executions", "worker_offline_executions", "external_operations", "evidence", "delivery_receipts"} {
		if n := count("SELECT count(*) FROM " + table); n != 0 {
			t.Fatalf("load/security matrix created forbidden %s rows: %d", table, n)
		}
	}

	status, err := store.ReadOperationalStatus(ctx)
	commandOK(t, err)
	if status.Ready || status.ProductionQualified || status.ServiceReadiness != production.ServiceReadinessUnobserved ||
		len(status.Snapshot.WorkerPolls) != 1 || status.Snapshot.WorkerPolls[0].WorkerProfile != profile ||
		status.Snapshot.WorkerPolls[0].KnownIdentities != workerCount {
		t.Fatalf("worker load facts promoted readiness or lost identity coverage: %#v", status)
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	percentile := func(p int) time.Duration {
		index := (len(durations)*p + 99) / 100
		if index < 1 {
			index = 1
		}
		if index > len(durations) {
			index = len(durations)
		}
		return durations[index-1]
	}
	t.Logf("LOAD_CHARACTERIZATION_NOT_SLO runs=%d worker_identities=%d denied=%d intake_wall=%s worker_wall=%s process_p50=%s process_p95=%s process_max=%s",
		runCount, workerCount, deniedCount, intakeDuration, workerWall, percentile(50), percentile(95), durations[len(durations)-1])
}
