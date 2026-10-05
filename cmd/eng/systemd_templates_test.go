package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSystemdServiceTemplateContract(t *testing.T) {
	cmd := exec.Command("python3", "-B", filepath.Join("..", "..", "scripts", "test_systemd_lifecycle.py"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("systemd template regression: %v\n%s", err, out)
	}
}
