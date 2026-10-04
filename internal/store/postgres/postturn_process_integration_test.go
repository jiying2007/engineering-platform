package postgres

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// Core/PG/mTLS, actual Preparer/Worker/Git/namespace and private restore run
// together. Only model protocol and chosen transport loss are TEST fixtures.
type postTurnLossTransport struct {
	inner  *controlclient.Client
	mode   string
	mu     sync.Mutex
	counts map[string]int
}

func (w *postTurnLossTransport) Subject() string { return w.inner.Subject() }
func (w *postTurnLossTransport) Call(ctx context.Context, method, route string, in, out any) error {
	w.mu.Lock()
	w.counts[route]++
	w.mu.Unlock()
	err := w.inner.Call(ctx, method, route, in, out)
	if err == nil && route == "/api/v1/worker/codex/report" && w.mode != "finalize" {
		return fmt.Errorf("TEST: response lost after Core commit")
	}
	if err == nil && route == "/api/v1/worker/codex/source-checkpoint" && w.mode == "checkpoint-lost" {
		return fmt.Errorf("TEST: checkpoint response lost after Core commit")
	}
	return err
}
func TestPostTurnFailureMTLSRetainsWithoutDowngradingCoreReceipt(t *testing.T) {
	for _, mode := range []string{"finalize", "report-lost", "checkpoint-lost"} {
		t.Run(mode, func(t *testing.T) {
			s := integrationStore(t)
			testsupport.RequireProcessNamespaces(t)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			root := t.TempDir()
			// Context bundles are deliberately read-only in production. Restore only
			// this test's own directory modes after both Preparers have been closed,
			// before testing.TempDir removes its tree as the non-root CI user.
			t.Cleanup(func() {
				if e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if d.IsDir() {
						return os.Chmod(path, 0700)
					}
					return nil
				}); e != nil {
					t.Errorf("test directory cleanup: %v", e)
				}
			})
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
			if mode == "finalize" {
				workerOK(t, os.WriteFile(filepath.Join(repo, "fixture-ignored-mode"), []byte("test\n"), 0600))
			}
			runGit(repo, "add", "-A")
			runGit(repo, "commit", "-m", "original base")
			base := runGit(repo, "rev-parse", "HEAD")
			python, e := exec.LookPath("python3")
			workerOK(t, e)
			binary := filepath.Join(root, "fixture-codex")
			program := []byte("#!" + python + " -S\n" + testsupport.CompletedCodexProtocol)
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

			transport := &postTurnLossTransport{inner: worker, mode: mode, counts: map[string]int{}}
			receipt, err := workeragent.ExecuteCodex(ctx, transport, p, request, runtime)
			var failed *workeragent.PostTurnFailureError
			var retained *workeragent.CheckpointRetainedError
			if receipt.Kind != "" || !errors.As(err, &failed) || !errors.As(err, &retained) || !failed.JournalSaved {
				t.Fatalf("post-turn retention missing: %v", err)
			}
			if retained.Registered != (mode != "checkpoint-lost") {
				t.Fatal("false registration confirmation")
			}
			state, e := s.GetCodex(ctx, input.RunID)
			workerOK(t, e)
			if state.SourceCheckpoint == nil || *state.SourceCheckpoint != retained.Artifact.Facts || state.Runtime.Transcript.Continuable() {
				t.Fatal("checkpoint absent or completed turn grants continuation")
			}
			if state.Runtime.Transcript.Close.TurnStatus != "completed" || !state.Runtime.Transcript.Close.ProcessScope.Quiescent() {
				t.Fatal("lost actual completed/stop proof")
			}
			if mode == "finalize" {
				if state.State != codexexec.Unknown || state.Receipt != nil || failed.Phase != "FINALIZE" {
					t.Fatal("failed finalization promoted success")
				}
			} else {
				if state.State != codexexec.Finished || state.Receipt == nil || failed.Phase != "RESULT_REPORT" {
					t.Fatal("lost reply downgraded committed receipt", state.State, failed.Phase)
				}
				if e = state.VerifyFinishedControls(); e != nil {
					t.Fatal(e)
				}
			}
			if transport.counts["/api/v1/worker/codex/start"] != 1 || transport.counts["/api/v1/worker/codex/source-checkpoint"] != 1 || transport.counts["/api/v1/worker/codex/fail"] != 1 {
				t.Fatal("model/checkpoint replay", transport.counts)
			}
			wantReports := 2
			if mode == "finalize" {
				wantReports = 0
			}
			if transport.counts["/api/v1/worker/codex/report"] != wantReports {
				t.Fatal("metadata report semantics changed")
			}
			destination := filepath.Join(root, "restored")
			facts, e := sourcecheckpoint.Restore(ctx, retained.Artifact.Path, retained.Artifact.Facts.ArchiveDigest, input.RunID, destination)
			workerOK(t, e)
			if facts != *state.SourceCheckpoint {
				t.Fatal("raw recovery bytes/descriptor mismatch")
			}
			data, e := os.ReadFile(filepath.Join(destination, "source", "hello.txt"))
			workerOK(t, e)
			if string(data) != "completed model source preserved\n" {
				t.Fatal("completed source not recovered")
			}
			if mode == "finalize" {
				data, e = os.ReadFile(filepath.Join(destination, "source", "private-output"))
				workerOK(t, e)
				if string(data) != "TEST-ONLY-PRIVATE\x00BYTES" {
					t.Fatal("ignored raw bytes missing")
				}
			}
			// Read back the actual private Worker records and the actual mTLS Core.
			// This observes facts only, even when Core committed before reply loss.
			permits, e := filepath.Glob(filepath.Join(preparedRoot, "workspaces", "*", "codex-"+state.Token.ID+".json"))
			workerOK(t, e)
			if len(permits) != 1 {
				t.Fatal("missing unique execution permit", permits)
			}
			permitBytes, e := os.ReadFile(permits[0])
			workerOK(t, e)
			readbackRequest := workeragent.PostTurnReadbackRequest{Records: filepath.Dir(permits[0]), RunID: input.RunID, ExecutionID: state.Token.ID, PermitDigest: canonical.BytesDigest(permitBytes), Archive: retained.Artifact.Path}
			if mode != "finalize" {
				readbackRequest.Bundle = filepath.Join(preparedRoot, "artifacts", state.Token.ID+".bundle")
			}
			local, e := workeragent.InspectPostTurn(ctx, readbackRequest)
			workerOK(t, e)
			if local.CoreObservation != "NOT_OBSERVED" || local.LocalObservation != "FAILED_UNCONFIRMED" || local.SourceBytes != "BYTES_VERIFIED" {
				t.Fatal("local readback invented Core authority", local)
			}
			observed, e := workeragent.ObservePostTurnCore(ctx, engineer, local)
			workerOK(t, e)
			if observed.CoreObservation != state.State || observed.CoreCheckpoint != "MATCHES_LOCAL_DESCRIPTOR" || observed.LocalObservation != local.LocalObservation || observed.ExecutionAuthorized || observed.ReplayAuthorized || observed.ProductionQualified || observed.LocalRegistrationClaim != (mode != "checkpoint-lost") {
				t.Fatal("readback conflated local error with Core state", observed)
			}

			// The original interruption-only continuation policy must still reject a
			// completed turn, regardless of whether its local packaging/report failed.
			current, _, e := s.GetExecution(input.RunID)
			workerOK(t, e)
			work, e = s.GetWork(work.ID)
			workerOK(t, e)
			cd, e := facts.Digest()
			workerOK(t, e)
			q := codexexec.ContinueRequest{SourceRunID: input.RunID, RunID: "forbidden-next", AttemptID: "next", CheckpointDigest: cd, ExpectedRunVersion: current.Version, ExpectedWorkVersion: work.Version, ExecutionEpoch: current.CurrentEpoch, RecoveryEpoch: state.Token.RecoveryEpoch, Reason: "TEST must reject completed-turn continuation"}
			if e = engineer.Call(ctx, http.MethodPost, "/api/v1/runs/"+input.RunID+"/continue", q, nil); e == nil {
				t.Fatal("preservation became replay permission")
			}
			if _, _, e = s.GetExecution(q.RunID); e == nil {
				t.Fatal("unauthorized successor created")
			}
			same, e := s.GetCodex(ctx, input.RunID)
			workerOK(t, e)
			a, _ := json.Marshal(state)
			b, _ := json.Marshal(same)
			if string(a) != string(b) {
				t.Fatal("failed continuation changed original proof")
			}
		})
	}
}
