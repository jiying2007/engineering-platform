package relayqualification

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestQualificationKitIsDeterministicAndCrossBound(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", contract.RequestedModel)

	first, err := BuildQualificationKit(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildQualificationKit(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	if first.ManifestDigest != second.ManifestDigest ||
		string(first.ContractJSON) != string(second.ContractJSON) ||
		string(first.CodexConfig) != string(second.CodexConfig) ||
		string(first.LiveManifestJSON) != string(second.LiveManifestJSON) ||
		string(first.RuntimeHandoffJSON) != string(second.RuntimeHandoffJSON) ||
		string(first.QualificationJSON) != string(second.QualificationJSON) ||
		string(first.ManifestJSON) != string(second.ManifestJSON) ||
		string(first.Checksums) != string(second.Checksums) {
		t.Fatal("same inputs did not produce deterministic qualification kit")
	}
	for _, required := range []string{
		KitContractFile,
		KitCodexConfigFile,
		KitLiveManifestFile,
		KitRuntimeHandoffFile,
		KitQualificationFile,
		KitManifestFile,
	} {
		if !strings.Contains(string(first.Checksums), required) {
			t.Fatalf("checksum manifest missing %s", required)
		}
	}
	if strings.Contains(string(first.CodexConfig), "secret") ||
		strings.Contains(string(first.ManifestJSON), "secret") {
		t.Fatal("qualification kit contains unexpected credential material")
	}
}

func TestQualificationKitChangesWhenRuntimeOrProviderChanges(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	firstContract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", firstContract.RequestedModel)
	first, err := BuildQualificationKit(context.Background(), firstContract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}

	secondContract := firstContract
	secondContract.StreamIdleTimeoutMS = 31000
	second, err := BuildQualificationKit(context.Background(), secondContract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	if first.ManifestDigest == second.ManifestDigest {
		t.Fatal("kit identity ignored provider configuration drift")
	}

	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.2'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildQualificationKit(context.Background(), firstContract, executable, qualification); err == nil {
		t.Fatal("kit accepted changed Codex executable")
	}
}

func TestQualificationKitManifestFileDigestsMatchBytes(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", contract.RequestedModel)
	kit, err := BuildQualificationKit(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		KitContractFile:       canonical.BytesDigest(kit.ContractJSON),
		KitCodexConfigFile:    canonical.BytesDigest(kit.CodexConfig),
		KitLiveManifestFile:   canonical.BytesDigest(kit.LiveManifestJSON),
		KitRuntimeHandoffFile: canonical.BytesDigest(kit.RuntimeHandoffJSON),
		KitQualificationFile:  canonical.BytesDigest(kit.QualificationJSON),
	}
	for _, file := range []QualificationKitFile{
		{Name: KitContractFile, Digest: want[KitContractFile]},
		{Name: KitCodexConfigFile, Digest: want[KitCodexConfigFile]},
		{Name: KitLiveManifestFile, Digest: want[KitLiveManifestFile]},
		{Name: KitRuntimeHandoffFile, Digest: want[KitRuntimeHandoffFile]},
		{Name: KitQualificationFile, Digest: want[KitQualificationFile]},
	} {
		if !strings.Contains(string(kit.ManifestJSON), file.Digest) {
			t.Fatalf("kit manifest missing digest for %s", file.Name)
		}
	}
}
