//go:build linux

package distribution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func installOK(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

// Real clean Git/buildinfo role fixture, not a deployed platform or service.
// Actual shipped programs are also tested by test-distribution.py and native CI.
func installFixture(t *testing.T, salt ...string) (string, string) {
	t.Helper()
	repo := t.TempDir()
	dist := t.TempDir()
	installOK(t, os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module "+Module+"\n\ngo 1.25.0\n"), 0600))
	fixtureSource := "package main\nfunc main() {}\n"
	if len(salt) > 1 {
		t.Fatal("at most one fixture salt")
	}
	if len(salt) == 1 {
		fixtureSource = fmt.Sprintf("package main\nconst fixtureSalt = %q\nfunc main() {}\n", salt[0])
	}
	for _, n := range Names() {
		dir := filepath.Join(repo, "cmd", n)
		installOK(t, os.MkdirAll(dir, 0700))
		installOK(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(fixtureSource), 0600))
	}
	run := func(tool string, args ...string) string {
		t.Helper()
		c := exec.Command(tool, args...)
		c.Dir = repo
		c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GOPROXY=off", "GOTOOLCHAIN=local")
		out, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("%s: %v %s", tool, e, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("git", "init")
	run("git", "add", ".")
	run("git", "-c", "user.name=TEST fixture", "-c", "user.email=test@example.invalid", "commit", "-m", "synthetic role source")
	sha := run("git", "rev-parse", "HEAD")
	var facts []File
	var sums strings.Builder
	for _, n := range Names() {
		run("go", "build", "-trimpath", "-o", filepath.Join(dist, n), "./cmd/"+n)
		raw, e := os.ReadFile(filepath.Join(dist, n))
		installOK(t, e)
		d := rawDigest(raw)
		facts = append(facts, File{Path: n, Size: int64(len(raw)), Digest: d})
		fmt.Fprintf(&sums, "%s  %s\n", d[7:], n)
	}
	raw, e := json.Marshal(facts)
	installOK(t, e)
	installOK(t, os.WriteFile(filepath.Join(dist, "file-manifest.json"), raw, 0644))
	installOK(t, os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(sums.String()), 0644))
	_, e = Verify(dist)
	installOK(t, e)
	return dist, sha
}

func TestInstallationCopiesExactRolesAndCanonicalTemplates(t *testing.T) {
	from, sha := installFixture(t)
	dest := filepath.Join(t.TempDir(), "installed")
	r, e := Install(context.Background(), from, dest, sha)
	installOK(t, e)
	if r.Status != "INSTALLED_BYTES_VERIFIED" || r.BinaryCount != 6 || r.TemplateCount < 20 || r.SourceCommit != sha || r.ServicesStarted || r.ConfigurationApplied || r.DependenciesIncluded || r.ExecutionAuthorized || r.ProductionQualified {
		t.Fatal("installation promoted authority", r)
	}
	installOK(t, os.RemoveAll(from))
	r2, e := VerifyInstallation(context.Background(), dest, sha)
	installOK(t, e)
	if r2.ManifestDigest != r.ManifestDigest {
		t.Fatal("installed manifest drift")
	}
	for _, mode := range []string{"extra", "missing", "template-bytes", "template-mode", "binary-link", "root-mode", "manifest"} {
		t.Run(mode, func(t *testing.T) {
			file := filepath.Join(dest, "templates", "control.env.example")
			raw, e := os.ReadFile(file)
			installOK(t, e)
			switch mode {
			case "extra":
				file = filepath.Join(dest, "extra")
				installOK(t, os.Mkdir(file, 0755))
				defer os.Remove(file)
			case "missing":
				installOK(t, os.Remove(file))
				defer os.WriteFile(file, raw, 0444)
			case "template-bytes":
				installOK(t, os.Chmod(file, 0644))
				installOK(t, os.WriteFile(file, []byte("FORGED"), 0444))
				installOK(t, os.Chmod(file, 0444))
				defer func() { _ = os.Chmod(file, 0644); _ = os.WriteFile(file, raw, 0444); _ = os.Chmod(file, 0444) }()
			case "template-mode":
				installOK(t, os.Chmod(file, 0644))
				defer os.Chmod(file, 0444)
			case "binary-link":
				file = filepath.Join(t.TempDir(), "alias")
				installOK(t, os.Link(filepath.Join(dest, "bin", "eng"), file))
				defer os.Remove(file)
			case "root-mode":
				installOK(t, os.Chmod(dest, 0777))
				defer os.Chmod(dest, 0755)
			case "manifest":
				file = filepath.Join(dest, "installation-manifest.json")
				raw, e = os.ReadFile(file)
				installOK(t, e)
				installOK(t, os.Chmod(file, 0644))
				installOK(t, os.WriteFile(file, []byte("{}\n"), 0444))
				installOK(t, os.Chmod(file, 0444))
				defer func() { _ = os.Chmod(file, 0644); _ = os.WriteFile(file, raw, 0444); _ = os.Chmod(file, 0444) }()
			}
			if _, e = VerifyInstallation(context.Background(), dest, sha); e == nil {
				t.Fatal("accepted malformed installed tree", mode)
			}
		})
	}
	_, e = VerifyInstallation(context.Background(), dest, sha)
	installOK(t, e)
}

func TestInstallationPreflightAndConcurrentNoOverwrite(t *testing.T) {
	from, sha := installFixture(t)
	for _, mode := range []string{"wrong-source", "exists", "alias-parent", "writable-parent", "overlap", "cancelled", "relative"} {
		t.Run(mode, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "installed")
			expected := sha
			ctx := context.Background()
			switch mode {
			case "wrong-source":
				expected = strings.Repeat("0", 40)
			case "exists":
				installOK(t, os.Mkdir(dest, 0700))
				installOK(t, os.WriteFile(filepath.Join(dest, "KEEP"), []byte("keep"), 0600))
			case "alias-parent":
				alias := filepath.Join(t.TempDir(), "alias")
				installOK(t, os.Symlink(parent, alias))
				dest = filepath.Join(alias, "installed")
			case "writable-parent":
				installOK(t, os.Chmod(parent, 0777))
			case "overlap":
				dest = filepath.Join(from, "installed")
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "relative":
				dest = "relative-installed"
			}
			if _, e := Install(ctx, from, dest, expected); e == nil {
				t.Fatal("unsafe install accepted")
			}
			if mode == "exists" {
				raw, e := os.ReadFile(filepath.Join(dest, "KEEP"))
				installOK(t, e)
				if string(raw) != "keep" {
					t.Fatal("overwritten")
				}
			} else if _, e := os.Lstat(dest); !os.IsNotExist(e) {
				t.Fatal("preflight rejection created destination", e)
			}
		})
	}
	dest := filepath.Join(t.TempDir(), "installed")
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, e := Install(context.Background(), from, dest, sha); done <- e }()
	}
	wins := 0
	for i := 0; i < 2; i++ {
		if <-done == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("unexpected successful installs", wins)
	}
	_, e := VerifyInstallation(context.Background(), dest, sha)
	installOK(t, e)
}
