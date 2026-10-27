//go:build linux

package access

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestConfigurationOpenRejectsUnwrittenFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadConfiguration(path, true)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO was accepted as a regular configuration file")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("configuration read blocked on FIFO without writer")
	}
}

func TestConfigurationOpenPreservesHostOwnedRegularSymlinkAndPermissions(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "actual.pem")
	alias := filepath.Join(root, "current.pem")
	if err := os.WriteFile(file, []byte("config-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, alias); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadConfiguration(alias, true); err != nil || string(data) != "config-data" {
		t.Fatalf("valid host-controlled configuration alias rejected: %q %v", data, err)
	}
	if err := os.Chmod(file, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfiguration(alias, true); err == nil {
		t.Fatal("secret file group/other readability was accepted")
	}
	if data, err := ReadConfiguration(alias, false); err != nil || string(data) != "config-data" {
		t.Fatalf("public configuration with safe permissions rejected: %q %v", data, err)
	}
}

func TestConfigurationOpenRejectsSymlinkToFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, "source.fifo")
	alias := filepath.Join(root, "config")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fifo, alias); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadConfiguration(alias, false)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO target accepted through symbolic link")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("configuration symlink to FIFO blocked")
	}
}
