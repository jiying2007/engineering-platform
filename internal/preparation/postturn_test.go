package preparation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

type completedTransport struct {
	mu                        sync.Mutex
	permit                    codexexec.Permit
	work, root, mode          string
	runtimeBinary, runtimeDir string
	qualificationRaw          []byte
	transcript                codexexec.ControlTranscript
	checkpoint                codexexec.SourceCheckpoint
	calls                     map[string]int
}

func (*completedTransport) Subject() string { return subject }
func (c *completedTransport) Call(_ context.Context, _, route string, in, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[route]++
	assign := func(v any) error {
		raw, e := json.Marshal(v)
		if e != nil {
			return e
		}
		return json.Unmarshal(raw, out)
	}
	switch route {
	case "/api/v1/worker/codex/start":
		return assign(c.permit)
	case "/api/v1/worker/codex/renew":
		if c.mode == "final-renew" {
			return fmt.Errorf("TEST-only unavailable final renew")
		}
		return assign(map[string]any{"lease_until": time.Now().Add(time.Minute)})
	case "/api/v1/worker/codex/controls/bind":
		return assign(map[string]bool{"bound": true})
	case "/api/v1/worker/codex/controls/claim":
		return assign(map[string]any{"delivery": nil})
	case "/api/v1/worker/codex/controls/close":
		close := in.(codexexec.ControlClose)
		if close.TurnStatus != "completed" || !close.ProcessScope.Quiescent() {
			return fmt.Errorf("completed quiescent fixture required")
		}
		c.transcript = codexexec.ControlTranscript{Version: 2, Close: close}
		if _, err := c.transcript.Digest(); err != nil {
			return err
		}
		switch c.mode {
		case "ignored":
			if err := os.WriteFile(filepath.Join(c.work, ".gitignore"), []byte("private-output\n"), 0600); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(c.work, "private-output"), []byte("TEST-ONLY-PRIVATE\x00BYTES"), 0600); err != nil {
				return err
			}
		case "unsafe-git":
			if err := os.WriteFile(filepath.Join(c.work, ".git", "config"), []byte("[core]\nrepositoryformatversion = 0\n"), 0600); err != nil {
				return err
			}
		case "archive-exists":
			if err := os.WriteFile(filepath.Join(c.root, "artifacts", c.permit.Token.ID+".source-checkpoint.tar"), []byte("TEST pre-existing do not replace"), 0600); err != nil {
				return err
			}
		}
		var record string
		switch c.mode {
		case "turn-save":
			record = c.permit.Token.ID + ":turn"
		case "phase-save":
			record = c.permit.Token.ID + ":post-turn:FINALIZE"
		case "result-save":
			record = c.permit.Token.ID + ":result"
		case "terminal-journal":
			record = c.permit.Token.ID + ":post-turn-failure"
		}
		if record != "" {
			if err := os.WriteFile(filepath.Join(filepath.Dir(c.work), "codex-"+sandbox.Hash([]byte(record))[7:]+".json"), []byte("TEST pre-existing do not replace"), 0600); err != nil {
				return err
			}
		}
		if c.mode == "bundle-exists" {
			if err := os.WriteFile(filepath.Join(c.root, "artifacts", c.permit.Token.ID+".bundle"), []byte("TEST pre-existing do not replace"), 0600); err != nil {
				return err
			}
		}
		return assign(c.transcript)
	case "/api/v1/worker/codex/report":
		if c.mode != "success" {
			return fmt.Errorf("TEST lost report reply")
		}
		report := in.(codexexec.Report)
		d, e := canonical.Digest(report.Result)
		if e != nil {
			return e
		}
		return assign(codexexec.Receipt{Kind: codexexec.Kind, Token: c.permit.Token, Worker: subject, PreparationDigest: c.permit.Preparation.FactsDigest, Result: report.Result, ResultDigest: d, ReceivedAt: time.Now().UTC()})
	case "/api/v1/worker/codex/source-checkpoint":
		c.checkpoint = in.(codexexec.SourceCheckpoint)
		if c.mode == "checkpoint-lost" {
			return fmt.Errorf("TEST lost checkpoint reply")
		}
		return assign(c.checkpoint)
	case "/api/v1/worker/codex/fail":
		return nil
	default:
		return fmt.Errorf("unexpected path %s", route)
	}
}
func completedSetup(t *testing.T, mode string) (*preparation.Preparer, preparation.Result, *completedTransport, codexexec.Start, workeragent.CodexRuntime) {
	t.Helper()
	a, config := fixture(t)
	python, err := exec.LookPath("python3")
	mustCheckpoint(t, err)
	base := t.TempDir()
	mustCheckpoint(t, os.Chmod(base, 0700))
	binary := filepath.Join(base, "fixture-codex")
	program := []byte("#!" + python + " -S\n" + testsupport.CompletedCodexProtocol)
	mustCheckpoint(t, os.WriteFile(binary, program, 0700))
	login := filepath.Join(base, "auth.json")
	mustCheckpoint(t, os.WriteFile(login, []byte(`{"fixture_only":"not-a-real-credential"}`), 0600))
	qualification := codexapp.QualificationReceipt{
		SchemaVersion: codexapp.QualificationSchemaVersion, CompatibilityContractVersion: codexapp.CompatibilityContractVersion,
		CLI: "codex-cli", Version: "0.157.1", BinaryDigest: canonical.BytesDigest(program),
		StableSchemaDigest: "sha256:" + strings.Repeat("a", 64), ExperimentalSchemaDigest: "sha256:" + strings.Repeat("b", 64),
		Transport: "stdio", FreshProcess: true, InitializePassed: true, ThreadStartPassed: true, ThreadStartModel: "fixture-only",
		StableSchemaContractChecked: true, ExperimentalSurfaceChecked: true,
		CredentialSafeConfigDigest: codexapp.CredentialSafeConfigDigest(), CredentialSafeProfileChecked: true,
		EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), EngineeringProfileChecked: true,
		IsolationMechanism: processscope.Mechanism, IsolationEnvironmentDigest: "sha256:" + strings.Repeat("c", 64),
		IsolatedEngineeringStartup: true, NamespaceInitReaped: true,
	}
	qualificationDigest, err := qualification.Digest()
	mustCheckpoint(t, err)
	qualificationRaw, err := codexapp.MarshalQualification(qualification)
	mustCheckpoint(t, err)
	qualificationPath := filepath.Join(base, "qualification.json")
	mustCheckpoint(t, os.WriteFile(qualificationPath, qualificationRaw, 0600))
	profile := codexexec.Profile{Version: 3, Provider: provideridentity.OpenAIChatGPTTrustedSelfHosted(), CodexVersion: "0.157.1", BinaryDigest: canonical.BytesDigest(program), QualificationDigest: qualificationDigest, EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), Model: "fixture-only", Sandbox: "workspace-write", ApprovalPolicy: "never"}
	pd, err := profile.Digest()
	mustCheckpoint(t, err)
	a.Task.TaskType = "FEATURE"
	a.Task.AllowedActions = []string{codexexec.Action}
	a.Intent.TaskDigest, err = a.Task.Digest()
	mustCheckpoint(t, err)
	a.Input.TaskContractDigest = a.Intent.TaskDigest
	a.Input.ToolProfile = "codex/" + pd
	a.Intent.InputDigest, err = a.Input.Digest()
	mustCheckpoint(t, err)
	a.IntentDigest, err = a.Intent.Digest()
	mustCheckpoint(t, err)
	config.Approvals[0].TaskDigest = a.Intent.TaskDigest
	config.Approvals[0].InputDigest = a.Intent.InputDigest
	p, err := preparation.New(config)
	mustCheckpoint(t, err)
	t.Cleanup(func() { _ = p.Close() })
	prepared, err := p.Prepare(context.Background(), subject, a)
	mustCheckpoint(t, err)
	validation, err := workerqueue.Validate(a)
	mustCheckpoint(t, err)
	fd, err := canonical.Digest(prepared.Facts)
	mustCheckpoint(t, err)
	prepReceipt := preparation.Receipt{Kind: preparation.Kind, Facts: prepared.Facts, FactsDigest: fd, ReceivedAt: time.Now().UTC(), Admission: workerqueue.Receipt{Token: a.Token, Worker: subject, Kind: workerqueue.Validated, Validation: validation, ReceivedAt: time.Now().UTC()}}
	permit := codexexec.Permit{Token: codexexec.Token{ID: strings.Repeat("e", 64), RunID: a.Intent.RunID, WorkerProfile: a.Token.Profile, ProfileDigest: pd}, Assignment: a, Preparation: prepReceipt, Profile: profile, LeaseUntil: time.Now().Add(time.Minute)}
	request := codexexec.Start{RunID: a.Intent.RunID, WorkerProfile: a.Token.Profile, Profile: profile}
	mustCheckpoint(t, permit.Check(subject, request))
	c := &completedTransport{permit: permit, work: prepared.Workspace.WorktreePath, root: config.Root, mode: mode, calls: map[string]int{}, runtimeBinary: binary, runtimeDir: base, qualificationRaw: qualificationRaw}
	return p, prepared, c, request, workeragent.CodexRuntime{Executable: binary, SavedLoginFile: login}
}
func TestActualWorkerPostTurnFailuresRetainSourceWithoutReplay(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	modes := map[string]string{"ignored": "FINALIZE", "turn-save": "TURN_RECEIPT", "phase-save": "FINALIZE", "result-save": "RESULT_PERSIST", "bundle-exists": "FINALIZE", "final-renew": "REPORT_RENEW", "report-lost": "RESULT_REPORT", "checkpoint-lost": "RESULT_REPORT", "unsafe-git": "FINALIZE", "archive-exists": "RESULT_REPORT", "terminal-journal": "RESULT_REPORT"}
	for mode, phase := range modes {
		t.Run(mode, func(t *testing.T) {
			p, prepared, c, request, runtime := completedSetup(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			receipt, err := workeragent.ExecuteCodex(ctx, c, p, request, runtime)
			var failure *workeragent.PostTurnFailureError
			if !errors.As(err, &failure) || failure.Phase != phase || failure.JournalSaved != (mode != "terminal-journal") || receipt.Kind != "" {
				t.Fatalf("missing bounded failure: %v %+v", err, failure)
			}
			if c.calls["/api/v1/worker/codex/start"] != 1 || c.calls["/api/v1/worker/codex/fail"] != 1 {
				t.Fatal("execution replay", c.calls)
			}
			if c.transcript.Continuable() {
				t.Fatal("completed turn became interrupted continuation")
			}
			journal := filepath.Join(filepath.Dir(c.work), "codex-"+sandbox.Hash([]byte(c.permit.Token.ID + ":post-turn-failure"))[7:]+".json")
			raw, e := os.ReadFile(journal)
			mustCheckpoint(t, e)
			if mode == "terminal-journal" {
				if string(raw) != "TEST pre-existing do not replace" {
					t.Fatal("failure journal overwritten")
				}
				var kept *workeragent.CheckpointRetainedError
				if !errors.As(err, &kept) || !kept.Registered || c.calls["/api/v1/worker/codex/source-checkpoint"] != 1 {
					t.Fatal("journal failure hid retained artifact")
				}
				return
			}
			var value map[string]any
			mustCheckpoint(t, json.Unmarshal(raw, &value))
			if value["phase"] != phase || value["execution_authorized"] != false || value["delivery_confirmed_by_worker"] != false {
				t.Fatal("journal claims authority", value)
			}
			info, e := os.Stat(journal)
			mustCheckpoint(t, e)
			if info.Mode().Perm() != 0600 {
				t.Fatal("nonprivate journal")
			}
			var kept *workeragent.CheckpointRetainedError
			if mode == "unsafe-git" || mode == "archive-exists" {
				if errors.As(err, &kept) || c.calls["/api/v1/worker/codex/source-checkpoint"] != 0 || value["source_checkpoint"] != nil {
					t.Fatal("failed capture forged preservation")
				}
				if mode == "archive-exists" {
					b, e := os.ReadFile(filepath.Join(c.root, "artifacts", c.permit.Token.ID+".source-checkpoint.tar"))
					mustCheckpoint(t, e)
					if string(b) != "TEST pre-existing do not replace" {
						t.Fatal("replaced artifact")
					}
				}
				return
			}
			if !errors.As(err, &kept) || kept.Registered != (mode != "checkpoint-lost") || c.calls["/api/v1/worker/codex/source-checkpoint"] != 1 {
				t.Fatal("checkpoint false registration/replay", err, c.calls)
			}
			parent := t.TempDir()
			mustCheckpoint(t, os.Chmod(parent, 0700))
			destination := filepath.Join(parent, "restore")
			facts, e := sourcecheckpoint.Restore(ctx, kept.Artifact.Path, kept.Artifact.Facts.ArchiveDigest, request.RunID, destination)
			mustCheckpoint(t, e)
			if facts != kept.Artifact.Facts || facts.BaseCommit != prepared.Facts.BaseCommit {
				t.Fatal("source lineage drift")
			}
			data, e := os.ReadFile(filepath.Join(destination, "source", "hello.txt"))
			mustCheckpoint(t, e)
			if string(data) != "completed model source preserved\n" {
				t.Fatal("source lost")
			}
			if mode == "ignored" {
				data, e = os.ReadFile(filepath.Join(destination, "source", "private-output"))
				mustCheckpoint(t, e)
				if string(data) != "TEST-ONLY-PRIVATE\x00BYTES" {
					t.Fatal("ignored private bytes lost")
				}
			}
			if mode == "report-lost" || mode == "checkpoint-lost" {
				if c.calls["/api/v1/worker/codex/report"] != 2 {
					t.Fatal("changed bounded metadata report semantics")
				}
			}
			if mode == "ignored" || mode == "bundle-exists" || mode == "result-save" {
				head, e := os.ReadFile(filepath.Join(c.work, ".git", "HEAD"))
				mustCheckpoint(t, e)
				if string(head) == prepared.Workspace.BaseCommit+"\n" {
					t.Fatal("fixture did not reach post-commit failure")
				}
				if _, _, e = p.CheckpointPaths(ctx, subject, c.permit.Assignment, prepared, c.permit.Preparation.FactsDigest); e == nil {
					t.Fatal("normal base-only path weakened")
				}
			}
		})
	}
}
func TestActualWorkerSuccessDoesNotCreateExtraSourceArchive(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	p, _, c, request, runtime := completedSetup(t, "success")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	receipt, err := workeragent.ExecuteCodex(ctx, c, p, request, runtime)
	mustCheckpoint(t, err)
	if receipt.Kind != codexexec.Kind || c.calls["/api/v1/worker/codex/source-checkpoint"] != 0 || c.calls["/api/v1/worker/codex/fail"] != 0 || c.calls["/api/v1/worker/codex/start"] != 1 || c.calls["/api/v1/worker/codex/report"] != 1 {
		t.Fatal("success regression", c.calls)
	}
	matches, err := filepath.Glob(filepath.Join(c.root, "artifacts", "*.source-checkpoint.tar"))
	mustCheckpoint(t, err)
	if len(matches) != 0 {
		t.Fatal("successful run got unsolicited source copy")
	}
}
