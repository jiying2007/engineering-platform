package provideridentity

import "testing"

func TestAdmittedProviderIdentities(t *testing.T) {
	for _, identity := range []Identity{
		OpenAIChatGPTTrustedSelfHosted(),
		OpenAIWIFUnattended(),
	} {
		if err := identity.Validate(); err != nil {
			t.Fatal(err)
		}
		if identity.ProviderID != ProviderOpenAICodex || identity.ProviderConfigDigest != OpenAICodexConfigDigest() {
			t.Fatalf("unexpected provider identity: %#v", identity)
		}
	}
}

func TestProviderIdentityRejectsCrossLaneAndUnknownProvider(t *testing.T) {
	cases := []struct {
		name string
		id   Identity
	}{
		{
			name: "saved-login-unattended",
			id: Identity{Version: Version1, ProviderID: ProviderOpenAICodex, CredentialMode: CredentialChatGPTSession, ExecutionMode: ExecutionUnattended, ProviderConfigDigest: OpenAICodexConfigDigest()},
		},
		{
			name: "wif-trusted-self-hosted",
			id: Identity{Version: Version1, ProviderID: ProviderOpenAICodex, CredentialMode: CredentialWorkloadIdentity, ExecutionMode: ExecutionTrustedSelfHosted, ProviderConfigDigest: OpenAICodexConfigDigest()},
		},
		{
			name: "unqualified-relay",
			id: Identity{Version: Version1, ProviderID: "company-relay", CredentialMode: "command-token", ExecutionMode: ExecutionUnattended, ProviderConfigDigest: OpenAICodexConfigDigest()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.id.Validate(); err == nil {
				t.Fatalf("unqualified provider identity accepted: %#v", tc.id)
			}
		})
	}
}
