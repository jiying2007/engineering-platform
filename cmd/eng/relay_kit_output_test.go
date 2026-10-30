package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// No external Codex/provider is needed to exercise the directory and member
// writer. A mutable pathname must never redirect output or deletion.
func TestRelayQualificationKitOutputPinnedToOriginalDirectory(t *testing.T) {
	parent := t.TempDir()
	out := filepath.Join(parent, "kit")
	moved := filepath.Join(parent, "moved-original")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(out)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := relayKitOutputDirectoryUnchanged(root, out, before); err != nil {
		t.Fatal(err)
	}
	if err := writeRelayKitFile(root, "relay-contract-v2.json", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(out, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(out, "DO_NOT_DELETE")
	if err := os.WriteFile(sentinel, []byte("retain original replacement data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := relayKitOutputDirectoryUnchanged(root, out, before); err == nil {
		t.Fatal("accepted replaced output directory")
	}
	if err := writeRelayKitFile(root, "relay-live-manifest.json", []byte("second-original")); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(moved, "relay-live-manifest.json"))
	if err != nil || !bytes.Equal(original, []byte("second-original")) {
		t.Fatalf("anchored write misdirected: %q, %v", original, err)
	}
	if _, err := os.Stat(filepath.Join(out, "relay-live-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("wrote to replacement directory: %v", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "retain original replacement data" {
		t.Fatalf("replacement directory damaged: %q %v", got, err)
	}
	if err := relayKitOutputDirectoryUnchanged(root, out, before); err == nil {
		t.Fatal("final output identity accepted changed path")
	}
}

func TestRelayQualificationKitWriterRequiresExactPrivateMember(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "kit")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := []byte("known finite nonsecret bytes")
	if err := writeRelayKitFile(root, "relay-contract-v2.json", data); err != nil {
		t.Fatal(err)
	}
	f, err := os.Lstat(filepath.Join(dir, "relay-contract-v2.json"))
	if err != nil || !f.Mode().IsRegular() || f.Mode().Perm() != 0o600 || f.Size() != int64(len(data)) {
		t.Fatalf("wrong private output: %v %v", f, err)
	}
	if err := writeRelayKitFile(root, "relay-contract-v2.json", data); err == nil {
		t.Fatal("exclusive file overwritten")
	}
	for _, name := range []string{"../outside", ".", "", "sub/child"} {
		if err := writeRelayKitFile(root, name, data); err == nil {
			t.Fatalf("invalid output member %q accepted", name)
		}
	}
	if err := writeRelayKitFile(root, "oversize", make([]byte, 2<<20+1)); err == nil {
		t.Fatal("oversize output accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "outside")); !os.IsNotExist(err) {
		t.Fatalf("parent traversal created file: %v", err)
	}
	if err := os.Symlink("relay-contract-v2.json", filepath.Join(dir, "symlink")); err != nil {
		t.Fatal(err)
	}
	if err := writeRelayKitFile(root, "symlink", data); err == nil {
		t.Fatal("symlink existing member overwritten")
	}
}
