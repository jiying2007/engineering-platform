package codexapp

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// qualificationReceiptV3 freezes the historical canonical field order and wire
// shape. Adding current fields here would change previously frozen digests.
// This type has no execution/admission methods.
type qualificationReceiptV3 struct {
	SchemaVersion                int    `json:"schema_version"`
	CompatibilityContractVersion int    `json:"compatibility_contract_version"`
	CLI                          string `json:"cli"`
	Version                      string `json:"version"`
	BinaryDigest                 string `json:"binary_digest"`
	StableSchemaDigest           string `json:"stable_schema_digest"`
	ExperimentalSchemaDigest     string `json:"experimental_schema_digest"`
	Transport                    string `json:"transport"`
	FreshProcess                 bool   `json:"fresh_process"`
	ManagedDaemon                bool   `json:"managed_daemon"`
	PerThreadConfigOverride      bool   `json:"per_thread_config_override"`
	InitializePassed             bool   `json:"initialize_passed"`
	ThreadStartPassed            bool   `json:"thread_start_passed"`
	ThreadStartModel             string `json:"thread_start_model"`
	StableSchemaContractChecked  bool   `json:"stable_schema_contract_checked"`
	ExperimentalSurfaceChecked   bool   `json:"experimental_surface_checked"`
	CredentialSafeConfigDigest   string `json:"credential_safe_config_digest"`
	CredentialSafeProfileChecked bool   `json:"credential_safe_profile_checked"`
	EngineeringConfigDigest      string `json:"engineering_config_digest"`
	EngineeringProfileChecked    bool   `json:"engineering_profile_checked"`
	IsolationMechanism           string `json:"isolation_mechanism"`
	IsolationEnvironmentDigest   string `json:"isolation_environment_digest"`
	IsolatedEngineeringStartup   bool   `json:"isolated_engineering_startup"`
	NamespaceInitReaped          bool   `json:"namespace_init_reaped"`
	ModelTurnExecuted            bool   `json:"model_turn_executed"`
	CredentialUsed               bool   `json:"credential_used"`
}

// VerifyRetainedQualification reads evidence for an already completed execution
// against the qualification digest frozen in its original Permit. It never
// starts a process, changes the receipt or qualifies it for a new execution.
// expectedDigest must come from that independently bound frozen identity, not
// from hashing the supplied receipt and treating it as its own trust anchor.
// A historical v3 result keeps SchemaVersion=3 and no v4 read-boundary claim;
// Validate, Digest and VerifyExecutableQualification continue to reject it.
func VerifyRetainedQualification(raw []byte, expectedDigest string) (QualificationReceipt, error) {
	var receipt QualificationReceipt
	if !canonical.ValidDigest(expectedDigest) || strictjson.Decode(raw, &receipt) != nil {
		return QualificationReceipt{}, fmt.Errorf("invalid retained Codex qualification")
	}
	var digest string
	var err error
	switch receipt.SchemaVersion {
	case QualificationSchemaVersion:
		digest, err = receipt.Digest()
	case 3:
		var original qualificationReceiptV3
		if strictjson.Decode(raw, &original) != nil || receipt.validateCompatibilityEvidence() != nil {
			return QualificationReceipt{}, fmt.Errorf("invalid retained v3 Codex qualification")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 26 {
			return QualificationReceipt{}, fmt.Errorf("incomplete retained v3 Codex qualification")
		}
		for _, value := range fields {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return QualificationReceipt{}, fmt.Errorf("null retained v3 Codex qualification field")
			}
		}
		digest, err = canonical.Digest(original)
	default:
		return QualificationReceipt{}, fmt.Errorf("unsupported retained Codex qualification schema")
	}
	if err != nil || digest != expectedDigest {
		return QualificationReceipt{}, fmt.Errorf("retained Codex qualification digest mismatch")
	}
	return receipt, nil
}
