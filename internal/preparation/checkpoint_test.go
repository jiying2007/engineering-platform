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
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func mustCheckpoint(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointPathsRemainBoundToOwnedPreparedWorkspace(t *testing.T) {
	a, config := fixture(t)
	p, err := preparation.New(config)
	mustCheckpoint(t, err)
	defer p.Close()
	result, err := p.Prepare(context.Background(), subject, a)
	mustCheckpoint(t, err)
	digest, err := canonical.Digest(result.Facts)
	mustCheckpoint(t, err)
	mustCheckpoint(t, os.WriteFile(filepath.Join(result.Workspace.WorktreePath, "hello.txt"), []byte("unfinished source\n"), 0600))
	source, output, err := p.CheckpointPaths(context.Background(), subject, a, result, digest)
	mustCheckpoint(t, err)
	if source != result.Workspace.WorktreePath || output != filepath.Join(config.Root, "artifacts") {
		t.Fatal("unowned checkpoint destination")
	}
	if p.Recheck(context.Background(), a, result) == nil {
		t.Fatal("dirty source passed original preparation recheck")
	}
	if _, _, err := p.CheckpointPaths(context.Background(), "other", a, result, digest); err == nil {
		t.Fatal("foreign checkpoint owner")
	}
	for _, change := range []func(*preparation.Result){
		func(r *preparation.Result) { r.Workspace.WorktreePath = config.Approvals[0].RepositoryPath },
		func(r *preparation.Result) { r.Facts.SourceDigest = canonical.BytesDigest([]byte("other")) },
		func(r *preparation.Result) { r.BundlePath = config.ContextSource },
	} {
		bad := result
		change(&bad)
		if _, _, err := p.CheckpointPaths(context.Background(), subject, a, bad, digest); err == nil {
			t.Fatal("altered checkpoint identity accepted")
		}
	}
}

// This transport is an explicit in-memory Core fixture. The actual Preparer,
// ExecuteCodex, runtime namespace, model protocol process, capture and restore
// paths run below; PostgreSQL+mTLS binding has separate mandatory CI coverage.
type stoppedCheckpointTransport struct {
	mu                            sync.Mutex
	permit                        codexexec.Permit
	work                          string
	delivery                      *codexexec.ControlDelivery
	transcript                    codexexec.ControlTranscript
	checkpoint                    codexexec.SourceCheckpoint
	calls                         map[string]int
	rejectReport, changedReadback bool
}

func (*stoppedCheckpointTransport) Subject() string { return subject }
func (c *stoppedCheckpointTransport) Call(_ context.Context, _, route string, in, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[route]++
	assign := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
	switch route {
	case "/api/v1/worker/codex/start":
		return assign(c.permit)
	case "/api/v1/worker/codex/renew":
		return assign(map[string]any{"lease_until": time.Now().Add(time.Minute)})
	case "/api/v1/worker/codex/controls/bind":
		binding := in.(codexexec.ControlBinding)
		payload := codexexec.ControlPayload{Binding: binding, Kind: codexexec.ControlInterrupt}
		hash, err := canonical.Digest(payload)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		c.delivery = &codexexec.ControlDelivery{
			Command: session.SteeringCommand{ID: "test-interrupt", RunID: binding.Token.RunID, ExecutionEpoch: binding.ExecutionEpoch, Sequence: 1, Actor: "urn:engineering-platform:engineer:test", ContentDigest: hash, CreatedAt: now},
			Payload: payload, State: codexexec.ControlDispatching, DispatchedAt: &now,
		}
		// The model fixture receives this source only after the real preparation
		// recheck. A stopped checkpoint must preserve it, not reset to the base.
		if err := os.WriteFile(filepath.Join(c.work, "hello.txt"), []byte("interrupted work preserved\n"), 0600); err != nil {
			return err
		}
		return assign(map[string]bool{"bound": true})
	case "/api/v1/worker/codex/controls/claim":
		if c.calls[route] == 1 {
			return assign(map[string]any{"delivery": c.delivery})
		}
		return assign(map[string]any{"delivery": nil})
	case "/api/v1/worker/codex/controls/report":
		report := in.(codexexec.ControlSettlement)
		c.delivery.State = report.Outcome
		now := time.Now().UTC()
		c.delivery.ResolvedAt = &now
		if err := os.WriteFile(filepath.Join(c.work, "allow-finish"), []byte("test barrier"), 0600); err != nil {
			return err
		}
		return assign(c.delivery)
	case "/api/v1/worker/codex/controls/close":
		close := in.(codexexec.ControlClose)
		if close.TurnStatus != "interrupted" || !close.ProcessScope.Quiescent() {
			return fmt.Errorf("actual interrupted/quiescent runtime required")
		}
		c.delivery.State = codexexec.ControlInterrupted
		c.transcript = codexexec.ControlTranscript{Version: 2, Close: close, Deliveries: []codexexec.ControlDelivery{*c.delivery}}
		if _, err := c.transcript.Digest(); err != nil {
			return err
		}
		return assign(c.transcript)
	case "/api/v1/worker/codex/source-checkpoint":
		c.checkpoint = in.(codexexec.SourceCheckpoint)
		if c.rejectReport {
			return fmt.Errorf("fixture: lost checkpoint registration reply")
		}
		reply := c.checkpoint
		if c.changedReadback {
			reply.ArchiveDigest = canonical.BytesDigest([]byte("foreign"))
		}
		return assign(reply)
	case "/api/v1/worker/codex/fail":
		return nil
	default:
		return fmt.Errorf("unexpected production-path request %s", route)
	}
}

func TestExecuteCodexRetainsInterruptedSourceWithoutReplayingOrFinishing(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, mode := range []string{"registered", "lost-reply", "changed-readback"} {
		t.Run(mode, func(t *testing.T) {
			a, config := fixture(t)
			python, err := exec.LookPath("python3")
			mustCheckpoint(t, err)
			base := t.TempDir()
			mustCheckpoint(t, os.Chmod(base, 0700))
			binary := filepath.Join(base, "fixture-codex")
			program := []byte("#!" + python + " -S\n" + testsupport.ControlledCodexProtocol)
			mustCheckpoint(t, os.WriteFile(binary, program, 0700))
			login := filepath.Join(base, "auth.json")
			mustCheckpoint(t, os.WriteFile(login, []byte(`{"fixture_only":"not-a-real-credential"}`), 0600))
			profile := codexexec.Profile{Version: 3, Provider: provideridentity.OpenAIChatGPTTrustedSelfHosted(), CodexVersion: "0.157.1", BinaryDigest: canonical.BytesDigest(program), QualificationDigest: canonical.BytesDigest([]byte("test-only qualification")), EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), Model: "fixture-only", Sandbox: "workspace-write", ApprovalPolicy: "never"}
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
			defer p.Close()
			prepared, err := p.Prepare(context.Background(), subject, a)
			mustCheckpoint(t, err)
			validation, err := workerqueue.Validate(a)
			mustCheckpoint(t, err)
			fd, err := canonical.Digest(prepared.Facts)
			mustCheckpoint(t, err)
			prepReceipt := preparation.Receipt{Kind: preparation.Kind, Facts: prepared.Facts, FactsDigest: fd, ReceivedAt: time.Now().UTC(), Admission: workerqueue.Receipt{Token: a.Token, Worker: subject, Kind: workerqueue.Validated, Validation: validation, ReceivedAt: time.Now().UTC()}}
			permit := codexexec.Permit{Token: codexexec.Token{ID: strings.Repeat("d", 64), RunID: a.Intent.RunID, WorkerProfile: a.Token.Profile, ProfileDigest: pd}, Assignment: a, Preparation: prepReceipt, Profile: profile, LeaseUntil: time.Now().Add(time.Minute)}
			request := codexexec.Start{RunID: a.Intent.RunID, WorkerProfile: a.Token.Profile, Profile: profile}
			mustCheckpoint(t, permit.Check(subject, request))
			c := &stoppedCheckpointTransport{permit: permit, work: prepared.Workspace.WorktreePath, calls: map[string]int{}, rejectReport: mode == "lost-reply", changedReadback: mode == "changed-readback"}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			receipt, err := workeragent.ExecuteCodex(ctx, c, p, request, workeragent.CodexRuntime{Executable: binary, SavedLoginFile: login})
			var retained *workeragent.CheckpointRetainedError
			if !errors.As(err, &retained) {
				t.Fatalf("actual Worker failed to retain interrupted source: %v", err)
			}
			if receipt.Kind != "" || retained.Registered != (mode == "registered") {
				t.Fatal("checkpoint promoted success or false registration")
			}
			if c.calls["/api/v1/worker/codex/start"] != 1 || c.calls["/api/v1/worker/codex/fail"] != 1 || c.calls["/api/v1/worker/codex/report"] != 0 || c.calls["/api/v1/worker/codex/source-checkpoint"] != 1 {
				t.Fatal("model/report replay or successful delivery", c.calls)
			}
			if c.checkpoint != retained.Artifact.Facts {
				t.Fatal("Core checkpoint differs from retained bytes")
			}
			facts, err := sourcecheckpoint.Restore(ctx, retained.Artifact.Path, retained.Artifact.Facts.ArchiveDigest, a.Intent.RunID, filepath.Join(base, "recovery-copy"))
			mustCheckpoint(t, err)
			if facts != retained.Artifact.Facts {
				t.Fatal("restored checkpoint drift")
			}
			source := filepath.Join(base, "recovery-copy", "source")
			raw, err := os.ReadFile(filepath.Join(source, "hello.txt"))
			mustCheckpoint(t, err)
			if string(raw) != "interrupted work preserved\n" {
				t.Fatal("source bytes lost")
			}
			if _, err := os.Lstat(filepath.Join(source, ".git")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("Git metadata included")
			}
			if _, err := os.Lstat(filepath.Join(source, "auth.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("runtime login included")
			}
			requests, err := os.ReadFile(filepath.Join(source, "requests.jsonl"))
			mustCheckpoint(t, err)
			if strings.Count(string(requests), "turn/interrupt") != 1 {
				t.Fatal("interrupt replay")
			}
		})
	}
}
