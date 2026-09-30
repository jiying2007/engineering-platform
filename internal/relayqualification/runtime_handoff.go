package relayqualification

import (
	"context"
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

const RuntimeHandoffSchemaVersion = 1

type RuntimeHandoff struct {
	SchemaVersion            int    `json:"schema_version"`
	LiveManifestDigest       string `json:"live_manifest_digest"`
	ProviderConfigDigest     string `json:"provider_config_digest"`
	CodexQualificationDigest string `json:"codex_qualification_digest"`
	CodexVersion             string `json:"codex_version"`
	CodexBinaryDigest        string `json:"codex_binary_digest"`
	RequestedModel           string `json:"requested_model"`
}

type RuntimeHandoffEnvelope struct {
	Handoff       RuntimeHandoff `json:"handoff"`
	HandoffDigest string         `json:"handoff_digest"`
}

func BuildRuntimeHandoff(
	ctx context.Context,
	contract Contract,
	executable string,
	qualification codexapp.QualificationReceipt,
) (RuntimeHandoffEnvelope, error) {
	live, err := BuildLiveManifest(contract)
	if err != nil {
		return RuntimeHandoffEnvelope{}, err
	}
	if err := codexapp.VerifyExecutableQualification(ctx, executable, contract.RequestedModel, qualification); err != nil {
		return RuntimeHandoffEnvelope{}, err
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil {
		return RuntimeHandoffEnvelope{}, err
	}
	handoff := RuntimeHandoff{
		SchemaVersion:            RuntimeHandoffSchemaVersion,
		LiveManifestDigest:       live.ManifestDigest,
		ProviderConfigDigest:     live.Manifest.ProviderConfigDigest,
		CodexQualificationDigest: qualificationDigest,
		CodexVersion:             qualification.Version,
		CodexBinaryDigest:        qualification.BinaryDigest,
		RequestedModel:           contract.RequestedModel,
	}
	if err := handoff.Validate(live, qualification); err != nil {
		return RuntimeHandoffEnvelope{}, err
	}
	digest, err := handoff.Digest()
	if err != nil {
		return RuntimeHandoffEnvelope{}, err
	}
	return RuntimeHandoffEnvelope{Handoff: handoff, HandoffDigest: digest}, nil
}

func (h RuntimeHandoff) Validate(live LiveManifestEnvelope, qualification codexapp.QualificationReceipt) error {
	if err := live.Manifest.Validate(); err != nil {
		return err
	}
	liveDigest, err := live.Manifest.Digest()
	if err != nil || liveDigest != live.ManifestDigest {
		return fmt.Errorf("relay live manifest digest mismatch")
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil {
		return err
	}
	if h.SchemaVersion != RuntimeHandoffSchemaVersion ||
		h.LiveManifestDigest != live.ManifestDigest ||
		h.ProviderConfigDigest != live.Manifest.ProviderConfigDigest ||
		h.CodexQualificationDigest != qualificationDigest ||
		h.CodexVersion != qualification.Version ||
		h.CodexBinaryDigest != qualification.BinaryDigest ||
		h.RequestedModel != live.Manifest.RequestedModel ||
		qualification.ThreadStartModel != h.RequestedModel ||
		!canonical.ValidDigest(h.CodexBinaryDigest) {
		return fmt.Errorf("invalid relay runtime handoff")
	}
	return nil
}

func (h RuntimeHandoff) Digest() (string, error) {
	if h.SchemaVersion != RuntimeHandoffSchemaVersion ||
		!canonical.ValidDigest(h.LiveManifestDigest) ||
		!canonical.ValidDigest(h.ProviderConfigDigest) ||
		!canonical.ValidDigest(h.CodexQualificationDigest) ||
		!codexapp.ValidCodexVersion(h.CodexVersion) ||
		!canonical.ValidDigest(h.CodexBinaryDigest) ||
		!validText(h.RequestedModel, 128) {
		return "", fmt.Errorf("invalid relay runtime handoff identity")
	}
	return canonical.Digest(h)
}
