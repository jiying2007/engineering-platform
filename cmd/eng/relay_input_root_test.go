package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestRelayRootInputAnchoredAfterDirectoryReplacement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux anchored no-follow input qualification")
	}
	parent := t.TempDir()
	kitPath := filepath.Join(parent, "kit")
	movedPath := filepath.Join(parent, "original")
	if err := os.Mkdir(kitPath, 0o700); err != nil {
		t.Fatal(err)
	}
	member := "manifest.json"
	if err := os.WriteFile(filepath.Join(kitPath, member), []byte("original-byte-identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(kitPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	// The path now denotes a different valid owner-private directory. An
	// already-open root must continue reading its original inode, not the
	// replacement's internally consistent-looking contents.
	if err := os.Rename(kitPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(kitPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kitPath, member), []byte("replacement-byte-identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readRelayRootInput(root, member, 64<<10, true)
	if err != nil || !bytes.Equal(got, []byte("original-byte-identity")) {
		t.Fatalf("input escaped anchored original directory: %q, %v", got, err)
	}
	if _, err := readRelayRootInput(root, "../kit/"+member, 64<<10, true); err == nil {
		t.Fatal("parent traversal accepted")
	}
	if err := os.Symlink(member, filepath.Join(movedPath, "symlink")); err != nil {
		t.Fatal(err)
	}
	if _, err := readRelayRootInput(root, "symlink", 64<<10, true); err == nil {
		t.Fatal("symlinked kit entry accepted")
	}
	if err := syscall.Mkfifo(filepath.Join(movedPath, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRelayRootInput(root, "fifo", 64<<10, true); err == nil {
		t.Fatal("FIFO kit entry accepted")
	}
}
