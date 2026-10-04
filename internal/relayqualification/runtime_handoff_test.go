package relayqualification

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func runtimeQualificationFixture(t *testing.T, executable, version, model string) codexapp.QualificationReceipt {
	t.Helper()
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	return codexapp.QualificationReceipt{
		SchemaVersion:                3,
		CompatibilityContractVersion: codexapp.CompatibilityContractVersion,
		CLI:                          "codex-cli",
		Version:                      version,
		BinaryDigest:                 canonical.BytesDigest(data),
		StableSchemaDigest:           canonical.BytesDigest([]byte("stable")),
		ExperimentalSchemaDigest:     canonical.BytesDigest([]byte("experimental")),
		Transport:                    "stdio",
		FreshProcess:                 true,
		ManagedDaemon:                false,
		PerThreadConfigOverride:      false,
		InitializePassed:             true,
		ThreadStartPassed:            true,
		ThreadStartModel:             model,
		StableSchemaContractChecked:  true,
		ExperimentalSurfaceChecked:   true,
		CredentialSafeConfigDigest:   codexapp.CredentialSafeConfigDigest(),
		CredentialSafeProfileChecked: true,
		EngineeringConfigDigest:      codexapp.EngineeringConfigDigest(),
		EngineeringProfileChecked:    true,
		IsolationMechanism:           "linux-user-pid-namespace-init-v1",
		IsolationEnvironmentDigest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		IsolatedEngineeringStartup:   true,
		NamespaceInitReaped:          true,
	}
}

func TestBuildRuntimeHandoffBindsLiveManifestAndExactCodexQualification(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", contract.RequestedModel)

	first, err := BuildRuntimeHandoff(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildRuntimeHandoff(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.HandoffDigest == "" {
		t.Fatalf("runtime handoff is not deterministic: %#v %#v", first, second)
	}
	if first.Handoff.CodexBinaryDigest != qualification.BinaryDigest ||
		first.Handoff.CodexVersion != qualification.Version ||
		first.Handoff.RequestedModel != contract.RequestedModel ||
		first.Handoff.LiveManifestDigest == "" ||
		first.Handoff.ProviderConfigDigest == "" ||
		first.Handoff.CodexQualificationDigest == "" {
		t.Fatalf("runtime handoff missing exact identity: %#v", first)
	}
}

func TestRuntimeHandoffRejectsQualificationOrBinaryDrift(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", contract.RequestedModel)

	drifted := qualification
	drifted.ThreadStartModel = "other-model"
	if _, err := BuildRuntimeHandoff(context.Background(), contract, executable, drifted); err == nil {
		t.Fatal("qualification model drift accepted")
	}

	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.2'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRuntimeHandoff(context.Background(), contract, executable, qualification); err == nil {
		t.Fatal("Codex executable drift accepted")
	}
}
