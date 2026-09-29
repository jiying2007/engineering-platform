package relayqualification

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestBuildLiveManifestFreezesOneReadOnlyNoToolTurn(t *testing.T) {
	contract := validContract()
	envelope, err := BuildLiveManifest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ManifestDigest == "" ||
		envelope.Manifest.ProviderConfigDigest == "" ||
		envelope.Manifest.MaxModelTurns != 1 ||
		!envelope.Manifest.ReadOnly ||
		envelope.Manifest.ApprovalPolicy != "never" ||
		envelope.Manifest.AllowTools ||
		envelope.Manifest.ToolNetwork ||
		envelope.Manifest.AllowWebSearch ||
		envelope.Manifest.RequestMaxRetries != 0 ||
		envelope.Manifest.StreamMaxRetries != 0 ||
		!envelope.Manifest.RequireGatewayOperationID ||
		!envelope.Manifest.RequireEffectiveProvider ||
		!envelope.Manifest.RequireEffectiveModel {
		t.Fatalf("unexpected manifest: %#v", envelope)
	}
	second, err := BuildLiveManifest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if envelope != second {
		t.Fatal("same contract did not produce deterministic live manifest")
	}
}

func TestLiveManifestDigestChangesWithProviderConfig(t *testing.T) {
	first := validContract()
	second := validContract()
	second.StreamIdleTimeoutMS = 31000

	a, err := BuildLiveManifest(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildLiveManifest(second)
	if err != nil {
		t.Fatal(err)
	}
	if a.ManifestDigest == b.ManifestDigest ||
		a.Manifest.ProviderConfigDigest == b.Manifest.ProviderConfigDigest {
		t.Fatal("live manifest ignored provider configuration drift")
	}
}

func TestSyntheticFutureReceiptMustMatchFrozenManifest(t *testing.T) {
	envelope, err := BuildLiveManifest(validContract())
	if err != nil {
		t.Fatal(err)
	}
	receipt := LiveReceiptContract{
		SchemaVersion:         LiveReceiptSchemaVersion,
		ManifestDigest:        envelope.ManifestDigest,
		ProviderConfigDigest:  envelope.Manifest.ProviderConfigDigest,
		GatewayOperationID:    "relay-op-123",
		RequestedModel:        envelope.Manifest.RequestedModel,
		EffectiveProvider:     "openai",
		EffectiveModel:        "gpt-5.6-sol",
		PromptDigest:          envelope.Manifest.PromptDigest,
		TurnStatus:            "completed",
		Output:                LiveProbeExpected,
		OutputDigest:          canonical.BytesDigest([]byte(LiveProbeExpected)),
		ModelTurns:            1,
		ApprovalRequests:      0,
		UnexpectedToolUse:     false,
		RequestRetries:        0,
		StreamRetries:         0,
		CredentialAccepted:    true,
		LiveModelTurnExecuted: true,
	}
	if err := receipt.ValidateAgainst(envelope); err != nil {
		t.Fatal(err)
	}

	receipt.ModelTurns = 2
	if err := receipt.ValidateAgainst(envelope); err == nil {
		t.Fatal("multi-turn receipt accepted")
	}
	receipt.ModelTurns = 1
	receipt.GatewayOperationID = ""
	if err := receipt.ValidateAgainst(envelope); err == nil {
		t.Fatal("receipt without gateway operation id accepted")
	}
	receipt.GatewayOperationID = "relay-op-123"
	receipt.Output = "different"
	receipt.OutputDigest = canonical.BytesDigest([]byte(receipt.Output))
	if err := receipt.ValidateAgainst(envelope); err == nil {
		t.Fatal("unexpected live output accepted")
	}
}
