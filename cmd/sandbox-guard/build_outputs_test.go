package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

func TestCollectBuildOutputsExactPrivateBytes(t *testing.T) {
	root := t.TempDir()
	contract := []sandbox.OutputSpec{{Name: "app.elf", MaxBytes: 16}, {Name: "app.map", MaxBytes: 16}}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, c := range contract {
		if err := os.WriteFile(filepath.Join(root, c.Name), []byte{0, 1, 255}, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files, err := collectBuildFiles(root, contract)
	if err != nil || len(files) != 2 {
		t.Fatal(files, err)
	}
	if files[0].Digest != sandbox.Hash([]byte{0, 1, 255}) {
		t.Fatal("raw byte drift")
	}
}
func TestCollectBuildOutputsRejectsAmbiguousFiles(t *testing.T) {
	for _, kind := range []string{"missing", "extra", "directory", "symlink", "hardlink", "fifo", "writable", "large", "root-alias", "public-root"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(root, "app.bin")
			if err := os.WriteFile(p, []byte("good"), 0600); err != nil {
				t.Fatal(err)
			}
			c := []sandbox.OutputSpec{{Name: "app.bin", MaxBytes: 8}}
			require := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal("negative fixture setup failed", err)
				}
			}
			switch kind {
			case "missing":
				require(os.Remove(p))
			case "extra":
				require(os.WriteFile(filepath.Join(root, "other"), nil, 0600))
			case "directory":
				require(os.Remove(p))
				require(os.Mkdir(p, 0700))
			case "symlink":
				require(os.Remove(p))
				require(os.Symlink("/etc/passwd", p))
			case "hardlink":
				require(os.Link(p, filepath.Join(t.TempDir(), "alias")))
			case "fifo":
				require(os.Remove(p))
				if err := syscall.Mkfifo(p, 0600); err != nil {
					t.Fatal(err)
				}
			case "writable":
				require(os.Chmod(p, 0660))
			case "large":
				require(os.WriteFile(p, make([]byte, 9), 0600))
			case "root-alias":
				a := filepath.Join(t.TempDir(), "alias")
				require(os.Symlink(root, a))
				root = a
			case "public-root":
				require(os.Chmod(root, 0755))
			}
			if _, err := collectBuildFiles(root, c); err == nil {
				t.Fatal("unsafe artifact accepted")
			}
		})
	}
}
func TestBuildReapNeverSignalsHostProcesses(t *testing.T) {
	if os.Getpid() == 1 {
		t.Skip("not a host guard test")
	}
	if reapBuildChildren() == nil {
		t.Fatal("host kill allowed")
	}
}
