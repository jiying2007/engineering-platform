//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Selected by the existing mandatory native CI's TestOfflineCommand regex.
// Missing systemd or noninteractive privilege FAILS that lane; local default
// skips only because the caller did not request the ephemeral-host integration.
func TestOfflineCommandSystemdLifecycle(t *testing.T) {
	if os.Getenv("EP_SANDBOX_INTEGRATION") != "1" {
		t.Skip("ephemeral systemd host integration not requested")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	git := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	git.Dir = root
	head, err := git.Output()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts", "ci-systemd-lifecycle.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "EXPECTED_SOURCE="+strings.TrimSpace(string(head)))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native systemd lifecycle: %v\n%s", err, out)
	}
	t.Logf("native systemd lifecycle (not production qualification):\n%s", out)
}
