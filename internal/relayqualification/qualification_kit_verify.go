package relayqualification

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type QualificationKitVerification struct {
	SchemaVersion            int    `json:"schema_version"`
	ManifestDigest           string `json:"manifest_digest"`
	ProviderConfigDigest     string `json:"provider_config_digest"`
	LiveManifestDigest       string `json:"live_manifest_digest"`
	RuntimeHandoffDigest     string `json:"runtime_handoff_digest"`
	CodexQualificationDigest string `json:"codex_qualification_digest"`
	CodexBinaryDigest        string `json:"codex_binary_digest"`
	RequestedModel           string `json:"requested_model"`
	FilesVerified            int    `json:"files_verified"`
}

func VerifyQualificationKit(kit QualificationKit) (QualificationKitVerification, error) {
	var out QualificationKitVerification
	for name, data := range map[string][]byte{
		KitContractFile:       kit.ContractJSON,
		KitCodexConfigFile:    kit.CodexConfig,
		KitLiveManifestFile:   kit.LiveManifestJSON,
		KitRuntimeHandoffFile: kit.RuntimeHandoffJSON,
		KitQualificationFile:  kit.QualificationJSON,
		KitManifestFile:       kit.ManifestJSON,
		KitChecksumsFile:      kit.Checksums,
	} {
		if len(data) == 0 || len(data) > 2<<20 {
			return out, fmt.Errorf("qualification kit file %s is empty or exceeds bound", name)
		}
	}

	var contract Contract
	if err := strictjson.Decode(kit.ContractJSON, &contract); err != nil {
		return out, fmt.Errorf("strict relay contract: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return out, err
	}
	normalizedContract, err := marshalKitJSON(contract)
	if err != nil || !bytes.Equal(normalizedContract, kit.ContractJSON) {
		return out, fmt.Errorf("relay contract bytes are not canonical")
	}
	renderedConfig, err := contract.RenderCodexConfig()
	if err != nil || !bytes.Equal(renderedConfig, kit.CodexConfig) {
		return out, fmt.Errorf("relay Codex config does not match contract")
	}

	var live LiveManifestEnvelope
	if err := strictjson.Decode(kit.LiveManifestJSON, &live); err != nil {
		return out, fmt.Errorf("strict live manifest: %w", err)
	}
	expectedLive, err := BuildLiveManifest(contract)
	if err != nil {
		return out, err
	}
	normalizedLive, err := marshalKitJSON(expectedLive)
	if err != nil || !bytes.Equal(normalizedLive, kit.LiveManifestJSON) || live != expectedLive {
		return out, fmt.Errorf("relay live manifest does not match contract")
	}

	var qualification codexapp.QualificationReceipt
	if err := strictjson.Decode(kit.QualificationJSON, &qualification); err != nil {
		return out, fmt.Errorf("strict Codex qualification: %w", err)
	}
	if err := qualification.Validate(); err != nil {
		return out, err
	}
	normalizedQualification, err := codexapp.MarshalQualification(qualification)
	if err != nil {
		return out, err
	}
	normalizedQualification = append(normalizedQualification, '\n')
	if !bytes.Equal(normalizedQualification, kit.QualificationJSON) {
		return out, fmt.Errorf("Codex qualification bytes are not canonical")
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil {
		return out, err
	}

	var handoff RuntimeHandoffEnvelope
	if err := strictjson.Decode(kit.RuntimeHandoffJSON, &handoff); err != nil {
		return out, fmt.Errorf("strict runtime handoff: %w", err)
	}
	if err := handoff.Handoff.Validate(live, qualification); err != nil {
		return out, err
	}
	handoffDigest, err := handoff.Handoff.Digest()
	if err != nil || handoffDigest != handoff.HandoffDigest {
		return out, fmt.Errorf("relay runtime handoff digest mismatch")
	}
	normalizedHandoff, err := marshalKitJSON(handoff)
	if err != nil || !bytes.Equal(normalizedHandoff, kit.RuntimeHandoffJSON) {
		return out, fmt.Errorf("relay runtime handoff bytes are not canonical")
	}

	var manifest QualificationKitManifest
	if err := strictjson.Decode(kit.ManifestJSON, &manifest); err != nil {
		return out, fmt.Errorf("strict qualification kit manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return out, err
	}
	manifestDigest, err := manifest.Digest()
	if err != nil {
		return out, err
	}
	normalizedManifest, err := marshalKitJSON(manifest)
	if err != nil || !bytes.Equal(normalizedManifest, kit.ManifestJSON) {
		return out, fmt.Errorf("qualification kit manifest bytes are not canonical")
	}

	if manifest.ProviderConfigDigest != live.Manifest.ProviderConfigDigest ||
		manifest.LiveManifestDigest != live.ManifestDigest ||
		manifest.RuntimeHandoffDigest != handoff.HandoffDigest ||
		manifest.CodexQualificationDigest != qualificationDigest ||
		manifest.CodexBinaryDigest != qualification.BinaryDigest ||
		manifest.RequestedModel != contract.RequestedModel {
		return out, fmt.Errorf("qualification kit authority identities are not cross-bound")
	}

	actualFiles := []QualificationKitFile{
		{Name: KitContractFile, Digest: canonical.BytesDigest(kit.ContractJSON)},
		{Name: KitCodexConfigFile, Digest: canonical.BytesDigest(kit.CodexConfig)},
		{Name: KitLiveManifestFile, Digest: canonical.BytesDigest(kit.LiveManifestJSON)},
		{Name: KitRuntimeHandoffFile, Digest: canonical.BytesDigest(kit.RuntimeHandoffJSON)},
		{Name: KitQualificationFile, Digest: canonical.BytesDigest(kit.QualificationJSON)},
	}
	if len(manifest.Files) != len(actualFiles) {
		return out, fmt.Errorf("qualification kit manifest file count mismatch")
	}
	for i := range actualFiles {
		if manifest.Files[i] != actualFiles[i] {
			return out, fmt.Errorf("qualification kit file digest mismatch for %s", actualFiles[i].Name)
		}
	}

	checksumInputs := append(append([]QualificationKitFile{}, actualFiles...),
		QualificationKitFile{Name: KitManifestFile, Digest: canonical.BytesDigest(kit.ManifestJSON)})
	var expectedSums strings.Builder
	for _, file := range checksumInputs {
		fmt.Fprintf(&expectedSums, "%s  %s\n", strings.TrimPrefix(file.Digest, "sha256:"), file.Name)
	}
	if !bytes.Equal([]byte(expectedSums.String()), kit.Checksums) {
		return out, fmt.Errorf("qualification kit SHA256SUMS mismatch")
	}

	out = QualificationKitVerification{
		SchemaVersion:            QualificationKitSchemaVersion,
		ManifestDigest:           manifestDigest,
		ProviderConfigDigest:     manifest.ProviderConfigDigest,
		LiveManifestDigest:       manifest.LiveManifestDigest,
		RuntimeHandoffDigest:     manifest.RuntimeHandoffDigest,
		CodexQualificationDigest: manifest.CodexQualificationDigest,
		CodexBinaryDigest:        manifest.CodexBinaryDigest,
		RequestedModel:           manifest.RequestedModel,
		FilesVerified:            len(actualFiles) + 2,
	}
	return out, nil
}
