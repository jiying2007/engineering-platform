// Package testutil builds isolated, no-network scratch fixtures. Only tests use it.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type Fixture struct{ Image, Guard, Socket string }

func Build(t *testing.T) Fixture         { return build(t, false) }
func BuildCompiler(t *testing.T) Fixture { return build(t, true) }
func build(t *testing.T, compiler bool) Fixture {
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
	dockerfile := "FROM scratch\nCOPY probe /probe\n"
	if compiler {
		copyCompiler(t, dir, run)
		dockerfile += "COPY rootfs /\n"
	}
	dockerfile += "LABEL engineering-platform.fixture=" + hex.EncodeToString(nonce) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
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

// Copy the trusted CI host compiler and only its native runtime dependencies
// into a no-network scratch fixture. No package manager, mutable image tag or
// host compiler mount enters the product Engine. The image ID binds these bytes.
func copyCompiler(t *testing.T, dir string, run func(string, ...string) string) {
	t.Helper()
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Fatal("native build integration requires gcc", err)
	}
	files := map[string]string{"/usr/bin/gcc": gcc}
	for _, name := range []string{"cc1", "collect2"} {
		p := run(gcc, "-print-prog-name="+name)
		if !filepath.IsAbs(p) {
			t.Fatal("missing compiler component", name)
		}
		files[p] = p
	}
	plugin := run(gcc, "-print-file-name=liblto_plugin.so")
	if !filepath.IsAbs(plugin) {
		t.Fatal("missing compiler plugin")
	}
	files[plugin] = plugin
	for _, name := range []string{"as", "ld"} {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		files["/usr/bin/"+name] = p
	}
	native := []string{}
	for _, src := range files {
		native = append(native, src)
	}
	for _, src := range native {
		dependencies := run("ldd", src)
		if strings.Contains(dependencies, "not found") {
			t.Fatal("missing native runtime dependency", src)
		}
		for _, field := range strings.Fields(dependencies) {
			if filepath.IsAbs(field) {
				files[field] = field
			}
		}
	}
	for dst, src := range files {
		resolved, err := filepath.EvalSymlinks(src)
		if err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(resolved)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "rootfs", strings.TrimPrefix(dst, "/"))
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			in.Close()
			t.Fatal(err)
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0555)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(out, in)
		closeIn, closeOut := in.Close(), out.Close()
		if copyErr != nil || closeIn != nil || closeOut != nil {
			t.Fatal("native tool copy failed", copyErr, closeIn, closeOut)
		}
	}
}
