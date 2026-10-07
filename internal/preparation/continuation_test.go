package preparation_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// This local codec/preparation fixture uses synthetic authority. The separate
// PostgreSQL+mTLS test exercises two actual Worker executions and real stop proof.
func sourceContinuationFixture(t *testing.T) (workerqueue.Assignment, preparation.Configuration, sourcecheckpoint.Artifact) {
	t.Helper()
	a, c := fixture(t)
	profile := codexexec.Profile{Version: 3, Provider: provideridentity.OpenAIChatGPTTrustedSelfHosted(), CodexVersion: "0.157.1", BinaryDigest: canonical.BytesDigest([]byte("fixture")), QualificationDigest: canonical.BytesDigest([]byte("test-only")), EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), Model: "fixture", Sandbox: "workspace-write", ApprovalPolicy: "never"}
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
	c.Approvals[0].TaskDigest = a.Intent.TaskDigest
	c.Approvals[0].InputDigest = a.Intent.InputDigest
	p, err := preparation.New(c)
	mustCheckpoint(t, err)
	defer p.Close()
	prepared, err := p.Prepare(context.Background(), subject, a)
	mustCheckpoint(t, err)
	fd, err := canonical.Digest(prepared.Facts)
	mustCheckpoint(t, err)
	v, err := workerqueue.Validate(a)
	mustCheckpoint(t, err)
	permit := codexexec.Permit{Token: codexexec.Token{ID: strings.Repeat("d", 64), RunID: a.Intent.RunID, WorkerProfile: a.Token.Profile, ProfileDigest: pd}, Assignment: a, Profile: profile, LeaseUntil: time.Now().Add(time.Minute), Preparation: preparation.Receipt{Kind: preparation.Kind, Facts: prepared.Facts, FactsDigest: fd, ReceivedAt: time.Now().UTC(), Admission: workerqueue.Receipt{Token: a.Token, Worker: subject, Kind: workerqueue.Validated, Validation: v, ReceivedAt: time.Now().UTC()}}}
	mustCheckpoint(t, os.WriteFile(filepath.Join(prepared.Workspace.WorktreePath, "hello.txt"), []byte("inherited unfinished\n"), 0600))
	mustCheckpoint(t, os.WriteFile(filepath.Join(prepared.Workspace.WorktreePath, "new.txt"), []byte("未完成\n"), 0600))
	tr := codexexec.ControlTranscript{Version: 2, Close: codexexec.ControlClose{Binding: codexexec.ControlBinding{Token: permit.Token, ExecutionEpoch: 1, ThreadID: "thread", TurnID: "turn"}, TurnStatus: "interrupted", ProcessScope: testsupport.ProcessScopeFixture()}}
	retained, err := p.CheckpointGitBundle(context.Background(), subject, a, prepared, fd, permit.Token.ID, false)
	mustCheckpoint(t, err)
	artifact, err := sourcecheckpoint.Capture(context.Background(), prepared.Workspace.WorktreePath, filepath.Join(c.Root, "artifacts"), permit, tr, sourcecheckpoint.GitBundle{Path: retained.Path, Digest: retained.Digest, Size: retained.Size, Head: retained.Head})
	mustCheckpoint(t, err)
	mustCheckpoint(t, os.Remove(retained.Path))
	ref, err := artifact.Facts.ContinuationRef()
	mustCheckpoint(t, err)
	a.Token.InboxID = 2
	a.Intent.RunID = "next"
	a.Input.RunID = "next"
	a.Input.Continuation = &ref
	a.Intent.InputDigest, err = a.Input.Digest()
	mustCheckpoint(t, err)
	a.IntentDigest, err = a.Intent.Digest()
	mustCheckpoint(t, err)
	c.Approvals[0].RunID = "next"
	c.Approvals[0].InputDigest = a.Intent.InputDigest
	c.Approvals[0].ContinuationArchive = artifact.Path
	return a, c, artifact
}

func TestContinuationPreparesSeedWithoutCommittingOrDroppingOriginalDiff(t *testing.T) {
	a, c, artifact := sourceContinuationFixture(t)
	ctx := context.Background()
	p, err := preparation.New(c)
	mustCheckpoint(t, err)
	defer p.Close()
	r, err := p.PrepareWithSource(ctx, subject, a, workeragent.RestoreContinuationSource)
	mustCheckpoint(t, err)
	mustCheckpoint(t, p.Recheck(ctx, a, r))
	if r.Facts.SeedCheckpointDigest != a.Input.Continuation.CheckpointDigest || r.Facts.SeedSourceDigest == "" || r.Facts.SeedSourceDigest == r.Facts.SourceDigest || r.Workspace.BaseCommit != a.Task.BaseCommit {
		t.Fatal("seed/base identities drift")
	}
	digest, err := canonical.Digest(r.Facts)
	mustCheckpoint(t, err)
	reopened, err := p.Reopen(ctx, subject, a, digest)
	mustCheckpoint(t, err)
	if reopened.Workspace != r.Workspace {
		t.Fatal("reopen lost seed")
	}
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(c.Git, append([]string{"-C", r.Workspace.WorktreePath}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if git("rev-parse", "HEAD") != a.Task.BaseCommit || !strings.Contains(git("diff", "--", "hello.txt"), "inherited unfinished") {
		t.Fatal("seed was silently committed or dropped")
	}
	mustCheckpoint(t, os.WriteFile(filepath.Join(r.Workspace.WorktreePath, "hello.txt"), []byte("inherited unfinished\nnew iteration\n"), 0600))
	if p.Recheck(ctx, a, r) == nil {
		t.Fatal("changed source passes pristine seed recheck")
	}
	result, err := p.FinalizeChangedWorkspace(ctx, subject, a, r, digest, strings.Repeat("f", 64))
	mustCheckpoint(t, err)
	if result.Facts.BaseCommit != a.Task.BaseCommit || !strings.Contains(git("diff", a.Task.BaseCommit, result.Facts.ResultCommit, "--", "hello.txt"), "inherited unfinished") || git("show", result.Facts.ResultCommit+":new.txt") != "未完成" {
		t.Fatal("original base diff was laundered")
	}
	_, err = sourcecheckpoint.Verify(ctx, artifact.Path, artifact.Facts.ArchiveDigest, artifact.Facts.Binding.Token.RunID)
	mustCheckpoint(t, err)
}
func TestContinuationPreparationRequiresNewExactApprovalAndBytes(t *testing.T) {
	for _, mode := range []string{"missing-archive", "missing-verifier", "old-approval", "wrong-digest", "foreign-descriptor", "changed-profile", "tamper", "wrong-task"} {
		t.Run(mode, func(t *testing.T) {
			a, c, artifact := sourceContinuationFixture(t)
			restore := preparation.SourceRestorer(workeragent.RestoreContinuationSource)
			switch mode {
			case "missing-archive":
				c.Approvals[0].ContinuationArchive = ""
			case "missing-verifier":
				restore = nil
			case "old-approval":
				c.Approvals[0].RunID = artifact.Facts.Binding.Token.RunID
			case "wrong-digest":
				a.Input.Continuation.ArchiveDigest = canonical.BytesDigest([]byte("wrong"))
			case "foreign-descriptor":
				a.Input.Continuation.CheckpointDigest = canonical.BytesDigest([]byte("wrong"))
			case "changed-profile":
				a.Input.ToolProfile = "codex/" + canonical.BytesDigest([]byte("changed"))
			case "wrong-task":
				a.Task.AcceptanceCriteria = []string{"different task"}
				a.Intent.TaskDigest, _ = a.Task.Digest()
				a.Input.TaskContractDigest = a.Intent.TaskDigest
				c.Approvals[0].TaskDigest = a.Intent.TaskDigest
			case "tamper":
				mustCheckpoint(t, os.Chmod(artifact.Path, 0600))
				mustCheckpoint(t, os.WriteFile(artifact.Path, []byte("changed archive"), 0600))
			}
			a.Intent.InputDigest, _ = a.Input.Digest()
			a.IntentDigest, _ = a.Intent.Digest()
			c.Approvals[0].InputDigest = a.Intent.InputDigest
			p, err := preparation.New(c)
			mustCheckpoint(t, err)
			defer p.Close()
			if _, err = p.PrepareWithSource(context.Background(), subject, a, restore); err == nil {
				t.Fatal("unsafe continuation accepted")
			}
			// The original retained workspace is untouched, and failed new preparation
			// must never expose a prepared.json. No cleanup touches the original archive.
			entries, err := os.ReadDir(filepath.Join(c.Root, "workspaces"))
			mustCheckpoint(t, err)
			if len(entries) != 1 {
				t.Fatal("failed seed leaked new workspace", len(entries))
			}
		})
	}
}
