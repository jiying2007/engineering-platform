package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

func TestRelayRenderCodexConfigCreatesOwnerPrivateFile(t *testing.T) {
	dir := t.TempDir()
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
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(dir, "contract.json")
	if err := os.WriteFile(contractPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "relay.config.toml")
	if err := relayRenderCodexConfig([]string{"--contract", contractPath, "--out", out}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("unexpected rendered config mode: %v", info.Mode())
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := contract.RenderCodexConfig()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(expected) {
		t.Fatalf("rendered bytes differ:\n%s\nwant:\n%s", got, expected)
	}
	if err := relayRenderCodexConfig([]string{"--contract", contractPath, "--out", out}); err == nil {
		t.Fatal("existing config file was overwritten")
	}
}
