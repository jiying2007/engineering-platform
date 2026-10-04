package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntegratedBranchRetirementSafety(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 is required for branch retirement safety tests")
	}
	cmd := exec.Command(python, "-B", filepath.Join("..", "..", "scripts", "test_retire_integrated_branches.py"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("branch retirement regression: %v\n%s", err, output)
	}
}
