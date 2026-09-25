package engineeringevidence

import (
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func offlineFixture(t *testing.T, exitCode int) OfflineImportRequest {
	t.Helper()
	taskDigest := "sha256:" + strings.Repeat("a", 64)
	profileDigest := "sha256:" + strings.Repeat("b", 64)
	baseCommit := strings.Repeat("1", 40)
	prepFacts := preparation.Facts{
		Version: 1, IntentDigest: "sha256:" + strings.Repeat("2", 64),
		InputDigest: "sha256:" + strings.Repeat("3", 64), TaskDigest: taskDigest,
		ApprovalDigest: "sha256:" + strings.Repeat("4", 64), BaseCommit: baseCommit,
		TreeCommit: strings.Repeat("5", 40), WorkspaceRecipe: "independent-git-snapshot-v1",
		SourceDigest: "sha256:" + strings.Repeat("6", 64), ConfigDigest: "sha256:" + strings.Repeat("7", 64),
		BundleDigest: "sha256:" + strings.Repeat("8", 64),
	}
	prepDigest, err := canonical.Digest(prepFacts)
	if err != nil {
		t.Fatal(err)
	}
	worker := "urn:engineering-platform:worker:test"
	prep := preparation.Receipt{
		Kind: preparation.Kind,
		Admission: workerqueue.Receipt{Worker: worker},
		Facts: prepFacts, FactsDigest: prepDigest, ReceivedAt: time.Unix(10, 0).UTC(),
	}
	result := sandbox.Result{
		Recipe: sandbox.Recipe, ProfileDigest: profileDigest,
		ContainerID: strings.Repeat("c", 64), ExitCode: exitCode,
		Stdout: []byte("ok\n"), Stderr: nil, UserID: 1000,
	}
	result.StdoutDigest = sandbox.Hash(result.Stdout)
	result.StderrDigest = sandbox.Hash(result.Stderr)
	resultDigest, err := canonical.Digest(result)
	if err != nil {
		t.Fatal(err)
	}
	receipt := offline.Receipt{
		Kind: offline.Kind,
		Token: offline.Token{ID: strings.Repeat("d", 64), RunID: "run-1", WorkerProfile: "worker/test", ProfileDigest: profileDigest},
		Worker: worker, PreparationDigest: prepDigest, Result: result, ResultDigest: resultDigest,
		ReceivedAt: time.Unix(20, 0).UTC(),
	}
	receiptDigest, err := OfflineReceiptDigest(receipt)
	if err != nil {
		t.Fatal(err)
	}
	delivery := core.DeliveryReceipt{
		ID: "delivery-1", TaskContractDigest: taskDigest, RunID: "run-1",
		BaseCommit: baseCommit, ResultCommit: strings.Repeat("9", 40),
		Artifacts: []core.ArtifactRef{{ID: "offline-receipt", Digest: receiptDigest, MediaType: OfflineArtifactType}},
	}
	delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	return OfflineImportRequest{
		EvidenceID: "evidence-1", RequirementID: "req-exec", EvidenceArtifactID: "offline-receipt",
		Delivery: delivery, Preparation: prep,
		Execution: offline.Status{Token: receipt.Token, State: offline.Finished, Receipt: &receipt},
	}
}

func TestVerifyOfflineImportPassAndFailAreFacts(t *testing.T) {
	pass := offlineFixture(t, 0)
	evidence, err := VerifyOfflineImport(pass)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Result != "PASS" || evidence.Issuer != OfflineIssuer || evidence.Procedure != OfflineProcedure ||
		len(evidence.ArtifactRefs) != 1 || evidence.ArtifactRefs[0] != "offline-receipt" {
		t.Fatalf("unexpected PASS evidence: %#v", evidence)
	}

	fail := offlineFixture(t, 17)
	evidence, err = VerifyOfflineImport(fail)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Result != "FAIL" {
		t.Fatalf("nonzero exit did not become retained FAIL evidence: %#v", evidence)
	}
}

func TestVerifyOfflineImportRejectsIdentityDrift(t *testing.T) {
	for _, mode := range []string{"artifact", "run", "preparation", "worker", "result"} {
		t.Run(mode, func(t *testing.T) {
			req := offlineFixture(t, 0)
			switch mode {
			case "artifact":
				req.Delivery.Artifacts[0].Digest = "sha256:" + strings.Repeat("f", 64)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			case "run":
				req.Execution.Receipt.Token.RunID = "other"
			case "preparation":
				req.Execution.Receipt.PreparationDigest = "sha256:" + strings.Repeat("f", 64)
			case "worker":
				req.Execution.Receipt.Worker = "urn:engineering-platform:worker:other"
			case "result":
				req.Execution.Receipt.Result.Stdout = []byte("tampered")
			}
			if _, err := VerifyOfflineImport(req); err == nil {
				t.Fatal("drifted offline evidence accepted")
			}
		})
	}
}
