package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelayPrequalificationPackUsesOnlyNonSecretInputs(t *testing.T) {
	dir := t.TempDir()
	write := func(name, value string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	args := []string{
		"--codex-provider", "company-relay-v1",
		"--base-url", "https://relay.example.invalid/v1",
		"--auth-env-key", "COMPANY_RELAY_TOKEN",
		"--model", "relay-gpt-5.6",
		"--gateway-policy", write("gateway.txt", "gateway"),
		"--privacy-policy", write("privacy.txt", "privacy"),
		"--model-mapping", write("mapping.txt", "mapping"),
	}
	if err := relayPrequalificationPack(args); err != nil {
		t.Fatal(err)
	}
}

func TestReadRelayPolicyFileRejectsSymlink(t *testing.T) {
	if testing.Short() {
		t.Skip("filesystem symlink test")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "policy.txt")
	if err := os.WriteFile(target, []byte("policy"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "policy-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := readRelayPolicyFile(link); err == nil {
		t.Fatal("symlinked policy file accepted")
	}
}


func TestRelayPrequalificationPackAllowsExplicitPrivateHTTPWithoutCredentialUse(t *testing.T) {
	dir := t.TempDir()
	write := func(name, value string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	args := []string{
		"--codex-provider", "company-relay-v1",
		"--base-url", "http://192.168.10.100:8317/v1",
		"--allow-insecure-private-http",
		"--auth-env-key", "COMPANY_RELAY_TOKEN",
		"--model", "relay-gpt-5.6",
		"--gateway-policy", write("gateway-private-http.txt", "gateway"),
		"--privacy-policy", write("privacy-private-http.txt", "privacy"),
		"--model-mapping", write("mapping-private-http.txt", "mapping"),
	}
	if err := relayPrequalificationPack(args); err != nil {
		t.Fatal(err)
	}
}
