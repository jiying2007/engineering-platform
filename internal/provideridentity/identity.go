package provideridentity

import (
	"fmt"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const (
	Version1 = 1

	ProviderOpenAICodex = "openai-codex"

	CredentialChatGPTSession    = "chatgpt-session"
	CredentialWorkloadIdentity  = "workload-identity"

	ExecutionTrustedSelfHosted = "trusted-self-hosted"
	ExecutionUnattended        = "unattended"
)

const openAICodexProviderContract = "version=1\nprovider=openai-codex\ntransport=codex-app-server\nendpoint=builtin\n"

type Identity struct {
	Version              int    `json:"version"`
	ProviderID           string `json:"provider_id"`
	CredentialMode       string `json:"credential_mode"`
	ExecutionMode        string `json:"execution_mode"`
	ProviderConfigDigest string `json:"provider_config_digest"`
}

func OpenAICodexConfigDigest() string {
	return canonical.BytesDigest([]byte(openAICodexProviderContract))
}

func New(providerID, credentialMode, executionMode string) (Identity, error) {
	identity := Identity{
		Version:              Version1,
		ProviderID:           providerID,
		CredentialMode:       credentialMode,
		ExecutionMode:        executionMode,
		ProviderConfigDigest: providerConfigDigest(providerID),
	}
	if err := identity.Validate(); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func OpenAIChatGPTTrustedSelfHosted() Identity {
	identity, err := New(ProviderOpenAICodex, CredentialChatGPTSession, ExecutionTrustedSelfHosted)
	if err != nil {
		panic(err)
	}
	return identity
}

func OpenAIWIFUnattended() Identity {
	identity, err := New(ProviderOpenAICodex, CredentialWorkloadIdentity, ExecutionUnattended)
	if err != nil {
		panic(err)
	}
	return identity
}

func (i Identity) Validate() error {
	if i.Version != Version1 ||
		strings.TrimSpace(i.ProviderID) != i.ProviderID || i.ProviderID == "" || len(i.ProviderID) > 128 ||
		strings.TrimSpace(i.CredentialMode) != i.CredentialMode || i.CredentialMode == "" || len(i.CredentialMode) > 128 ||
		strings.TrimSpace(i.ExecutionMode) != i.ExecutionMode || i.ExecutionMode == "" || len(i.ExecutionMode) > 128 ||
		!canonical.ValidDigest(i.ProviderConfigDigest) {
		return fmt.Errorf("invalid provider identity")
	}
	if i.ProviderID != ProviderOpenAICodex || i.ProviderConfigDigest != OpenAICodexConfigDigest() {
		return fmt.Errorf("provider is not qualified for Codex execution")
	}
	switch {
	case i.CredentialMode == CredentialChatGPTSession && i.ExecutionMode == ExecutionTrustedSelfHosted:
		return nil
	case i.CredentialMode == CredentialWorkloadIdentity && i.ExecutionMode == ExecutionUnattended:
		return nil
	default:
		return fmt.Errorf("provider credential/execution combination is not admitted")
	}
}

func providerConfigDigest(providerID string) string {
	switch providerID {
	case ProviderOpenAICodex:
		return OpenAICodexConfigDigest()
	default:
		return ""
	}
}
