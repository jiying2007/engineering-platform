//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/production"
)

func TestProductionSLOV2UsesDescriptorBoundReader(t *testing.T) {
	_, legacyPath, _ := operationalCLIFixture(t, false)
	if err := productionSLOReport([]string{"--observations", legacyPath}); err != nil {
		t.Fatalf("existing v2 input rejected: %v", err)
	}

	// V2 and V3 now share the descriptor-bound, no-follow/nonblocking reader.
	data, err := production.ReadOperationalSLOManifest(legacyPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("regular v2 file failed safe opening: %v", err)
	}
	link := filepath.Join(t.TempDir(), "legacy-link.json")
	if err := os.Symlink(legacyPath, link); err != nil {
		t.Fatal(err)
	}
	if err := productionSLOReport([]string{"--observations", link}); err == nil {
		t.Fatal("v2 symlink accepted")
	}
	fifo := filepath.Join(t.TempDir(), "replaced-fifo.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := productionSLOReport([]string{"--observations", fifo}); err == nil {
		t.Fatal("v2 FIFO accepted")
	}
}
