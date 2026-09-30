package relayqualification

import (
	"os"
	"path/filepath"
	"testing"
)

func runtimeTestExecutable(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root, executable
}
