package githubpublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

type publisherState struct {
	run    run.Run
	task   core.TaskContract
	status codexexec.Status
}

func (s publisherState) GetExecution(id string) (run.Run, session.Session, error) {
	if id != s.run.ID {
		return run.Run{}, session.Session{}, os.ErrNotExist
	}
	return s.run, *session.New(id, s.run.CurrentEpoch), nil
}

func (s publisherState) GetTaskByDigest(string) (core.TaskContract, error) { return s.task, nil }

func (s publisherState) GetCodex(context.Context, string) (codexexec.Status, error) {
	return s.status, nil
}

type publisherRemote struct {
	publishCalls int
	observeCalls int
	plan         Plan
	receipt      PublicationReceipt
	observation  ObserveResult
	err          error
}

func (r *publisherRemote) Publish(_ context.Context, plan Plan, _ string) (PublicationReceipt, error) {
	r.publishCalls++
	r.plan = plan
	if r.err != nil {
		return PublicationReceipt{}, r.err
	}
	receipt := r.receipt
	if receipt.Version == 0 {
		receipt = PublicationReceipt{
			Version: 1, Repository: plan.Repository, BaseRef: plan.BaseRef, BaseCommit: plan.BaseCommit,
			Branch: plan.Branch, ResultCommit: plan.ResultCommit, PullRequestNumber: 17,
			PullRequestURL: "https://github.com/" + plan.Repository + "/pull/17",
			PullRequestState: "open", PublicationOutcome: "CREATED",
		}
	}
	return receipt, nil
}

func (r *publisherRemote) Observe(_ context.Context, plan Plan) (ObserveResult, error) {
	r.observeCalls++
	r.plan = plan
	if r.err != nil {
		return ObserveResult{}, r.err
	}
	result := r.observation
	if result.Outcome == ObservedConfirmed && result.Receipt.Version == 0 {
		result.Receipt = PublicationReceipt{
			Version: 1, Repository: plan.Repository, BaseRef: plan.BaseRef, BaseCommit: plan.BaseCommit,
			Branch: plan.Branch, ResultCommit: plan.ResultCommit, PullRequestNumber: 17,
			PullRequestURL: "https://github.com/" + plan.Repository + "/pull/17",
			PullRequestState: "open", PublicationOutcome: "OBSERVED",
		}
	}
	return result, nil
}

func TestProviderPublishesOnlyExactRetainedCodexResult(t *testing.T) {
	state, config := publisherFixture(t)
	remote := &publisherRemote{}
	provider, err := New(config, state, remote)
	if err != nil {
		t.Fatal(err)
	}
	request := action.Request{
		ID: "publish-1", RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
		Action: Action, RiskClass: action.ControlledMutation, Capability: Capability,
		ParametersDigest: state.status.Receipt.ResultDigest,
	}
	result, err := provider.Dispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != action.DispatchConfirmed || remote.publishCalls != 1 ||
		remote.plan.ResultCommit != state.status.Receipt.Result.Change.ResultCommit ||
		remote.plan.BundleDigest != state.status.Receipt.Result.Change.BundleDigest ||
		remote.plan.Branch != "engineering-platform/"+state.status.Token.ID[:24] {
		t.Fatalf("unexpected publication: %#v plan=%#v", result, remote.plan)
	}
	var receipt PublicationReceipt
	if json.Unmarshal([]byte(result.ObservedState), &receipt) != nil ||
		receipt.PullRequestURL != result.ExternalRef || receipt.Validate(remote.plan) != nil {
		t.Fatalf("invalid retained publication receipt: %s", result.ObservedState)
	}

	bad := request
	bad.ParametersDigest = digestOf("different-result")
	badResult, err := provider.Dispatch(context.Background(), bad)
	if err != nil || badResult.Outcome != action.DispatchUnknown || badResult.ObservedState != "PRECONDITION_FAILED" {
		t.Fatalf("mismatched result was not retained as precondition failure: %#v err=%v", badResult, err)
	}
	if remote.publishCalls != 1 {
		t.Fatalf("mismatched result reached GitHub remote: %d", remote.publishCalls)
	}
}

func TestProviderReconciliationNeverReplaysPublication(t *testing.T) {
	state, config := publisherFixture(t)
	for _, tc := range []struct {
		name string
		seen Observation
		want action.ReconcileOutcome
	}{
		{"confirmed", ObservedConfirmed, action.ReconcileConfirmed},
		{"absent", ObservedAbsent, action.ReconcileSafeToRetry},
		{"partial", ObservedPartial, action.ReconcileManual},
		{"conflict", ObservedConflict, action.ReconcileManual},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := &publisherRemote{observation: ObserveResult{Outcome: tc.seen}}
			provider, err := New(config, state, remote)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Reconcile(context.Background(), action.Operation{
				ID: "publish-unknown", RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
				Action: Action, RiskClass: action.ControlledMutation, Capability: Capability,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != tc.want || remote.observeCalls != 1 || remote.publishCalls != 0 {
				t.Fatalf("reconcile=%#v observe=%d publish=%d", result, remote.observeCalls, remote.publishCalls)
			}
		})
	}
}

func TestProviderRejectsChangedOrUnsafeBundle(t *testing.T) {
	state, config := publisherFixture(t)
	remote := &publisherRemote{}
	provider, err := New(config, state, remote)
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(config.ArtifactRoot, state.status.Token.ID+".bundle")
	if err := os.WriteFile(bundle, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Dispatch(context.Background(), action.Request{
		RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch, Action: Action,
		RiskClass: action.ControlledMutation, Capability: Capability,
		ParametersDigest: state.status.Receipt.ResultDigest,
	})
	if err != nil || result.Outcome != action.DispatchUnknown || result.ObservedState != "PRECONDITION_FAILED" {
		t.Fatalf("changed retained bundle was not rejected deterministically: %#v err=%v", result, err)
	}
	if remote.publishCalls != 0 {
		t.Fatal("unsafe bundle reached GitHub remote")
	}
}

func publisherFixture(t *testing.T) (publisherState, Configuration) {
	t.Helper()
	root := t.TempDir()
	token := filepath.Join(root, "github-token")
	if err := os.WriteFile(token, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	artifactRoot := filepath.Join(root, "artifacts")
	if err := os.Mkdir(artifactRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	execution := strings.Repeat("a", 64)
	bundleBytes := []byte("retained-git-bundle-fixture")
	bundleHash := sha256.Sum256(bundleBytes)
	bundleDigest := "sha256:" + hex.EncodeToString(bundleHash[:])
	if err := os.WriteFile(filepath.Join(artifactRoot, execution+".bundle"), bundleBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	base := strings.Repeat("1", 40)
	resultCommit := strings.Repeat("4", 40)
	tokenValue := codexexec.Token{
		ID: execution, RunID: "run-publish", WorkerProfile: "worker/ubuntu",
		ProfileDigest: digestOf("profile"), RecoveryEpoch: 0,
	}
	codexReceipt := codexapp.EngineeringReceipt{
		SchemaVersion: 1, CLI: "codex-cli", Version: codexapp.QualifiedCodexVersion,
		BinaryDigest: digestOf("binary"), EngineeringConfigDigest: codexapp.EngineeringConfigDigest(),
		CredentialMode: "workload_identity", FederationRuleID: "rule-1", Model: "gpt-test",
		PromptDigest: digestOf("prompt"), ThreadID: "thread-1", TurnID: "turn-1",
		TurnStatus: "completed", Output: "done", OutputDigest: canonical.BytesDigest([]byte("done")),
		CommandCount: 1, FileChangeCount: 1, ApprovalRequests: 0, AssertionRemovedBeforeTurn: true,
	}
	result := codexexec.Result{
		PromptIdentityDigest: digestOf("prompt-identity"),
		Codex:                codexReceipt,
		Change: workspace.ChangeFacts{
			Recipe: workspace.FinalizeRecipe, BaseCommit: base, BaseTree: strings.Repeat("2", 40),
			BaseSourceDigest: digestOf("base-source"), ResultCommit: resultCommit,
			ResultTree: strings.Repeat("5", 40), ResultSourceDigest: digestOf("result-source"),
			BundleDigest: bundleDigest, BundleSize: int64(len(bundleBytes)),
		},
	}
	resultDigest, err := canonical.Digest(result)
	if err != nil {
		t.Fatal(err)
	}
	receipt := codexexec.Receipt{
		Kind: codexexec.Kind, Token: tokenValue, Worker: "urn:engineering-platform:worker:test",
		PreparationDigest: digestOf("preparation"), Result: result, ResultDigest: resultDigest,
		ReceivedAt: time.Now().UTC(),
	}
	state := publisherState{
		run: run.Run{
			ID: "run-publish", TaskContractDigest: digestOf("task"), RunInputManifestDigest: digestOf("input"),
			State: run.Running, Version: 1, CurrentEpoch: 1, CurrentAttemptID: "attempt-1", ControlOwner: "RUNTIME",
		},
		task: core.TaskContract{
			ID: "task-publish", WorkItemID: "work-publish", TaskType: "FEATURE",
			Repository: "jiying2007/engineering-platform", BaseCommit: base,
			AllowedActions: []string{Action}, Revision: 1,
		},
		status: codexexec.Status{Token: tokenValue, State: codexexec.Finished, Receipt: &receipt},
	}
	config := Configuration{
		Version: 1, ArtifactRoot: artifactRoot, GitExecutable: git, TokenFile: token,
		Targets: []TargetPolicy{{
			Repository: "jiying2007/engineering-platform", BaseRef: "main",
			BranchPrefix: "engineering-platform/",
		}},
	}
	return state, config
}

func digestOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
