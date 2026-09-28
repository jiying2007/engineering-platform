package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func profileQualificationFixture(t *testing.T, binary, version, model string) codexapp.QualificationReceipt {
	t.Helper()
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	return codexapp.QualificationReceipt{
		SchemaVersion: 2, CompatibilityContractVersion: codexapp.CompatibilityContractVersion,
		CLI: "codex-cli", Version: version, BinaryDigest: canonical.BytesDigest(data),
		StableSchemaDigest:       canonical.BytesDigest([]byte("stable")),
		ExperimentalSchemaDigest: canonical.BytesDigest([]byte("experimental")),
		Transport:                "stdio", FreshProcess: true, ManagedDaemon: false, PerThreadConfigOverride: false,
		InitializePassed: true, ThreadStartPassed: true, ThreadStartModel: model,
		StableSchemaContractChecked: true, ExperimentalSurfaceChecked: true,
		CredentialSafeConfigDigest:   codexapp.CredentialSafeConfigDigest(),
		CredentialSafeProfileChecked: true,
		EngineeringConfigDigest:      codexapp.EngineeringConfigDigest(),
		EngineeringProfileChecked:    true,
	}
}

func TestBuildCodexProfileBindsExactBinaryModelAndPolicyGrant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	qualification := profileQualificationFixture(t, binary, "0.157.1", "gpt-5.6-sol")
	output, err := buildCodexProfile(binary, "gpt-5.6-sol", qualification)
	if err != nil {
		t.Fatal(err)
	}
	if output.Profile.Version != 2 ||
		output.Profile.CodexVersion != "0.157.1" ||
		output.Profile.QualificationDigest == "" ||
		output.Profile.QualificationDigest != output.QualificationDigest ||
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

func TestBuildCodexProfileRejectsQualificationDriftAndUnsafeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	wrong := filepath.Join(dir, "wrong")
	if err := os.WriteFile(wrong, []byte("#!/bin/sh\necho codex-cli 0.154.0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	wrongQualification := profileQualificationFixture(t, wrong, "0.157.1", "gpt-5.6-sol")
	if _, err := buildCodexProfile(wrong, "gpt-5.6-sol", wrongQualification); err == nil {
		t.Fatal("version drift from qualification accepted")
	}
	unsafe := filepath.Join(dir, "unsafe")
	if err := os.WriteFile(unsafe, []byte("#!/bin/sh\necho codex-cli 0.155.0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0o722); err != nil {
		t.Fatal(err)
	}
	unsafeQualification := profileQualificationFixture(t, unsafe, "0.157.1", "gpt-5.6-sol")
	if _, err := buildCodexProfile(unsafe, "gpt-5.6-sol", unsafeQualification); err == nil {
		t.Fatal("group/world-writable binary accepted")
	}
}
