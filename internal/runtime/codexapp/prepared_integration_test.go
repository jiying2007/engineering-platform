package codexapp

import (
	"context"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
	"github.com/jiying2007/engineering-platform/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreparedWorkspaceContextAndAppServerSubprocess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repository := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git fixture: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	git("config", "user.name", "test")
	git("config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "source.txt"), []byte("frozen source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "source.txt")
	git("commit", "-m", "fixture")
	base := git("rev-parse", "HEAD")
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := manager.Create(ctx, workspace.Spec{ID: "integration", Repository: repository, BaseCommit: base})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := manager.Cleanup(context.Background(), w); err != nil {
			t.Error(err)
		}
	}()
	data := []byte("Approved design data, not an execution grant.\n")
	ref := core.ContextRef{Source: "docs:design", Type: "DOCUMENT", Version: "r1", Digest: canonical.BytesDigest(data), Trust: core.ContextApproved}
	input := core.RunInputManifest{RunID: "offline-run", TaskContractDigest: canonical.BytesDigest([]byte("task")), ContextRefs: []core.ContextRef{ref}, RuntimeProfile: "codex-read-only", ToolProfile: "read-only", WorkerProfile: "offline", PolicyProfile: "offline"}
	digest, _ := input.Digest()
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, strings.TrimPrefix(ref.Digest, "sha256:")+".bin"), data, 0o400); err != nil {
		t.Fatal(err)
	}
	source, err := contextbundle.NewLocalSource(sourceRoot, []contextbundle.LocalApproval{{Subject: contextbundle.Subject{RunID: input.RunID, TaskContractDigest: input.TaskContractDigest, RunInputDigest: digest}, Refs: input.ContextRefs}})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	materializer, err := contextbundle.New(t.TempDir(), source, source, contextbundle.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer materializer.Close()
	bundle, err := materializer.Materialize(ctx, input, w.WorktreePath)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(bundle.Path, 0o700)
	verified, err := materializer.Verify(ctx, input)
	if err != nil || verified.Digest != bundle.Digest {
		t.Fatal("bundle verification", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	exerciseProcess(t, NewProvider(exe), runtimeprovider.LaunchSpec{Dir: w.WorktreePath, Env: []string{"HOME=" + w.HomePath}})
	head, err := manager.Head(ctx, w)
	if err != nil || head != base {
		t.Fatal("base changed", err)
	}
	clean, err := manager.IsClean(ctx, w)
	if err != nil || !clean {
		t.Fatal("workspace modified", err)
	}
	// Offline subprocess proves protocol assembly. It is NOT a model/provider pilot,
	// an OS sandbox check, or proof the real Codex model consumed Context bytes.
}
