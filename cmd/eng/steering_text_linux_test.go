//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSteeringTextLinuxSafeFileOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("你好小窝"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := readSteeringText(path)
	if err != nil || text != "你好小窝" {
		t.Fatalf("valid UTF-8 steering text rejected: %q %v", text, err)
	}
}

func TestSteeringTextLinuxRaceDoesNotWaitForFIFOWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "steering.txt")
	if err := os.WriteFile(path, []byte("do not repeat"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := readSteeringTextWithOpener(path, func(name string) (*os.File, error) {
			if err := os.Rename(name, filepath.Join(dir, "original.txt")); err != nil {
				return nil, err
			}
			if err := syscall.Mkfifo(name, 0o600); err != nil {
				return nil, err
			}
			return openSteeringFile(name)
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO substitution accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steering input blocked on substituted FIFO")
	}
}

func TestSteeringTextLinuxRaceRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "steering.txt")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("safe text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("safe text"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readSteeringTextWithOpener(path, func(name string) (*os.File, error) {
		if err := os.Remove(name); err != nil {
			return nil, err
		}
		if err := os.Symlink(target, name); err != nil {
			return nil, err
		}
		return openSteeringFile(name)
	})
	if err == nil || got != "" {
		t.Fatalf("symlink substituted after Lstat: got=%q err=%v", got, err)
	}
}
