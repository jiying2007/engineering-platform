package postgres

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/verification"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

func continuationPolicy(profileDigest string) access.Document {
	doc := controlWirePolicy(continuationOwner, "urn:engineering-platform:viewer:continuation", "worker/codex", profileDigest)
	doc.Principals[0].Capabilities = append(doc.Principals[0].Capabilities, access.RunStart)
	doc.Principals = append(doc.Principals,
		access.PrincipalSpec{Subject: "urn:engineering-platform:engineer:start-only", Scope: "platform", Capabilities: []string{access.Read, access.RunStart}},
		access.PrincipalSpec{Subject: "urn:engineering-platform:engineer:control-only", Scope: "platform", Capabilities: []string{access.Read, access.RunControl}},
		access.PrincipalSpec{Subject: "urn:engineering-platform:engineer:not-owner", Scope: "platform", Capabilities: []string{access.Read, access.RunStart, access.RunControl}})
	return doc
}
func TestContinuationPolicyValidatesWithoutDatabase(t *testing.T) {
	_, e := access.New(continuationPolicy(canonical.BytesDigest([]byte("fixture"))))
	workerOK(t, e)
}

// Full production API/PG/Preparer/Worker/Git/kernel path. Only the model protocol
// is replaced by a local subprocess. No live account/provider/device evidence.
func TestCodexContinuationMTLSTwoActualWorkerExecutions(t *testing.T) {
	s := integrationStore(t)
	testsupport.RequireProcessNamespaces(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	root := t.TempDir()
	workerOK(t, os.Chmod(root, 0700))
	repo, objects, preparedRoot := filepath.Join(root, "repository"), filepath.Join(root, "context"), filepath.Join(root, "prepared")
	for _, p := range []string{repo, objects, preparedRoot} {
		workerOK(t, os.Mkdir(p, 0700))
	}
	git, e := exec.LookPath("git")
	workerOK(t, e)
	git, e = filepath.EvalSymlinks(git)
	workerOK(t, e)
	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, git, append([]string{"-C", dir}, args...)...)
		data, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("test Git: %v %s", e, data)
		}
		return strings.TrimSpace(string(data))
	}
	runGit(repo, "init")
	runGit(repo, "config", "user.name", "test-only")
	runGit(repo, "config", "user.email", "test@example.invalid")
	workerOK(t, os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("source\n"), 0600))
	runGit(repo, "add", "hello.txt")
	runGit(repo, "commit", "-m", "original base")
	base := runGit(repo, "rev-parse", "HEAD")
	python, e := exec.LookPath("python3")
	workerOK(t, e)
	binary := filepath.Join(root, "fixture-codex")
	program := []byte("#!" + python + " -S\n" + testsupport.ContinuationCodexProtocol)
	workerOK(t, os.WriteFile(binary, program, 0700))
	login := filepath.Join(root, "auth.json")
	workerOK(t, os.WriteFile(login, []byte(`{"fixture_only":"not-a-real-credential"}`), 0600))
	profile := codexexec.Profile{Version: 3, Provider: provideridentity.OpenAIChatGPTTrustedSelfHosted(), CodexVersion: "0.157.1", BinaryDigest: canonical.BytesDigest(program), QualificationDigest: canonical.BytesDigest([]byte("TEST-ONLY not live qualification")), EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), Model: "fixture-only", Sandbox: "workspace-write", ApprovalPolicy: "never"}
	pd, e := profile.Digest()
	workerOK(t, e)
	work := core.WorkItem{ID: "continue-work", Title: "source continuation test", HumanOwner: continuationOwner, State: core.WorkDraft, Version: 1, CreatedAt: time.Now().UTC()}
	workerOK(t, s.CreateWork(work))
	plan := verification.Plan{ID: "continue-plan", Criteria: []verification.Criterion{{ID: "all-source", Statement: "verify full diff from original base", Requirements: []verification.EvidenceRequirement{{ID: "test", Procedure: "test"}}}}}
	planDigest, e := plan.Digest()
	workerOK(t, e)
	task := core.TaskContract{ID: "continue-task", WorkItemID: work.ID, TaskType: "FEATURE", Repository: "logical-repository", BaseCommit: base, Revision: 1, AcceptanceCriteria: []string{"preserve interrupted source and complete the second change"}, AllowedActions: []string{codexexec.Action}, VerificationPlanID: plan.ID, VerificationPlanDigest: planDigest}
	taskDigest, e := task.Digest()
	workerOK(t, e)
	work.ActiveTaskContractDigest = taskDigest
	work.State = core.WorkReady
	workerOK(t, s.CreateTaskAndUpdateWork(task, plan, 1, work))
	input := core.RunInputManifest{RunID: "source-run", TaskContractDigest: taskDigest, RuntimeProfile: "codex/runtime", ToolProfile: "codex/" + pd, WorkerProfile: "worker/codex", PolicyProfile: "policy"}
	inputDigest, e := input.Digest()
	workerOK(t, e)
	value := run.New(input.RunID, taskDigest, inputDigest)
	attempt, e := value.StartAttempt("first", time.Now().UTC())
	workerOK(t, e)
	sess := session.New(input.RunID, 1)
	work, e = s.GetWork(work.ID)
	workerOK(t, e)
	wv := work.Version
	work.State = core.WorkExecuting
	work.ActiveRunID = value.ID
	workerOK(t, s.CreateExecutionAndUpdateWork(*value, attempt, *sess, input, wv, work))
	relayWorkerFixture(t, s)
	policy, e := access.New(continuationPolicy(pd))
	workerOK(t, e)
	handler, e := api.NewAuthenticatedHandler(s, nil, policy, api.AuthenticatedOptions{})
	workerOK(t, e)
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client := func(subject string) *controlclient.Client {
		t.Helper()
		c, e := controlclient.New(server.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, subject)}})
		workerOK(t, e)
		t.Cleanup(c.Close)
		return c
	}
	worker, engineer := client(codexTestWorker), client(continuationOwner)
	cfg := preparation.Configuration{Version: 1, Worker: codexTestWorker, Root: preparedRoot, Git: git, ContextSource: objects, Approvals: []preparation.Approval{{RunID: value.ID, TaskDigest: taskDigest, InputDigest: inputDigest, Repository: task.Repository, RepositoryPath: repo}}}
	p, e := preparation.New(cfg)
	workerOK(t, e)
	defer p.Close()
	prep, e := workeragent.PrepareOnce(ctx, worker, input.WorkerProfile, p)
	workerOK(t, e)
	if prep == nil {
		t.Fatal("missing actual preparation")
	}
	request := codexexec.Start{RunID: input.RunID, WorkerProfile: input.WorkerProfile, Profile: profile}
	runtime := workeragent.CodexRuntime{Executable: binary, SavedLoginFile: login}
	done := make(chan error, 1)
	go func() { _, e := workeragent.ExecuteCodex(ctx, worker, p, request, runtime); done <- e }()
	waitFor(t, func() bool {
		r, e := s.GetCodexControlRuntime(ctx, input.RunID)
		return e == nil && r != nil && r.State == "ACTIVE"
	})
	control := codexexec.ControlInput{ID: "explicit-stop", ExecutionEpoch: 1, Sequence: 1, Actor: continuationOwner, ThreadID: "thread", TurnID: "turn"}
	workerOK(t, engineer.Call(ctx, http.MethodPost, "/api/v1/runs/"+input.RunID+"/interrupt", control, nil))
	var retained *workeragent.CheckpointRetainedError
	select {
	case e = <-done:
		if !errors.As(e, &retained) {
			t.Fatalf("interrupted source not retained: %v", e)
		}
	case <-ctx.Done():
		t.Fatal("source worker timeout")
	}
	if !retained.Registered {
		t.Fatal("actual checkpoint registration not confirmed")
	}
	old, e := s.GetCodex(ctx, input.RunID)
	workerOK(t, e)
	if old.State != codexexec.Unknown || old.SourceCheckpoint == nil || !old.Runtime.Transcript.Continuable() {
		t.Fatal("source is not continuable")
	}
	oldBytes, _ := json.Marshal(old.Runtime)
	cd, e := retained.Artifact.Facts.Digest()
	workerOK(t, e)
	current, _, e := s.GetExecution(input.RunID)
	workerOK(t, e)
	work, e = s.GetWork(work.ID)
	workerOK(t, e)
	q := codexexec.ContinueRequest{SourceRunID: input.RunID, RunID: "next-run", AttemptID: "next-attempt", CheckpointDigest: cd, ExpectedRunVersion: current.Version, ExpectedWorkVersion: work.Version, ExecutionEpoch: current.CurrentEpoch, RecoveryEpoch: old.Token.RecoveryEpoch, Reason: "Explicitly continue these reviewed source bytes in a new Run"}
	for _, who := range []string{"urn:engineering-platform:engineer:start-only", "urn:engineering-platform:engineer:control-only"} {
		expectHTTP(t, client(who).Call(ctx, http.MethodPost, "/api/v1/runs/"+input.RunID+"/continue", q, nil), http.StatusForbidden)
	}
	if e = client("urn:engineering-platform:engineer:not-owner").Call(ctx, http.MethodPost, "/api/v1/runs/"+input.RunID+"/continue", q, nil); e == nil {
		t.Fatal("foreign owner authorized continuation")
	}
	var decision codexexec.ContinuationReceipt
	workerOK(t, engineer.Call(ctx, http.MethodPost, "/api/v1/runs/"+input.RunID+"/continue", q, &decision))
	workerOK(t, decision.Validate())
	workerOK(t, engineer.Call(ctx, http.MethodGet, "/api/v1/runs/"+input.RunID+"/continuation", nil, &decision))
	// The decision does not execute a model or inherit host-path approval.
	assertCount(t, s, "SELECT count(*) FROM worker_codex_executions WHERE run_id='next-run'", 0)
	relayWorkerFixture(t, s)
	nextInputDigest, e := decision.Input.Digest()
	workerOK(t, e)
	cfg.Approvals = append(cfg.Approvals, preparation.Approval{RunID: q.RunID, TaskDigest: taskDigest, InputDigest: nextInputDigest, Repository: task.Repository, RepositoryPath: repo, ContinuationArchive: retained.Artifact.Path})
	nextPreparer, e := preparation.New(cfg)
	workerOK(t, e)
	defer nextPreparer.Close()
	nextPrep, e := workeragent.PrepareOnce(ctx, worker, decision.Input.WorkerProfile, nextPreparer)
	workerOK(t, e)
	if nextPrep == nil || nextPrep.Facts.SeedCheckpointDigest != cd || nextPrep.Facts.SeedSourceDigest == "" || nextPrep.Facts.SourceDigest != prep.Facts.SourceDigest {
		t.Fatal("seed or original base not retained")
	}
	nextRequest := request
	nextRequest.RunID = q.RunID
	final, e := workeragent.ExecuteCodex(ctx, worker, nextPreparer, nextRequest, runtime)
	workerOK(t, e)
	if final.Kind != codexexec.Kind || final.Token.RunID != q.RunID || final.Token.ID == old.Token.ID || final.Result.Change.BaseCommit != base || final.Result.Change.BaseSourceDigest != prep.Facts.SourceDigest || final.Result.Codex.ProcessScope.NamespaceID == old.Runtime.Transcript.Close.ProcessScope.NamespaceID {
		t.Fatal("new execution or full-base lineage lost")
	}
	sourceNow, e := s.GetCodex(ctx, input.RunID)
	workerOK(t, e)
	sourceNowBytes, _ := json.Marshal(sourceNow.Runtime)
	if sourceNow.State != codexexec.Stopped || sourceNow.Receipt != nil || string(sourceNowBytes) != string(oldBytes) {
		t.Fatal("old outcome was replayed or rewritten")
	}
	// Inspect the actual result commit/bundle, not a synthetic Result structure.
	bundle := filepath.Join(preparedRoot, "artifacts", final.Token.ID+".bundle")
	runGit(repo, "bundle", "verify", bundle)
	runGit(repo, "fetch", bundle, "HEAD")
	content := runGit(repo, "show", final.Result.Change.ResultCommit+":hello.txt")
	if content != "first change retained\nsecond change completed" {
		t.Fatal("first or second source change lost", content)
	}
	if runGit(repo, "show", final.Result.Change.ResultCommit+":partial.txt") != "unfinished source from first process" {
		t.Fatal("untracked inherited file lost")
	}
	if runGit(repo, "rev-parse", "HEAD") != base {
		t.Fatal("authoritative repository modified")
	}
	_, e = sourcecheckpoint.Verify(ctx, retained.Artifact.Path, retained.Artifact.Facts.ArchiveDigest, input.RunID)
	workerOK(t, e)
	assertCount(t, s, "SELECT count(*) FROM codex_continuations", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='run.source-continuation-authorized'", 1)
}
