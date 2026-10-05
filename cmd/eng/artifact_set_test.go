package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestArtifactSetCLIValidation(t *testing.T) {
	for _, args := range [][]string{
		{}, {"unknown"}, {"pack"}, {"verify"}, {"restore"},
		{"verify", "--archive", "/a", "--archive-digest", "sha256:" + strings.Repeat("a", 64), "--run", "r", "--run", "x"},
		{"restore", "--archive", "/a", "--archive-digest", "x", "--run", "r"},
		{"pack", "--plan", "/p", "--plan-digest", "x", "--out", "/o", "--core"},
		{"verify", "--archive", "/a", "--archive-digest", "x", "--run", "r", "--out", "/o"},
		{"pack", "--plan", "/p", "--plan-digest", "x", "--out", "/o", "extra"},
		{"verify", "--archive", "/a", "--archive-digest", "x", "--run", "r", "--execute"},
	} {
		if _, err := parseArtifactSet(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}
func TestArtifactSetCLIRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux implementation tested in canonical CI")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.bin")
	data := []byte("TEST-ONLY output\x00bytes")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	p := artifactset.Plan{Version: 1, Subject: artifactset.Subject{RunID: "fixture-run", ExecutionID: strings.Repeat("a", 64), TaskDigest: canonical.BytesDigest([]byte("task")), InputDigest: canonical.BytesDigest([]byte("input")), BaseCommit: strings.Repeat("b", 40)}, Members: []artifactset.Input{{Entry: artifactset.Entry{ID: "firmware.bin", Kind: "build-output", Size: int64(len(data)), Digest: canonical.BytesDigest(data)}, Path: source}}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(root, "plan.json")
	if err = os.WriteFile(plan, raw, 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "set.tar")
	ctx := context.Background()
	r, err := executeArtifactSet(ctx, []string{"pack", "--plan", plan, "--plan-digest", canonical.BytesDigest(raw), "--out", archive})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(plan); err != nil {
		t.Fatal(err)
	}
	args := []string{"verify", "--archive", archive, "--archive-digest", r.ArchiveDigest, "--run", p.Subject.RunID}
	if _, err = executeArtifactSet(ctx, args); err != nil {
		t.Fatal(err)
	}
	args[0] = "restore"
	into := filepath.Join(root, "restored")
	args = append(args, "--into", into)
	restored, err := executeArtifactSet(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ExecutionAuthorized || restored.ProducerSemanticsVerified || restored.ProductionQualified {
		t.Fatal("archive grants authority")
	}
	got, err := os.ReadFile(filepath.Join(into, "files", "firmware.bin"))
	if err != nil || string(got) != string(data) {
		t.Fatal("restore mismatch", err)
	}
	if _, err = executeArtifactSet(ctx, args); err == nil {
		t.Fatal("overwrite accepted")
	}
}
