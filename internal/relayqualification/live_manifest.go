package relayqualification

import (
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const (
	LiveManifestSchemaVersion = 1
	LiveReceiptSchemaVersion  = 1

	LiveProbePrompt   = "Reply with only: engineering-platform relay qualification"
	LiveProbeExpected = "engineering-platform relay qualification"
)

type LiveManifest struct {
	SchemaVersion              int    `json:"schema_version"`
	ProviderConfigDigest       string `json:"provider_config_digest"`
	ContractDigest             string `json:"contract_digest"`
	RendererContractVersion    int    `json:"renderer_contract_version"`
	CodexConfigDigest          string `json:"codex_config_digest"`
	ProviderID                 string `json:"provider_id"`
	CodexProviderID            string `json:"codex_provider_id"`
	RequestedModel             string `json:"requested_model"`
	PromptDigest               string `json:"prompt_digest"`
	ExpectedOutputDigest       string `json:"expected_output_digest"`
	MaxModelTurns              int    `json:"max_model_turns"`
	ReadOnly                   bool   `json:"read_only"`
	ApprovalPolicy             string `json:"approval_policy"`
	AllowTools                 bool   `json:"allow_tools"`
	ToolNetwork                bool   `json:"tool_network"`
	AllowWebSearch             bool   `json:"allow_web_search"`
	RequestMaxRetries          int    `json:"request_max_retries"`
	StreamMaxRetries           int    `json:"stream_max_retries"`
	MaxOutputBytes             int    `json:"max_output_bytes"`
	MaxWallTimeMS              int    `json:"max_wall_time_ms"`
	RequireGatewayOperationID  bool   `json:"require_gateway_operation_id"`
	RequireEffectiveProvider   bool   `json:"require_effective_provider"`
	RequireEffectiveModel      bool   `json:"require_effective_model"`
	GatewayPolicyDigest        string `json:"gateway_policy_digest"`
	PrivacyPolicyDigest        string `json:"privacy_policy_digest"`
	ModelMappingDigest         string `json:"model_mapping_digest"`
	FutureReceiptSchemaVersion int    `json:"future_receipt_schema_version"`
}

type LiveManifestEnvelope struct {
	Manifest       LiveManifest `json:"manifest"`
	ManifestDigest string       `json:"manifest_digest"`
}

func BuildLiveManifest(c Contract) (LiveManifestEnvelope, error) {
	assessment, err := Evaluate(c)
	if err != nil {
		return LiveManifestEnvelope{}, err
	}
	manifest := LiveManifest{
		SchemaVersion:              LiveManifestSchemaVersion,
		ProviderConfigDigest:       assessment.ProviderConfigDigest,
		ContractDigest:             assessment.ContractDigest,
		RendererContractVersion:    assessment.RendererContractVersion,
		CodexConfigDigest:          assessment.CodexConfigDigest,
		ProviderID:                 c.ProviderID,
		CodexProviderID:            c.CodexProviderID,
		RequestedModel:             c.RequestedModel,
		PromptDigest:               canonical.BytesDigest([]byte(LiveProbePrompt)),
		ExpectedOutputDigest:       canonical.BytesDigest([]byte(LiveProbeExpected)),
		MaxModelTurns:              1,
		ReadOnly:                   true,
		ApprovalPolicy:             "never",
		AllowTools:                 false,
		ToolNetwork:                false,
		AllowWebSearch:             false,
		RequestMaxRetries:          0,
		StreamMaxRetries:           0,
		MaxOutputBytes:             64 << 10,
		MaxWallTimeMS:              90000,
		RequireGatewayOperationID:  true,
		RequireEffectiveProvider:   true,
		RequireEffectiveModel:      true,
		GatewayPolicyDigest:        c.GatewayPolicyDigest,
		PrivacyPolicyDigest:        c.PrivacyPolicyDigest,
		ModelMappingDigest:         c.ModelMappingDigest,
		FutureReceiptSchemaVersion: LiveReceiptSchemaVersion,
	}
	if err := manifest.Validate(); err != nil {
		return LiveManifestEnvelope{}, err
	}
	digest, err := manifest.Digest()
	if err != nil {
		return LiveManifestEnvelope{}, err
	}
	return LiveManifestEnvelope{Manifest: manifest, ManifestDigest: digest}, nil
}

func (m LiveManifest) Validate() error {
	if m.SchemaVersion != LiveManifestSchemaVersion ||
		!canonical.ValidDigest(m.ProviderConfigDigest) ||
		!canonical.ValidDigest(m.ContractDigest) ||
		m.RendererContractVersion != RendererContractVersion ||
		!canonical.ValidDigest(m.CodexConfigDigest) ||
		m.ProviderID != ProviderCompanyRelay ||
		!providerIDPattern.MatchString(m.CodexProviderID) ||
		!validText(m.RequestedModel, 128) ||
		m.PromptDigest != canonical.BytesDigest([]byte(LiveProbePrompt)) ||
		m.ExpectedOutputDigest != canonical.BytesDigest([]byte(LiveProbeExpected)) ||
		m.MaxModelTurns != 1 ||
		!m.ReadOnly ||
		m.ApprovalPolicy != "never" ||
		m.AllowTools ||
		m.ToolNetwork ||
		m.AllowWebSearch ||
		m.RequestMaxRetries != 0 ||
		m.StreamMaxRetries != 0 ||
		m.MaxOutputBytes != 64<<10 ||
		m.MaxWallTimeMS != 90000 ||
		!m.RequireGatewayOperationID ||
		!m.RequireEffectiveProvider ||
		!m.RequireEffectiveModel ||
		!canonical.ValidDigest(m.GatewayPolicyDigest) ||
		!canonical.ValidDigest(m.PrivacyPolicyDigest) ||
		!canonical.ValidDigest(m.ModelMappingDigest) ||
		m.FutureReceiptSchemaVersion != LiveReceiptSchemaVersion {
		return fmt.Errorf("invalid relay live qualification manifest")
	}
	return nil
}

func (m LiveManifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(m)
}

type LiveReceiptContract struct {
	SchemaVersion         int    `json:"schema_version"`
	ManifestDigest        string `json:"manifest_digest"`
	ProviderConfigDigest  string `json:"provider_config_digest"`
	GatewayOperationID    string `json:"gateway_operation_id"`
	RequestedModel        string `json:"requested_model"`
	EffectiveProvider     string `json:"effective_provider"`
	EffectiveModel        string `json:"effective_model"`
	PromptDigest          string `json:"prompt_digest"`
	TurnStatus            string `json:"turn_status"`
	Output                string `json:"output"`
	OutputDigest          string `json:"output_digest"`
	ModelTurns            int    `json:"model_turns"`
	ApprovalRequests      int    `json:"approval_requests"`
	UnexpectedToolUse     bool   `json:"unexpected_tool_use"`
	RequestRetries        int    `json:"request_retries"`
	StreamRetries         int    `json:"stream_retries"`
	CredentialAccepted    bool   `json:"credential_accepted"`
	LiveModelTurnExecuted bool   `json:"live_model_turn_executed"`
}

func (r LiveReceiptContract) ValidateAgainst(envelope LiveManifestEnvelope) error {
	if err := envelope.Manifest.Validate(); err != nil {
		return err
	}
	digest, err := envelope.Manifest.Digest()
	if err != nil || digest != envelope.ManifestDigest {
		return fmt.Errorf("relay live manifest digest mismatch")
	}
	m := envelope.Manifest
	if r.SchemaVersion != m.FutureReceiptSchemaVersion ||
		r.ManifestDigest != envelope.ManifestDigest ||
		r.ProviderConfigDigest != m.ProviderConfigDigest ||
		!validText(r.GatewayOperationID, 256) ||
		r.RequestedModel != m.RequestedModel ||
		!validText(r.EffectiveProvider, 128) ||
		!validText(r.EffectiveModel, 128) ||
		r.PromptDigest != m.PromptDigest ||
		r.TurnStatus != "completed" ||
		r.Output != LiveProbeExpected ||
		r.OutputDigest != canonical.BytesDigest([]byte(r.Output)) ||
		r.ModelTurns != 1 ||
		r.ApprovalRequests != 0 ||
		r.UnexpectedToolUse ||
		r.RequestRetries != 0 ||
		r.StreamRetries != 0 ||
		!r.CredentialAccepted ||
		!r.LiveModelTurnExecuted {
		return fmt.Errorf("relay live qualification receipt does not satisfy frozen manifest")
	}
	return nil
}
