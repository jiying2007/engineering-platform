package relayqualification

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func validContract() Contract {
	return Contract{
		SchemaVersion:               SchemaVersion,
		ProviderID:                  ProviderCompanyRelay,
		CodexProviderID:             "company-relay-v1",
		BaseURL:                     "https://relay.example.invalid/v1",
		WireAPI:                     WireAPIResponses,
		AuthMode:                    AuthEnvKey,
		EnvKey:                      "COMPANY_RELAY_TOKEN",
		RequestedModel:              "relay-gpt-5.6",
		RequestMaxRetries:           0,
		StreamMaxRetries:            0,
		StreamIdleTimeoutMS:         30000,
		SupportsWebSockets:          false,
		SupportsStandaloneWebSearch: false,
		GatewayPolicyDigest:         canonical.BytesDigest([]byte("gateway-policy")),
		PrivacyPolicyDigest:         canonical.BytesDigest([]byte("privacy-policy")),
		ModelMappingDigest:          canonical.BytesDigest([]byte("model-mapping")),
	}
}

func TestEvaluateProducesNonAdmittingDeterministicAssessment(t *testing.T) {
	contract := validContract()
	first, err := Evaluate(contract)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(contract)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.ProviderConfigDigest == "" {
		t.Fatalf("assessment is not deterministic: %#v %#v", first, second)
	}
	if first.Stage != StageRepositoryPrequalification ||
		first.AccountVerified ||
		first.LiveModelTurnExecuted ||
		first.ProviderAdmitted ||
		first.NextGate != NextGateLiveReadOnlyNoTool {
		t.Fatalf("prequalification incorrectly grants live authority: %#v", first)
	}
}

func TestContractRejectsUnsafeOrAmbiguousProviderConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Contract)
	}{
		{"reserved-provider-id", func(c *Contract) { c.CodexProviderID = "openai" }},
		{"http-endpoint", func(c *Contract) { c.BaseURL = "http://relay.example.invalid/v1" }},
		{"endpoint-query", func(c *Contract) { c.BaseURL = "https://relay.example.invalid/v1?route=x" }},
		{"wrong-wire-api", func(c *Contract) { c.WireAPI = "chat-completions" }},
		{"openai-env-reuse", func(c *Contract) { c.EnvKey = "OPENAI_API_KEY" }},
		{"request-retry", func(c *Contract) { c.RequestMaxRetries = 1 }},
		{"stream-retry", func(c *Contract) { c.StreamMaxRetries = 1 }},
		{"websocket", func(c *Contract) { c.SupportsWebSockets = true }},
		{"web-search", func(c *Contract) { c.SupportsStandaloneWebSearch = true }},
		{"missing-policy-digest", func(c *Contract) { c.PrivacyPolicyDigest = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contract := validContract()
			tc.mutate(&contract)
			if err := contract.Validate(); err == nil {
				t.Fatalf("unsafe relay contract accepted: %#v", contract)
			}
		})
	}
}

func TestCommandTokenContractUsesAbsoluteDedicatedCommand(t *testing.T) {
	contract := validContract()
	contract.AuthMode = AuthCommandToken
	contract.EnvKey = ""
	contract.AuthCommand = "/usr/local/bin/company-relay-token"
	if err := contract.Validate(); err != nil {
		t.Fatal(err)
	}
	contract.AuthCommand = "company-relay-token"
	if err := contract.Validate(); err == nil {
		t.Fatal("relative token command accepted")
	}
}
