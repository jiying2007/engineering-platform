package preparation_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

const subject = "urn:engineering-platform:worker:prepare-test"

func fixture(t *testing.T) (workerqueue.Assignment, preparation.Configuration) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	source := filepath.Join(base, "objects")
	root := filepath.Join(base, "prepared")
	for _, dir := range []string{repo, source, root} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, append([]string{"-C", repo}, args...)...)
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture Git: %v %s", err, data)
		}
		return strings.TrimSpace(string(data))
	}
	runGit("init")
	runGit("config", "user.name", "Preparation Test")
	runGit("config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "hello.txt")
	runGit("commit", "-m", "base")
	commit := runGit("rev-parse", "HEAD")
	task := core.TaskContract{ID: "task", WorkItemID: "work", Repository: "logical-repo", BaseCommit: commit, AcceptanceCriteria: []string{"test"}, Revision: 1}
	td, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	bytes := []byte("approved context\n")
	digest := canonical.BytesDigest(bytes)
	ref := core.ContextRef{Source: "docs:spec", Type: "DOCUMENT", Version: "v1", Digest: digest, Trust: core.ContextApproved}
	if err := os.WriteFile(filepath.Join(source, strings.TrimPrefix(digest, "sha256:")+".bin"), bytes, 0o400); err != nil {
		t.Fatal(err)
	}
	input := core.RunInputManifest{RunID: "run", TaskContractDigest: td, ContextRefs: []core.ContextRef{ref}, RuntimeProfile: "codex", WorkerProfile: "worker/prepare", ToolProfile: "read", PolicyProfile: "policy"}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	intent := workerqueue.Intent{RunID: input.RunID, TaskDigest: td, InputDigest: inputDigest, ExecutionEpoch: 1}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	a := workerqueue.Assignment{Token: workerqueue.Token{InboxID: 1, Generation: 1, Profile: input.WorkerProfile}, Task: task, Input: input, Intent: intent, IntentDigest: intentDigest, LeaseUntil: time.Now().Add(time.Minute)}
	c := preparation.Configuration{Version: 1, Worker: subject, Root: root, Git: git, ContextSource: source, Approvals: []preparation.Approval{{RunID: input.RunID, TaskDigest: td, InputDigest: inputDigest, Repository: task.Repository, RepositoryPath: repo, Refs: []core.ContextRef{ref}}}}
	return a, c
}
func TestPreparedActualBytesIndependentWorkspaceAndRecheck(t *testing.T) {
	a, c := fixture(t)
	p, err := preparation.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	result, err := p.Prepare(context.Background(), subject, a)
	if err != nil {
		t.Fatal(err)
	}
	if result.Facts.ExecutionStarted || result.Facts.OSIsolated || result.Facts.Context.Entries[0].Size != 17 {
		t.Fatalf("unexpected facts: %#v", result.Facts)
	}
	if err := result.Facts.Check(a); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result.Facts)
	if strings.Contains(string(raw), c.Root) || strings.Contains(string(raw), c.ContextSource) || strings.Contains(string(raw), c.Approvals[0].RepositoryPath) {
		t.Fatal("machine path contaminated retained identity")
	}
	if err := p.Recheck(context.Background(), a, result); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Workspace.WorktreePath, "hello.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p.Recheck(context.Background(), a, result) == nil {
		t.Fatal("changed source was accepted")
	}
	if err := p.Cleanup(context.Background(), result); err != nil {
		t.Fatal(err)
	}
}
func TestPreparationRejectsUnapprovedInputBeforeGit(t *testing.T) {
	for _, bad := range []string{"subject", "input", "repository"} {
		t.Run(bad, func(t *testing.T) {
			a, c := fixture(t)
			switch bad {
			case "input":
				c.Approvals[0].InputDigest = canonical.BytesDigest([]byte("other"))
			case "repository":
				c.Approvals[0].Repository = "other"
			}
			p, err := preparation.New(c)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			actor := subject
			if bad == "subject" {
				actor = "urn:engineering-platform:other"
			}
			if _, err := p.Prepare(context.Background(), actor, a); !errors.Is(err, contextbundle.ErrDenied) {
				t.Fatalf("approval bypass: %v", err)
			}
			entries, err := os.ReadDir(filepath.Join(c.Root, "workspaces"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("denial touched source slots: %v", err)
			}
		})
	}
}
func TestPreparationRejectsSourceDigestMismatchAndCleansOwnedSlot(t *testing.T) {
	a, c := fixture(t)
	file := filepath.Join(c.ContextSource, strings.TrimPrefix(a.Input.ContextRefs[0].Digest, "sha256:")+".bin")
	if err := os.Chmod(file, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o400); err != nil {
		t.Fatal(err)
	}
	p, err := preparation.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Prepare(context.Background(), subject, a); !errors.Is(err, contextbundle.ErrDigest) {
		t.Fatalf("digest mismatch accepted: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(c.Root, "workspaces"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed preparation leaked slot: %v", err)
	}
}
func TestPreparedFactsRejectIdentityAndAuthorityInflation(t *testing.T) {
	a, c := fixture(t)
	p, err := preparation.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	result, err := p.Prepare(context.Background(), subject, a)
	if err != nil {
		t.Fatal(err)
	}
	changes := []func(*preparation.Facts){
		func(f *preparation.Facts) { f.ExecutionStarted = true }, func(f *preparation.Facts) { f.OSIsolated = true },
		func(f *preparation.Facts) { f.InputDigest = canonical.BytesDigest([]byte("wrong")) },
		func(f *preparation.Facts) { f.BundleDigest = canonical.BytesDigest([]byte("wrong")) },
		func(f *preparation.Facts) { f.BaseCommit = strings.Repeat("0", 40) },
		func(f *preparation.Facts) { f.Context.Entries[0].File = "../outside" },
	}
	for i, change := range changes {
		raw, _ := json.Marshal(result.Facts)
		var f preparation.Facts
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		change(&f)
		if f.Check(a) == nil {
			t.Fatalf("invalid fact %d accepted", i)
		}
	}
	result.Facts.SourceDigest = canonical.BytesDigest([]byte("invented"))
	if p.Recheck(context.Background(), a, result) == nil {
		t.Fatal("facts detached from actual workspace")
	}
}

type transport struct {
	a             workerqueue.Assignment
	calls, claims int
	saved         *preparation.Receipt
	reject        bool
}

func (*transport) Subject() string { return subject }
func (c *transport) Call(_ context.Context, _, path string, in, out any) error {
	assign := func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, out)
	}
	switch path {
	case "/api/v1/worker/prepare-claim":
		c.claims++
		return assign(struct {
			Assignment *workerqueue.Assignment `json:"assignment"`
		}{&c.a})
	case "/api/v1/worker/renew":
		return assign(struct {
			Until time.Time `json:"lease_until"`
		}{time.Now().Add(time.Minute)})
	case "/api/v1/worker/prepared":
		c.calls++
		report := in.(preparation.Report)
		if c.saved == nil {
			v, err := workerqueue.Validate(c.a)
			if err != nil {
				return err
			}
			digest, err := canonical.Digest(report.Facts)
			if err != nil {
				return err
			}
			c.saved = &preparation.Receipt{Kind: preparation.Kind, Admission: workerqueue.Receipt{Token: c.a.Token, Kind: workerqueue.Validated, Worker: subject, Validation: v, ReceivedAt: time.Now()}, Facts: report.Facts, FactsDigest: digest, ReceivedAt: time.Now()}
			return errors.New("test transport: first acknowledgement lost")
		}
		if c.reject {
			c.saved.Facts.ExecutionStarted = true
		}
		return assign(c.saved)
	default:
		return errors.New("unexpected API path")
	}
}
func TestPrepareAgentRetriesOnlyIdenticalReceiptNotLocalWork(t *testing.T) {
	a, c := fixture(t)
	p, err := preparation.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := &transport{a: a}
	receipt, err := workeragent.PrepareOnce(context.Background(), client, a.Token.Profile, p)
	if err != nil {
		t.Fatal(err)
	}
	if client.claims != 1 || client.calls != 2 || receipt.Kind != preparation.Kind {
		t.Fatalf("unsafe retry behavior %#v", client)
	}
	entries, err := os.ReadDir(filepath.Join(c.Root, "workspaces"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("local work repeated: %v", err)
	}
}
