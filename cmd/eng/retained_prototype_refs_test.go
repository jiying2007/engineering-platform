package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRetainedPrototypeRefDriftGuard(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 is required for retained prototype ref tests")
	}
	cmd := exec.Command(python, "-B", filepath.Join("..", "..", "scripts", "test_verify_retained_prototype_refs.py"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("retained prototype ref regression: %v\n%s", err, output)
	}
}
