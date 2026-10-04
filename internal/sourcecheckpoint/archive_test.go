package sourcecheckpoint

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func contractFixture(t *testing.T) (codexexec.Profile, codexexec.Permit) {
	t.Helper()
	p := codexexec.Profile{
		Version:                 3,
		Provider:                provideridentity.OpenAIWIFUnattended(),
		CodexVersion:            "0.157.1",
		BinaryDigest:            "sha256:" + strings.Repeat("a", 64),
		QualificationDigest:     "sha256:" + strings.Repeat("e", 64),
		EngineeringConfigDigest: codexapp.EngineeringConfigDigest(),
		Model:                   "gpt-test",
		Sandbox:                 "workspace-write",
		ApprovalPolicy:          "never",
	}
	pd, err := p.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{
		ID:                     "task",
		WorkItemID:             "work",
		TaskType:               "FEATURE",
		Repository:             "repo",
		BaseCommit:             strings.Repeat("1", 40),
		AcceptanceCriteria:     []string{"change code", "tests pass"},
		AllowedActions:         []string{codexexec.Action},
		ExpectedOutputs:        []string{"source change"},
		VerificationPlanID:     "vp",
		VerificationPlanDigest: "sha256:" + strings.Repeat("2", 64),
		Revision:               1,
	}
	td, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	input := core.RunInputManifest{
		RunID:              "run",
		TaskContractDigest: td,
		RuntimeProfile:     "codex/runtime",
		ToolProfile:        "codex/" + pd,
		WorkerProfile:      "worker/codex",
		PolicyProfile:      "policy",
	}
	id, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	intent := workerqueue.Intent{
		RunID:          "run",
		TaskDigest:     td,
		InputDigest:    id,
		ExecutionEpoch: 1,
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	wt := workerqueue.Token{
		Profile:       "worker/codex",
		InboxID:       1,
		Generation:    1,
		RecoveryEpoch: 0,
	}
	a := workerqueue.Assignment{
		Token:        wt,
		LeaseUntil:   time.Unix(100, 0),
		Intent:       intent,
		IntentDigest: intentDigest,
		Input:        input,
		Task:         task,
	}
	validation, err := workerqueue.Validate(a)
	if err != nil {
		t.Fatal(err)
	}
	manifest := contextbundle.Manifest{
		SchemaVersion:  1,
		RunInputDigest: id,
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bundleDigest := canonical.BytesDigest(manifestRaw)
	facts := preparation.Facts{
		Version:         1,
		IntentDigest:    intentDigest,
		InputDigest:     id,
		TaskDigest:      td,
		ApprovalDigest:  "sha256:" + strings.Repeat("3", 64),
		BaseCommit:      task.BaseCommit,
		TreeCommit:      strings.Repeat("4", 40),
		WorkspaceRecipe: workspace.Recipe,
		SourceDigest:    "sha256:" + strings.Repeat("5", 64),
		ConfigDigest:    "sha256:" + strings.Repeat("6", 64),
		BundleDigest:    bundleDigest,
		Context:         manifest,
	}
	fd, err := canonical.Digest(facts)
	if err != nil {
		t.Fatal(err)
	}
	worker := "urn:engineering-platform:worker:codex"
	prep := preparation.Receipt{
		Kind: preparation.Kind,
		Admission: workerqueue.Receipt{
			Token:      wt,
			Worker:     worker,
			Kind:       workerqueue.Validated,
			Validation: validation,
			ReceivedAt: time.Unix(10, 0),
		},
		Facts:       facts,
		FactsDigest: fd,
		ReceivedAt:  time.Unix(11, 0),
	}
	return p, codexexec.Permit{
		Token: codexexec.Token{
			ID:            strings.Repeat("d", 64),
			RunID:         "run",
			WorkerProfile: "worker/codex",
			ProfileDigest: pd,
		},
		Assignment:  a,
		Preparation: prep,
		Profile:     p,
		LeaseUntil:  time.Unix(200, 0),
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) (string, string, codexexec.Permit, codexexec.ControlTranscript) {
	t.Helper()
	root := t.TempDir()
	must(t, os.Chmod(root, 0700))
	source := filepath.Join(root, "source")
	out := filepath.Join(root, "archives")
	must(t, os.Mkdir(source, 0700))
	must(t, os.Mkdir(out, 0700))
	_, p := contractFixture(t)
	tr := codexexec.ControlTranscript{Version: 2, Close: codexexec.ControlClose{Binding: codexexec.ControlBinding{Token: p.Token, ExecutionEpoch: 1, ThreadID: "thread", TurnID: "turn"}, TurnStatus: "interrupted", ProcessScope: testsupport.ProcessScopeFixture()}}
	for _, d := range []string{".git", "empty", "ignored", "nested"} {
		must(t, os.Mkdir(filepath.Join(source, d), 0700))
	}
	for name, data := range map[string][]byte{".git/SECRET": []byte("EXCLUDED-GIT-METADATA"), ".gitignore": []byte("ignored/\n"), "changed.c": []byte("// 未完成但需要保留\n"), "ignored/blob": {0, 1, 2, 255}, "nested/zero": {}} {
		must(t, os.WriteFile(filepath.Join(source, name), data, 0600))
	}
	must(t, os.WriteFile(filepath.Join(source, "run.sh"), []byte("#!/bin/sh\n"), 0700))
	must(t, os.Symlink("../changed.c", filepath.Join(source, "nested/link")))
	return source, out, p, tr
}
func TestSourceCheckpointRawRoundTripAndNoOverwrite(t *testing.T) {
	src, out, p, tr := fixture(t)
	ctx := context.Background()
	a, err := Capture(ctx, src, out, p, tr)
	must(t, err)
	raw, err := os.ReadFile(a.Path)
	must(t, err)
	if bytes.Contains(raw, []byte("EXCLUDED-GIT-METADATA")) {
		t.Fatal("Git metadata copied")
	}
	facts, err := Verify(ctx, a.Path, a.Facts.ArchiveDigest, p.Token.RunID)
	must(t, err)
	if facts != a.Facts {
		t.Fatal("readback drift")
	}
	dst := filepath.Join(filepath.Dir(out), "recovery")
	facts, err = Restore(ctx, a.Path, a.Facts.ArchiveDigest, p.Token.RunID, dst)
	must(t, err)
	if facts != a.Facts {
		t.Fatal("restore facts drift")
	}
	if _, err := os.Lstat(filepath.Join(dst, "source/.git")); !os.IsNotExist(err) {
		t.Fatal("Git metadata restored")
	}
	if _, err := os.Lstat(filepath.Join(dst, "INCOMPLETE")); !os.IsNotExist(err) {
		t.Fatal("incomplete restore")
	}
	got, err := os.ReadFile(filepath.Join(dst, "source/ignored/blob"))
	must(t, err)
	if !bytes.Equal(got, []byte{0, 1, 2, 255}) {
		t.Fatal("ignored bytes lost")
	}
	target, err := os.Readlink(filepath.Join(dst, "source/nested/link"))
	must(t, err)
	if target != "../changed.c" {
		t.Fatal("link changed")
	}
	st, err := os.Stat(filepath.Join(dst, "source/run.sh"))
	must(t, err)
	if st.Mode().Perm() != 0700 {
		t.Fatal("execute bit lost")
	}
	if _, err = Restore(ctx, a.Path, a.Facts.ArchiveDigest, p.Token.RunID, dst); err == nil {
		t.Fatal("existing destination overwritten")
	}
	if _, err = Capture(ctx, src, out, p, tr); err == nil {
		t.Fatal("existing checkpoint overwritten")
	}
	after, _ := os.ReadFile(a.Path)
	if !bytes.Equal(raw, after) {
		t.Fatal("immutable archive changed")
	}
}
func TestSourceCheckpointRequiresProofAndExactPermit(t *testing.T) {
	for _, mode := range []string{"unreaped", "foreign", "bad-transcript", "bad-task"} {
		t.Run(mode, func(t *testing.T) {
			src, out, p, tr := fixture(t)
			switch mode {
			case "unreaped":
				tr.Close.ProcessScope.InitReaped = false
				tr.Close.ProcessScope.ReapedAt = time.Time{}
			case "foreign":
				tr.Close.Binding.Token.RunID = "other"
			case "bad-transcript":
				tr.Version = 1
			case "bad-task":
				p.Assignment.Intent.TaskDigest = canonical.BytesDigest([]byte("other"))
			}
			if _, err := Capture(context.Background(), src, out, p, tr); err == nil {
				t.Fatal("invalid capture accepted")
			}
			files, _ := os.ReadDir(out)
			if len(files) != 0 {
				t.Fatal("invalid capture wrote archive")
			}
		})
	}
}
func TestSourceCheckpointRejectsUnsafeFilesystem(t *testing.T) {
	for _, mode := range []string{"absolute-link", "escape-link", "chain-escape", "cycle", "too-large", "nested-git", "overlap", "aliased-root"} {
		t.Run(mode, func(t *testing.T) {
			src, out, p, tr := fixture(t)
			switch mode {
			case "absolute-link":
				must(t, os.Symlink("/etc/passwd", filepath.Join(src, "bad")))
			case "escape-link":
				must(t, os.Symlink("../outside", filepath.Join(src, "bad")))
			case "chain-escape":
				must(t, os.Symlink("..", filepath.Join(src, "nested/back")))
				must(t, os.Symlink("nested/back/../outside", filepath.Join(src, "bad")))
			case "cycle":
				must(t, os.Symlink("bad", filepath.Join(src, "bad")))
			case "too-large":
				f, err := os.Create(filepath.Join(src, "big"))
				must(t, err)
				must(t, f.Truncate(MaxFile+1))
				must(t, f.Close())
			case "nested-git":
				must(t, os.Mkdir(filepath.Join(src, "nested/.git"), 0700))
			case "overlap":
				out = filepath.Join(src, "output")
				must(t, os.Mkdir(out, 0700))
			case "aliased-root":
				link := src + "-alias"
				must(t, os.Symlink(src, link))
				src = link
			}
			if _, err := Capture(context.Background(), src, out, p, tr); err == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}
func mutateArchive(t *testing.T, raw []byte, mutate func(*Manifest), extra bool) []byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(raw))
	h, err := tr.Next()
	must(t, err)
	data, err := io.ReadAll(tr)
	must(t, err)
	var m Manifest
	must(t, json.Unmarshal(data, &m))
	mutate(&m)
	data, err = json.Marshal(m)
	must(t, err)
	h.Size = int64(len(data))
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	must(t, tw.WriteHeader(h))
	_, err = tw.Write(data)
	must(t, err)
	for {
		head, e := tr.Next()
		if e == io.EOF {
			break
		}
		must(t, e)
		must(t, tw.WriteHeader(head))
		_, err = io.Copy(tw, tr)
		must(t, err)
	}
	if extra {
		must(t, tw.WriteHeader(&tar.Header{Name: "source/EXTRA", Typeflag: tar.TypeReg, Mode: 0600}))
	}
	must(t, tw.Close())
	return b.Bytes()
}
func TestSourceCheckpointRejectsTamperEvenWithRecomputedOuterHash(t *testing.T) {
	src, out, p, tr := fixture(t)
	ctx := context.Background()
	a, err := Capture(ctx, src, out, p, tr)
	must(t, err)
	raw, err := os.ReadFile(a.Path)
	must(t, err)
	for _, mode := range []string{"missing-entry", "path-traversal", "digest", "extra", "trailer", "scope"} {
		t.Run(mode, func(t *testing.T) {
			changed := mutateArchive(t, raw, func(m *Manifest) {
				switch mode {
				case "missing-entry":
					m.Entries = m.Entries[1:]
				case "path-traversal":
					m.Entries[0].Path = "../escape"
				case "digest":
					m.Entries[0].Digest = canonical.BytesDigest([]byte("other"))
				case "scope":
					m.Transcript.Close.ProcessScope.NamespaceID = m.Transcript.Close.ProcessScope.ParentNamespaceID
				}
				m.SnapshotDigest, _ = canonical.Digest(m.Entries)
			}, mode == "extra")
			if mode == "trailer" {
				changed = append(changed, []byte("unexpected second archive")...)
			}
			filename := filepath.Join(out, mode+".tar")
			must(t, os.WriteFile(filename, changed, 0600))
			dst := filepath.Join(filepath.Dir(out), "restore-"+mode)
			if _, err := Restore(ctx, filename, canonical.BytesDigest(changed), p.Token.RunID, dst); err == nil {
				t.Fatal("tampered archive restored")
			}
			if _, err := os.Lstat(dst); !os.IsNotExist(err) {
				t.Fatal("invalid bytes created recovery directory")
			}
		})
	}
	if _, err := Verify(ctx, a.Path, canonical.BytesDigest([]byte("wrong")), p.Token.RunID); err == nil {
		t.Fatal("wrong anchor accepted")
	}
	if _, err := Verify(ctx, a.Path, a.Facts.ArchiveDigest, "other-run"); err == nil {
		t.Fatal("foreign run accepted")
	}
}
func TestSourceCheckpointEmptyAndCancelled(t *testing.T) {
	src, out, p, tr := fixture(t)
	for _, name := range []string{".gitignore", "changed.c", "ignored", "nested", "empty", "run.sh"} {
		must(t, os.RemoveAll(filepath.Join(src, name)))
	}
	a, err := Capture(context.Background(), src, out, p, tr)
	must(t, err)
	_, err = Verify(context.Background(), a.Path, a.Facts.ArchiveDigest, p.Token.RunID)
	must(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Verify(ctx, a.Path, a.Facts.ArchiveDigest, p.Token.RunID); err == nil {
		t.Fatal("cancelled verification succeeded")
	}
}
