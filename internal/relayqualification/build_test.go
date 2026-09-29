package relayqualification

import "testing"

func TestBuildPackDerivesPolicyDigestsAndNegativeAuthority(t *testing.T) {
	input := BuildInput{
		CodexProviderID:     "company-relay-v1",
		BaseURL:             "https://relay.example.invalid/v1",
		AuthMode:            AuthEnvKey,
		EnvKey:              "COMPANY_RELAY_TOKEN",
		RequestedModel:      "relay-gpt-5.6",
		StreamIdleTimeoutMS: 30000,
		GatewayPolicy:       []byte("gateway-policy-v1"),
		PrivacyPolicy:       []byte("privacy-policy-v1"),
		ModelMapping:        []byte("model-map-v1"),
	}
	first, err := BuildPack(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPack(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Contract != second.Contract || first.Assessment != second.Assessment {
		t.Fatal("same non-secret inputs did not produce the same prequalification pack")
	}
	if first.Contract.GatewayPolicyDigest == "" ||
		first.Contract.PrivacyPolicyDigest == "" ||
		first.Contract.ModelMappingDigest == "" {
		t.Fatalf("policy digests not derived: %#v", first.Contract)
	}
	if first.Assessment.ProviderConfigDigest == "" ||
		first.Assessment.AccountVerified ||
		first.Assessment.LiveModelTurnExecuted ||
		first.Assessment.ProviderAdmitted {
		t.Fatalf("operator pack granted authority: %#v", first.Assessment)
	}
}

func TestBuildPackRejectsInvalidAuthSelection(t *testing.T) {
	base := BuildInput{
		CodexProviderID:     "company-relay-v1",
		BaseURL:             "https://relay.example.invalid/v1",
		RequestedModel:      "relay-gpt-5.6",
		StreamIdleTimeoutMS: 30000,
		GatewayPolicy:       []byte("gateway"),
		PrivacyPolicy:       []byte("privacy"),
		ModelMapping:        []byte("mapping"),
	}
	base.AuthMode = AuthEnvKey
	base.EnvKey = "OPENAI_API_KEY"
	if _, err := BuildPack(base); err == nil {
		t.Fatal("reserved credential locator accepted")
	}
	base.EnvKey = ""
	base.AuthMode = AuthCommandToken
	base.AuthCommand = "relative-token-command"
	if _, err := BuildPack(base); err == nil {
		t.Fatal("relative token command accepted")
	}
}
