package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sandbox/testutil"
)

func TestRealOfflineBuildOutputContract(t *testing.T) {
	f := testutil.Build(t)
	source, bundle := t.TempDir(), t.TempDir()
	guard, err := os.ReadFile(f.Guard)
	if err != nil {
		t.Fatal(err)
	}
	p := sandbox.Profile{Image: f.Image, GuardDigest: sandbox.Hash(guard), Argv: []string{"/probe", "build-output", "good"}, Seconds: 5, Outputs: []sandbox.OutputSpec{{Name: "app.bin", MaxBytes: 64}, {Name: "app.map", MaxBytes: 64}}}
	e, err := sandbox.New(f.Socket, f.Guard)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, mode := range []string{"good", "background", "missing", "extra", "symlink", "hardlink", "fifo", "large", "directory", "forge-log", "failed"} {
		t.Run(mode, func(t *testing.T) {
			p.Argv = []string{"/probe", "build-output", mode}
			r, err := e.Run(context.Background(), p, source, bundle)
			switch mode {
			case "good", "background":
				if err != nil || r.BuildOutputs == nil || len(r.BuildOutputs.Files) != 2 || string(r.BuildOutputs.Files[0].Bytes) != "actual output\x00\xff" || !r.BuildOutputs.ChildrenReaped || r.ExitCode != 0 {
					t.Fatalf("actual outputs rejected: %v %+v", err, r)
				}
				if !strings.Contains(string(r.Stdout), "OFFLINE_BUILD_PASS") {
					t.Fatal("command stdout lost")
				}
			case "failed":
				if err != nil || r.ExitCode != 7 || r.BuildOutputs == nil || r.BuildOutputs.State != "NOT_COLLECTED_EXIT_NONZERO" || len(r.BuildOutputs.Files) != 0 {
					t.Fatal("failed command output mislabeled", err, r)
				}
			default:
				if err == nil {
					t.Fatal("invalid output/log became receipt", mode)
				}
			}
		})
	}
	// Source/Context mounts and permissions are unchanged by output collection.
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 0 {
		t.Fatal("source changed")
	}
	entries, err = os.ReadDir(bundle)
	if err != nil || len(entries) != 0 {
		t.Fatal("context changed")
	}
}

func TestRealOfflineCBuildOutputs(t *testing.T) {
	f := testutil.BuildCompiler(t)
	source, bundle := t.TempDir(), t.TempDir()
	code := []byte("int entry(void) { return 42; }\n")
	if err := os.WriteFile(filepath.Join(source, "main.c"), code, 0400); err != nil {
		t.Fatal(err)
	}
	guard, err := os.ReadFile(f.Guard)
	if err != nil {
		t.Fatal(err)
	}
	p := sandbox.Profile{Image: f.Image, GuardDigest: sandbox.Hash(guard), Argv: []string{"/usr/bin/gcc", "-nostdlib", "-ffreestanding", "-fno-pie", "-no-pie", "/workspace/main.c", "-Wl,-e,entry,-Map=/tmp/ep-output/app.map", "-o", "/tmp/ep-output/app.elf"}, Seconds: 15, Outputs: []sandbox.OutputSpec{{Name: "app.elf", MaxBytes: 128 << 10}, {Name: "app.map", MaxBytes: 64 << 10}}}
	e, err := sandbox.New(f.Socket, f.Guard)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	r, err := e.Run(context.Background(), p, source, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 {
		t.Fatalf("actual isolated compiler exit %d: %s", r.ExitCode, r.Stderr)
	}
	if r.BuildOutputs == nil || len(r.BuildOutputs.Files) != 2 || len(r.BuildOutputs.Files[0].Bytes) < 4 || string(r.BuildOutputs.Files[0].Bytes[:4]) != "\x7fELF" || !strings.Contains(string(r.BuildOutputs.Files[1].Bytes), "entry") {
		t.Fatal("native compiler artifacts missing")
	}
	after, err := os.ReadFile(filepath.Join(source, "main.c"))
	if err != nil || string(after) != string(code) {
		t.Fatal("source bytes changed")
	}
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 1 {
		t.Fatal("build output leaked into source")
	}

	// Prove retained bytes do not depend on the destroyed container or original
	// source/staging files. This synthetic Run names only this test operation.
	stage, store := t.TempDir(), t.TempDir()
	for _, dir := range []string{stage, store} {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	plan := artifactset.Plan{Version: 1, Subject: artifactset.Subject{RunID: "offline-c-build-test", ExecutionID: strings.Repeat("a", 64), TaskDigest: p.GuardDigest, InputDigest: r.ProfileDigest, BaseCommit: strings.Repeat("b", 40)}}
	for _, f := range r.BuildOutputs.Files {
		file := filepath.Join(stage, f.Name)
		if err := os.WriteFile(file, f.Bytes, 0600); err != nil {
			t.Fatal(err)
		}
		plan.Members = append(plan.Members, artifactset.Input{Entry: artifactset.Entry{ID: f.Name, Kind: "build-output", Size: int64(f.Size), Digest: f.Digest}, Path: file})
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	planFile := filepath.Join(stage, "plan.json")
	if err := os.WriteFile(planFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(store, "build.tar")
	packed, err := artifactset.Pack(context.Background(), planFile, sandbox.Hash(raw), archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{source, bundle, stage} {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(store, "restored")
	restored, err := artifactset.Restore(context.Background(), archive, packed.ArchiveDigest, plan.Subject.RunID, target)
	if err != nil || restored.ExecutionAuthorized || restored.ProductionQualified {
		t.Fatal("unsafe or failed output restoration", err)
	}
	for _, f := range r.BuildOutputs.Files {
		name := filepath.Join(target, "files", f.Name)
		raw, err := os.ReadFile(name)
		if err != nil || sandbox.Hash(raw) != f.Digest {
			t.Fatal("restored output differs")
		}
		st, err := os.Stat(name)
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal("restored binary became executable")
		}
	}
	t.Logf("actual isolated host-C build retained ELF/map, source unchanged; image=%s profile=%s", f.Image, r.ProfileDigest)
}
