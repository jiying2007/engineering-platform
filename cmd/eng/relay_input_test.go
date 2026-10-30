//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRelayBoundedInputRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contract")
	expected := []byte("non-secret-policy")
	if err := os.WriteFile(path, expected, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readRelayInput(path, 64<<10, false)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("regular input: %v %q", err, got)
	}
	got, err = readRelayInput(path, 64<<10, true)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("private input: %v", err)
	}
	if _, err = readRelayInput(path, 4, false); err == nil {
		t.Fatal("oversized source accepted")
	}
	if err = os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = readRelayInput(path, 64<<10, true); err == nil {
		t.Fatal("nonprivate kit input accepted")
	}
	if err = os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	sym := filepath.Join(dir, "symlink")
	if err = os.Symlink(path, sym); err != nil {
		t.Fatal(err)
	}
	if _, err = readRelayInput(sym, 64<<10, false); err == nil {
		t.Fatal("symlink accepted")
	}
	if file, err := openSteeringFile(sym); err == nil {
		file.Close()
		t.Fatal("nofollow opener followed symlink")
	}
	fifo := filepath.Join(dir, "fifo")
	if err = syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	// A nonblocking opener must return immediately even if a previously
	// checked regular path has been swapped to a FIFO by another process.
	file, err := openSteeringFile(fifo)
	if err == nil {
		stat, serr := file.Stat()
		_ = file.Close()
		if serr != nil || stat.Mode().IsRegular() {
			t.Fatal("FIFO was mistaken for a regular file", serr)
		}
	}
	if _, err = readRelayInput(fifo, 64<<10, false); err == nil {
		t.Fatal("FIFO accepted")
	}
}
