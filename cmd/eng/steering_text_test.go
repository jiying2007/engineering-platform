package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSteeringTextDescriptorAndPathMustMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(path, []byte("keep scope"), 0o600); err != nil {
		t.Fatal(err)
	}
	opener := func(name string) (*os.File, error) {
		// After the reader's first Lstat, replace the named regular file.
		if err := os.Rename(name, filepath.Join(dir, "original.txt")); err != nil {
			return nil, err
		}
		if err := os.WriteFile(name, []byte("keep scope"), 0o600); err != nil {
			return nil, err
		}
		return os.Open(name)
	}
	if text, err := readSteeringTextWithOpener(path, opener); err == nil || text != "" {
		t.Fatalf("same-byte different-inode steering input accepted: %q %v", text, err)
	}
}

func TestSteeringTextRejectsUnboundedOrEmptyFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	for _, payload := range []string{"", strings.Repeat("x", maxSteeringTextBytes+1)} {
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if text, err := readSteeringTextWithOpener(path, os.Open); err == nil || text != "" {
			t.Fatalf("invalid length accepted: bytes=%d err=%v", len(payload), err)
		}
	}
}
