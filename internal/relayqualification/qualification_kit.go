package relayqualification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

const QualificationKitSchemaVersion = 1

const (
	KitContractFile       = "relay-contract-v2.json"
	KitCodexConfigFile    = "relay-codex-config.toml"
	KitLiveManifestFile   = "relay-live-manifest.json"
	KitRuntimeHandoffFile = "relay-runtime-handoff.json"
	KitQualificationFile  = "codex-compatibility-qualification.json"
	KitManifestFile       = "relay-qualification-kit-manifest.json"
	KitChecksumsFile      = "SHA256SUMS"
)

type QualificationKitFile struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type QualificationKitManifest struct {
	SchemaVersion            int                    `json:"schema_version"`
	ProviderConfigDigest     string                 `json:"provider_config_digest"`
	LiveManifestDigest       string                 `json:"live_manifest_digest"`
	RuntimeHandoffDigest     string                 `json:"runtime_handoff_digest"`
	CodexQualificationDigest string                 `json:"codex_qualification_digest"`
	CodexBinaryDigest        string                 `json:"codex_binary_digest"`
	RequestedModel           string                 `json:"requested_model"`
	Files                    []QualificationKitFile `json:"files"`
}

type QualificationKit struct {
	ContractJSON       []byte
	CodexConfig        []byte
	LiveManifestJSON   []byte
	RuntimeHandoffJSON []byte
	QualificationJSON  []byte
	ManifestJSON       []byte
	Checksums          []byte
	ManifestDigest     string
}

func BuildQualificationKit(
	ctx context.Context,
	contract Contract,
	executable string,
	qualification codexapp.QualificationReceipt,
) (QualificationKit, error) {
	var kit QualificationKit
	live, err := BuildLiveManifest(contract)
	if err != nil {
		return kit, err
	}
	handoff, err := BuildRuntimeHandoff(ctx, contract, executable, qualification)
	if err != nil {
		return kit, err
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil {
		return kit, err
	}

	contractJSON, err := marshalKitJSON(contract)
	if err != nil {
		return kit, err
	}
	config, err := contract.RenderCodexConfig()
	if err != nil {
		return kit, err
	}
	liveJSON, err := marshalKitJSON(live)
	if err != nil {
		return kit, err
	}
	handoffJSON, err := marshalKitJSON(handoff)
	if err != nil {
		return kit, err
	}
	qualificationJSON, err := codexapp.MarshalQualification(qualification)
	if err != nil {
		return kit, err
	}
	qualificationJSON = append(qualificationJSON, '\n')

	files := []QualificationKitFile{
		{Name: KitContractFile, Digest: canonical.BytesDigest(contractJSON)},
		{Name: KitCodexConfigFile, Digest: canonical.BytesDigest(config)},
		{Name: KitLiveManifestFile, Digest: canonical.BytesDigest(liveJSON)},
		{Name: KitRuntimeHandoffFile, Digest: canonical.BytesDigest(handoffJSON)},
		{Name: KitQualificationFile, Digest: canonical.BytesDigest(qualificationJSON)},
	}
	manifest := QualificationKitManifest{
		SchemaVersion:            QualificationKitSchemaVersion,
		ProviderConfigDigest:     live.Manifest.ProviderConfigDigest,
		LiveManifestDigest:       live.ManifestDigest,
		RuntimeHandoffDigest:     handoff.HandoffDigest,
		CodexQualificationDigest: qualificationDigest,
		CodexBinaryDigest:        qualification.BinaryDigest,
		RequestedModel:           contract.RequestedModel,
		Files:                    files,
	}
	if err := manifest.Validate(); err != nil {
		return kit, err
	}
	manifestDigest, err := manifest.Digest()
	if err != nil {
		return kit, err
	}
	manifestJSON, err := marshalKitJSON(manifest)
	if err != nil {
		return kit, err
	}

	checksumInputs := append(append([]QualificationKitFile{}, files...),
		QualificationKitFile{Name: KitManifestFile, Digest: canonical.BytesDigest(manifestJSON)})
	var sums strings.Builder
	for _, file := range checksumInputs {
		fmt.Fprintf(&sums, "%s  %s\n", strings.TrimPrefix(file.Digest, "sha256:"), file.Name)
	}

	kit = QualificationKit{
		ContractJSON:       contractJSON,
		CodexConfig:        config,
		LiveManifestJSON:   liveJSON,
		RuntimeHandoffJSON: handoffJSON,
		QualificationJSON:  qualificationJSON,
		ManifestJSON:       manifestJSON,
		Checksums:          []byte(sums.String()),
		ManifestDigest:     manifestDigest,
	}
	return kit, nil
}

func (m QualificationKitManifest) Validate() error {
	if m.SchemaVersion != QualificationKitSchemaVersion ||
		!canonical.ValidDigest(m.ProviderConfigDigest) ||
		!canonical.ValidDigest(m.LiveManifestDigest) ||
		!canonical.ValidDigest(m.RuntimeHandoffDigest) ||
		!canonical.ValidDigest(m.CodexQualificationDigest) ||
		!canonical.ValidDigest(m.CodexBinaryDigest) ||
		!validText(m.RequestedModel, 128) ||
		len(m.Files) != 5 {
		return fmt.Errorf("invalid relay qualification kit manifest")
	}
	expected := []string{
		KitContractFile,
		KitCodexConfigFile,
		KitLiveManifestFile,
		KitRuntimeHandoffFile,
		KitQualificationFile,
	}
	for i, file := range m.Files {
		if file.Name != expected[i] || !canonical.ValidDigest(file.Digest) {
			return fmt.Errorf("invalid relay qualification kit file identity")
		}
	}
	return nil
}

func (m QualificationKitManifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(m)
}

func marshalKitJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
