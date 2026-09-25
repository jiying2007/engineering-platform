// Package testutil builds isolated, no-network scratch fixtures. Only tests use it.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type Fixture struct{ Image, Guard, Socket string }

func Build(t *testing.T) Fixture {
	t.Helper()
	if os.Getenv("EP_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set EP_SANDBOX_INTEGRATION=1 for mandatory real Docker integration")
	}
	if os.Getuid() == 0 {
		t.Fatal("real sandbox suite must run as a non-root worker")
	}
	_, this, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(this), "../../.."))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	socket, err := filepath.EvalSymlinks("/var/run/docker.sock")
	if err != nil {
		t.Fatal(err)
	}
	fixture := Fixture{Guard: filepath.Join(dir, "guard"), Socket: socket}
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture %s: %v: %s", name, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("go", "build", "-o", fixture.Guard, "./cmd/sandbox-guard")
	run("go", "build", "-o", filepath.Join(dir, "probe"), "./internal/sandbox/testdata/probe.go")
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\nCOPY probe /probe\nLABEL engineering-platform.fixture="+hex.EncodeToString(nonce)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	iid := filepath.Join(dir, "image-id")
	run("docker", "build", "--network=none", "--iidfile", iid, dir)
	data, err := os.ReadFile(iid)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Image = strings.TrimSpace(string(data))
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		cmd := exec.CommandContext(clean, "docker", "image", "rm", fixture.Image)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("remove fixture image: %v %s", err, out)
		}
	})
	return fixture
}
