package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The actual seven-file kit is verified in
// TestRelayVerifyQualificationKitAcceptsBuiltDirectory. This test guards
// against accidentally replacing bounded enumeration with ReadDir(-1).
func TestRelayKitDirectoryEnumerationIsBounded(t *testing.T) {
	for _, count := range []int{7, 8, 257} {
		t.Run(fmt.Sprintf("members_%d", count), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "kit")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < count; i++ {
				path := filepath.Join(dir, fmt.Sprintf("member-%04d", i))
				if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			fd, err := os.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			entries, readErr := readBoundedRelayKitEntries(fd)
			closeErr := fd.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("bounded directory read: %v close=%v", readErr, closeErr)
			}
			want := count
			if want > 8 {
				want = 8
			}
			if len(entries) != want {
				t.Fatalf("expected at most first eight members: got %d, want %d", len(entries), want)
			}
			if count > 7 {
				if _, err := readRelayQualificationKitDir(dir); err == nil {
					t.Fatal("oversized qualification kit directory accepted")
				}
			}
		})
	}
}
