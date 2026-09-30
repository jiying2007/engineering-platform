package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

func TestRelayVerifyQualificationKitAcceptsBuiltDirectory(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.157.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contract := relayqualification.Contract{
		SchemaVersion:               relayqualification.SchemaVersion,
		ProviderID:                  relayqualification.ProviderCompanyRelay,
		CodexProviderID:             "company-relay-v1",
		BaseURL:                     "https://relay.example.invalid/v1",
		WireAPI:                     relayqualification.WireAPIResponses,
		AuthMode:                    relayqualification.AuthEnvKey,
		EnvKey:                      "COMPANY_RELAY_TOKEN",
		RequestedModel:              "relay-gpt-5.6",
		RequestMaxRetries:           0,
		StreamMaxRetries:            0,
		StreamIdleTimeoutMS:         30000,
		SupportsWebSockets:          false,
		SupportsStandaloneWebSearch: false,
		GatewayPolicyDigest:         "sha256:8d8aeaa2bc3a79f95b066db3084456a98e29012a88a465a617135f4e1f843045",
		PrivacyPolicyDigest:         "sha256:9a51a5c5766532a21cc820c92d47654864d21f56a2ee8a48979ec68a06b01c47",
		ModelMappingDigest:          "sha256:8c8de70f6cb4216643e713078d44070aa9751166980692c9ee860b93fdab9f5c",
	}
	qualification := runtimeQualificationFixtureForCLI(t, executable, contract.RequestedModel)
	kit, err := relayqualification.BuildQualificationKit(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "kit")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		relayqualification.KitContractFile:       kit.ContractJSON,
		relayqualification.KitCodexConfigFile:    kit.CodexConfig,
		relayqualification.KitLiveManifestFile:   kit.LiveManifestJSON,
		relayqualification.KitRuntimeHandoffFile: kit.RuntimeHandoffJSON,
		relayqualification.KitQualificationFile:  kit.QualificationJSON,
		relayqualification.KitManifestFile:       kit.ManifestJSON,
		relayqualification.KitChecksumsFile:      kit.Checksums,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := relayVerifyQualificationKit([]string{"--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "EXTRA"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := relayVerifyQualificationKit([]string{"--dir", dir}); err == nil {
		t.Fatal("qualification kit with extra file accepted")
	}
}

func TestRelayVerifyQualificationKitRejectsSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "kit")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, relayqualification.KitContractFile)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := readRelayQualificationKitDir(dir); err == nil {
		t.Fatal("symlinked kit entry accepted")
	}
}
