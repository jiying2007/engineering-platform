package engineeringevidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

func codexEvidenceFixture(t *testing.T) CodexImportRequest {
	t.Helper()
	bundle := []byte("git bundle fixture bytes\n")
	bundlePath := filepath.Join(t.TempDir(), "result.bundle")
	if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	change := workspace.ChangeFacts{
		Recipe:             workspace.FinalizeRecipe,
		BaseCommit:         strings.Repeat("1", 40),
		BaseTree:           strings.Repeat("2", 40),
		BaseSourceDigest:   "sha256:" + strings.Repeat("3", 64),
		ResultCommit:       strings.Repeat("4", 40),
		ResultTree:         strings.Repeat("5", 40),
		ResultSourceDigest: "sha256:" + strings.Repeat("6", 64),
		BundleDigest:       canonical.BytesDigest(bundle),
		BundleSize:         int64(len(bundle)),
	}
	modelReceipt := codexapp.EngineeringReceipt{
		SchemaVersion: 1,
		CLI: "codex-cli",
		Version: codexapp.QualifiedCodexVersion,
		BinaryDigest: "sha256:" + strings.Repeat("7", 64),
		EngineeringConfigDigest: codexapp.EngineeringConfigDigest(),
		CredentialMode: "workload_identity",
		FederationRuleID: "rule-test",
		Model: "gpt-5.6-sol",
		PromptDigest: "sha256:" + strings.Repeat("8", 64),
		ThreadID: "thread-1",
		TurnID: "turn-1",
		TurnStatus: "completed",
		Output: "implemented and tested",
		OutputDigest: canonical.BytesDigest([]byte("implemented and tested")),
		CommandCount: 2,
		FailedCommands: 0,
		FileChangeCount: 1,
		ApprovalRequests: 0,
		AssertionRemovedBeforeTurn: true,
	}
	result := codexexec.Result{
		PromptIdentityDigest: "sha256:" + strings.Repeat("9", 64),
		Codex: modelReceipt,
		Change: change,
	}
	resultDigest, err := canonical.Digest(result)
	if err != nil {
		t.Fatal(err)
	}
	token := codexexec.Token{
		ID: strings.Repeat("a", 64),
		RunID: "run-codex",
		WorkerProfile: "worker/codex",
		ProfileDigest: "sha256:" + strings.Repeat("b", 64),
		RecoveryEpoch: 1,
	}
	receipt := codexexec.Receipt{
		Kind: codexexec.Kind,
		Token: token,
		Worker: "urn:engineering-platform:worker:codex",
		PreparationDigest: "sha256:" + strings.Repeat("c", 64),
		Result: result,
		ResultDigest: resultDigest,
		ReceivedAt: time.Unix(100, 0).UTC(),
	}
	receiptDigest, err := CodexReceiptDigest(receipt)
	if err != nil {
		t.Fatal(err)
	}
	delivery := core.DeliveryReceipt{
		ID: "delivery-codex",
		TaskContractDigest: "sha256:" + strings.Repeat("d", 64),
		RunID: token.RunID,
		BaseCommit: change.BaseCommit,
		ResultCommit: change.ResultCommit,
		Artifacts: []core.ArtifactRef{
			{ID: "codex-receipt", Digest: receiptDigest, MediaType: CodexReceiptArtifactType},
			{ID: "codex-bundle", Digest: change.BundleDigest, MediaType: CodexBundleArtifactType},
		},
	}
	delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	return CodexImportRequest{
		EvidenceID: "evidence-codex",
		RequirementID: "req-codex",
		ReceiptArtifactID: "codex-receipt",
		BundleArtifactID: "codex-bundle",
		Delivery: delivery,
		Execution: codexexec.Status{Token: token, State: codexexec.Finished, Receipt: &receipt},
		BundlePath: bundlePath,
	}
}

func TestVerifyCodexImportBindsReceiptBundleAndDelivery(t *testing.T) {
	req := codexEvidenceFixture(t)
	got, err := VerifyCodexImport(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != "PASS" || got.Issuer != CodexIssuer || got.Procedure != CodexProcedure ||
		len(got.ArtifactRefs) != 2 || got.ArtifactRefs[0] != "codex-receipt" || got.ArtifactRefs[1] != "codex-bundle" {
		t.Fatalf("unexpected Codex evidence: %#v", got)
	}
}

func TestVerifyCodexImportRejectsDrift(t *testing.T) {
	for _, mode := range []string{"state", "run", "receipt-artifact", "bundle-artifact", "bundle-bytes", "result-commit", "model-output"} {
		t.Run(mode, func(t *testing.T) {
			req := codexEvidenceFixture(t)
			switch mode {
			case "state":
				req.Execution.State = codexexec.Unknown
			case "run":
				req.Execution.Receipt.Token.RunID = "other"
			case "receipt-artifact":
				req.Delivery.Artifacts[0].Digest = "sha256:" + strings.Repeat("e", 64)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			case "bundle-artifact":
				req.Delivery.Artifacts[1].Digest = "sha256:" + strings.Repeat("f", 64)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			case "bundle-bytes":
				if err := os.WriteFile(req.BundlePath, []byte("tampered"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "result-commit":
				req.Delivery.ResultCommit = strings.Repeat("0", 40)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			case "model-output":
				req.Execution.Receipt.Result.Codex.Output = "changed"
			}
			if _, err := VerifyCodexImport(req); err == nil {
				t.Fatal("drifted Codex evidence accepted")
			}
		})
	}
}
