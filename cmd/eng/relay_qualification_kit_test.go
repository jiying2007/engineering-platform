package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/relayqualification"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func TestRelayQualificationKitCreatesCompleteOwnerPrivateBundle(t *testing.T) {
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
		GatewayPolicyDigest:         canonical.BytesDigest([]byte("gateway")),
		PrivacyPolicyDigest:         canonical.BytesDigest([]byte("privacy")),
		ModelMappingDigest:          canonical.BytesDigest([]byte("mapping")),
	}
	contractBytes, _ := json.Marshal(contract)
	contractPath := filepath.Join(root, "contract.json")
	if err := os.WriteFile(contractPath, contractBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	binaryBytes, _ := os.ReadFile(executable)
	qualification := codexapp.QualificationReceipt{
		SchemaVersion:                3,
		CompatibilityContractVersion: codexapp.CompatibilityContractVersion,
		CLI:                          "codex-cli",
		Version:                      "0.157.1",
		BinaryDigest:                 canonical.BytesDigest(binaryBytes),
		StableSchemaDigest:           canonical.BytesDigest([]byte("stable")),
		ExperimentalSchemaDigest:     canonical.BytesDigest([]byte("experimental")),
		Transport:                    "stdio",
		FreshProcess:                 true,
		ManagedDaemon:                false,
		PerThreadConfigOverride:      false,
		InitializePassed:             true,
		ThreadStartPassed:            true,
		ThreadStartModel:             contract.RequestedModel,
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
	qualificationBytes, _ := json.Marshal(qualification)
	qualificationPath := filepath.Join(root, "qualification.json")
	if err := os.WriteFile(qualificationPath, qualificationBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "kit")
	if err := relayQualificationKit([]string{
		"--contract", contractPath,
		"--codex", executable,
		"--qualification", qualificationPath,
		"--out-dir", out,
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(out)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("unexpected kit directory: %v %v", info, err)
	}
	for _, name := range []string{
		relayqualification.KitContractFile,
		relayqualification.KitCodexConfigFile,
		relayqualification.KitLiveManifestFile,
		relayqualification.KitRuntimeHandoffFile,
		relayqualification.KitQualificationFile,
		relayqualification.KitManifestFile,
		relayqualification.KitChecksumsFile,
	} {
		fileInfo, err := os.Lstat(filepath.Join(out, name))
		if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("unexpected kit file %s: %v %v", name, fileInfo, err)
		}
	}
	if err := relayQualificationKit([]string{
		"--contract", contractPath,
		"--codex", executable,
		"--qualification", qualificationPath,
		"--out-dir", out,
	}); err == nil {
		t.Fatal("existing kit directory was overwritten")
	}
}
