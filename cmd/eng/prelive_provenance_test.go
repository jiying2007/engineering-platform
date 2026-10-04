package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPreliveProvenanceRegression(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 required for exact pre-live provenance regression")
	}
	cmd := exec.Command(python, "-B", filepath.Join("..", "..", "scripts", "test_prelive_provenance.py"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("provenance regression: %v\n%s", err, out)
	}
}
