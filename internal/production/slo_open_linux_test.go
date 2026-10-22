//go:build linux

package production

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestOperationalSLOOpenRejectsReplacementFIFOAndSymlink(t *testing.T) {
	// A child process bounds the regression: replacing O_NONBLOCK with a plain
	// open must fail this test instead of hanging the whole test runner.
	if os.Getenv("ENG_TEST_OPERATIONAL_SLI_OPEN_CHILD") != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestOperationalSLOOpenRejectsReplacementFIFOAndSymlink$", "-test.v")
		cmd.Env = append(os.Environ(), "ENG_TEST_OPERATIONAL_SLI_OPEN_CHILD=1")
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("substituted FIFO blocked descriptor open: %v", ctx.Err())
		}
		if err != nil {
			t.Fatalf("descriptor rejection failed: %v\n%s", err, out)
		}
		return
	}
	for _, kind := range []string{"fifo", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			name := "source.json"
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			before, err := root.Lstat(name)
			if err != nil || !before.Mode().IsRegular() {
				t.Fatal("fixture was not a regular source before substitution")
			}
			if err := os.Rename(path, filepath.Join(dir, "saved.json")); err != nil {
				t.Fatal(err)
			}
			if kind == "fifo" {
				err = syscall.Mkfifo(path, 0o600)
			} else {
				// Resolving this in-root link would return the original inode,
				// so descriptor identity alone would not reject it.
				err = os.Symlink("saved.json", path)
			}
			if err != nil {
				t.Fatal(err)
			}
			f, err := openOperationalSLIFile(root, name)
			if f != nil {
				f.Close()
			}
			if err == nil {
				t.Fatalf("substituted %s was opened", kind)
			}
		})
	}
}

func TestOperationalSLOManifestUsesSafeReadback(t *testing.T) {
	for _, kind := range []string{"regular", "fifo", "symlink", "writable", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "manifest.json")
			raw := []byte(`{"version":3}`)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "fifo", "symlink":
				if err := os.Rename(path, filepath.Join(dir, "saved.json")); err != nil {
					t.Fatal(err)
				}
				if kind == "fifo" {
					err = syscall.Mkfifo(path, 0o600)
				} else {
					err = os.Symlink("saved.json", path)
				}
			case "writable":
				err = os.Chmod(path, 0o666)
			case "oversize":
				err = os.WriteFile(path, make([]byte, (1<<20)+1), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := ReadOperationalSLOManifest(path)
			if kind == "regular" {
				if err != nil || string(got) != string(raw) {
					t.Fatalf("regular manifest readback: %v", err)
				}
			} else if err == nil {
				t.Fatalf("unsafe %s manifest accepted", kind)
			}
		})
	}
}
