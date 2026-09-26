package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func TestBuildCodexProfileBindsExactBinaryModelAndPolicyGrant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.155.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := buildCodexProfile(binary, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if output.Profile.Version != 1 ||
		output.Profile.CodexVersion != codexapp.QualifiedCodexVersion ||
		output.Profile.Model != "gpt-5.6-sol" ||
		output.Profile.Sandbox != "workspace-write" ||
		output.Profile.ApprovalPolicy != "never" ||
		output.Profile.ToolNetwork ||
		output.ProfileDigest == "" ||
		output.ToolProfile != "codex/"+output.ProfileDigest ||
		output.ActionGrant.Action != codexexec.Action ||
		output.ActionGrant.RiskClass != "CONTROLLED_MUTATION" ||
		output.ActionGrant.Capability != output.ProfileDigest {
		t.Fatalf("unexpected profile output: %#v", output)
	}
}

func TestBuildCodexProfileRejectsWrongVersionAndUnsafeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	wrong := filepath.Join(dir, "wrong")
	if err := os.WriteFile(wrong, []byte("#!/bin/sh\necho codex-cli 0.154.0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := buildCodexProfile(wrong, "gpt-5.6-sol"); err == nil {
		t.Fatal("wrong Codex version accepted")
	}
	unsafe := filepath.Join(dir, "unsafe")
	if err := os.WriteFile(unsafe, []byte("#!/bin/sh\necho codex-cli 0.155.0\n"), 0o722); err != nil {
		t.Fatal(err)
	}
	if _, err := buildCodexProfile(unsafe, "gpt-5.6-sol"); err == nil {
		t.Fatal("group/world-writable binary accepted")
	}
}
