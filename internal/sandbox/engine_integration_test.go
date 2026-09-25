package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sandbox/testutil"
)

func TestRealOfflineContainerBoundaryAndCancellation(t *testing.T) {
	f := testutil.Build(t)
	source, bundle := t.TempDir(), t.TempDir()
	for path, body := range map[string]string{filepath.Join(source, "hello.txt"): "actual source\n", filepath.Join(bundle, "manifest.json"): "{}"} {
		if err := os.WriteFile(path, []byte(body), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(t.TempDir(), "host-secret")
	if err := os.WriteFile(secret, []byte("never expose"), 0o600); err != nil {
		t.Fatal(err)
	}
	guard, err := os.ReadFile(f.Guard)
	if err != nil {
		t.Fatal(err)
	}
	p := sandbox.Profile{Image: f.Image, GuardDigest: sandbox.Hash(guard), Argv: []string{"/probe", secret}, Seconds: 5}
	e, err := sandbox.New(f.Socket, f.Guard)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	result, err := e.Run(context.Background(), p, source, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !strings.Contains(string(result.Stdout), "OFFLINE_PROBE_PASS") {
		t.Fatalf("probe failed: %d %s %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	p.Argv = []string{"/probe", "flood"}
	if _, err := e.Run(context.Background(), p, source, bundle); err == nil {
		t.Fatal("guard output limit was silently truncated into a receipt")
	}
	p.Argv = []string{"/probe", "sleep"}
	p.Seconds = 2
	result, err = e.Run(context.Background(), p, source, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("PID1 guard did not enforce timer: %#v", result)
	}
	p.Seconds = 30
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := e.Run(ctx, p, source, bundle); done <- err }()
	// Wait for an actual running owned container, not an arbitrary sleep.
	deadline := time.Now().Add(5 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		cmd := exec.Command("docker", "ps", "--filter", "label=engineering-platform.offline", "--filter", "ancestor="+f.Image, "--format", "{{.ID}}")
		data, _ := cmd.Output()
		if strings.TrimSpace(string(data)) != "" {
			seen = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancellation returned success")
	}
	if !seen {
		t.Fatal("cancellation test never reached running container")
	}
	cmd := exec.Command("docker", "ps", "-aq", "--filter", "label=engineering-platform.offline", "--filter", "ancestor="+f.Image)
	data, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(data)) != "" {
		t.Fatal("container survived confirmed cleanup", err, string(data))
	}
	// Retain an explicit test log rather than labelling helper output as a model.
	proof, _ := json.Marshal(map[string]any{"engine_image": f.Image, "guard": p.GuardDigest, "scope": "offline-container-fixture-not-codex"})
	t.Log(string(proof))
}
