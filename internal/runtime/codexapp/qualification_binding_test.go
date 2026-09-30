package codexapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func executableQualificationFixture(t *testing.T, executable, version, model string) QualificationReceipt {
	t.Helper()
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	return QualificationReceipt{
		SchemaVersion:                2,
		CompatibilityContractVersion: CompatibilityContractVersion,
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
		CredentialSafeConfigDigest:   CredentialSafeConfigDigest(),
		CredentialSafeProfileChecked: true,
		EngineeringConfigDigest:      EngineeringConfigDigest(),
		EngineeringProfileChecked:    true,
	}
}

func TestVerifyExecutableQualificationRebindsExactBinaryVersionAndModel(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	receipt := executableQualificationFixture(t, executable, "0.157.1", "relay-gpt-5.6")
	if err := VerifyExecutableQualification(context.Background(), executable, "relay-gpt-5.6", receipt); err != nil {
		t.Fatal(err)
	}

	if err := VerifyExecutableQualification(context.Background(), executable, "other-model", receipt); err == nil {
		t.Fatal("model drift accepted")
	}

	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.2'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutableQualification(context.Background(), executable, "relay-gpt-5.6", receipt); err == nil {
		t.Fatal("binary byte drift accepted")
	}
}

func TestVerifyExecutableQualificationRejectsVersionMismatchEvenWithMatchingBytes(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.158.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	receipt := executableQualificationFixture(t, executable, "0.157.1", "relay-gpt-5.6")
	if err := VerifyExecutableQualification(context.Background(), executable, "relay-gpt-5.6", receipt); err == nil {
		t.Fatal("version mismatch accepted")
	}
}
