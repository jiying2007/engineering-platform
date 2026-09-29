package relayqualification

import "github.com/jiying2007/engineering-platform/internal/canonical"

type BuildInput struct {
	CodexProviderID     string
	BaseURL             string
	AllowInsecurePrivateHTTP bool
	AuthMode            string
	EnvKey              string
	AuthCommand         string
	RequestedModel      string
	StreamIdleTimeoutMS int
	GatewayPolicy       []byte
	PrivacyPolicy       []byte
	ModelMapping        []byte
}

type Pack struct {
	Contract   Contract   `json:"contract"`
	Assessment Assessment `json:"assessment"`
}

func BuildPack(input BuildInput) (Pack, error) {
	contract := Contract{
		SchemaVersion:               SchemaVersion,
		ProviderID:                  ProviderCompanyRelay,
		CodexProviderID:             input.CodexProviderID,
		BaseURL:                     input.BaseURL,
		AllowInsecurePrivateHTTP:    input.AllowInsecurePrivateHTTP,
		WireAPI:                     WireAPIResponses,
		AuthMode:                    input.AuthMode,
		EnvKey:                      input.EnvKey,
		AuthCommand:                 input.AuthCommand,
		RequestedModel:              input.RequestedModel,
		RequestMaxRetries:           0,
		StreamMaxRetries:            0,
		StreamIdleTimeoutMS:         input.StreamIdleTimeoutMS,
		SupportsWebSockets:          false,
		SupportsStandaloneWebSearch: false,
		GatewayPolicyDigest:         canonical.BytesDigest(input.GatewayPolicy),
		PrivacyPolicyDigest:         canonical.BytesDigest(input.PrivacyPolicy),
		ModelMappingDigest:          canonical.BytesDigest(input.ModelMapping),
	}
	assessment, err := Evaluate(contract)
	if err != nil {
		return Pack{}, err
	}
	return Pack{Contract: contract, Assessment: assessment}, nil
}
