package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOptionalPrivateToken(t *testing.T) {
	if token, err := optionalPrivateToken(""); err != nil || token != "" {
		t.Fatalf("empty optional token failed: %q %v", token, err)
	}
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("ghs_fixture_token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := optionalPrivateToken(path)
	if err != nil || token != "ghs_fixture_token" {
		t.Fatalf("private token rejected: %q %v", token, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := optionalPrivateToken(path); err == nil {
		t.Fatal("world-readable GitHub token accepted")
	}
}

func TestOptionalPrivateTokenRejectsDirectory(t *testing.T) {
	if _, err := optionalPrivateToken(t.TempDir()); err == nil {
		t.Fatal("directory accepted as GitHub token file")
	}
}
