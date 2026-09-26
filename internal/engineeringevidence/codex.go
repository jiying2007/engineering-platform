package engineeringevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

const (
	CodexIssuer              = access.CodexEvidenceIssuer
	CodexProcedure           = access.CodexEvidenceProcedure
	CodexImporter            = access.CodexEvidenceImporterSubject
	CodexReceiptArtifactType = "application/vnd.engineering-platform.codex-execution+json"
	CodexBundleArtifactType  = "application/vnd.git.bundle"
)

type CodexImportRequest struct {
	EvidenceID        string
	RequirementID     string
	ReceiptArtifactID string
	BundleArtifactID  string
	Delivery          core.DeliveryReceipt
	Execution         codexexec.Status
	BundlePath        string
}

func CodexReceiptDigest(receipt codexexec.Receipt) (string, error) {
	if err := validateCodexReceipt(receipt); err != nil {
		return "", err
	}
	return canonical.Digest(receipt)
}

func VerifyCodexImport(req CodexImportRequest) (core.EvidenceRef, error) {
	var empty core.EvidenceRef
	if !validLogicalID(req.EvidenceID) || !validLogicalID(req.RequirementID) ||
		!validLogicalID(req.ReceiptArtifactID) || !validLogicalID(req.BundleArtifactID) ||
		req.ReceiptArtifactID == req.BundleArtifactID {
		return empty, fmt.Errorf("bounded distinct evidence artifact IDs required")
	}
	if err := verifyDelivery(req.Delivery); err != nil {
		return empty, err
	}
	if req.Execution.State != codexexec.Finished || req.Execution.Receipt == nil {
		return empty, fmt.Errorf("Core-bound Codex execution is not FINISHED")
	}
	receipt := *req.Execution.Receipt
	if receipt.Token != req.Execution.Token || receipt.Token.RunID != req.Delivery.RunID {
		return empty, fmt.Errorf("Codex execution does not bind Delivery Run")
	}
	if err := validateCodexReceipt(receipt); err != nil {
		return empty, err
	}
	change := receipt.Result.Change
	if strings.ToLower(req.Delivery.BaseCommit) != change.BaseCommit ||
		strings.ToLower(req.Delivery.ResultCommit) != change.ResultCommit {
		return empty, fmt.Errorf("Codex result commits do not bind Delivery")
	}
	receiptDigest, err := canonical.Digest(receipt)
	if err != nil {
		return empty, err
	}
	receiptArtifact, ok := exactArtifact(req.Delivery, req.ReceiptArtifactID)
	if !ok || receiptArtifact.Digest != receiptDigest {
		return empty, fmt.Errorf("Delivery must bind exact Codex receipt digest")
	}
	if receiptArtifact.MediaType != "" && receiptArtifact.MediaType != CodexReceiptArtifactType {
		return empty, fmt.Errorf("Codex receipt artifact has wrong media type")
	}
	bundleArtifact, ok := exactArtifact(req.Delivery, req.BundleArtifactID)
	if !ok || bundleArtifact.Digest != change.BundleDigest {
		return empty, fmt.Errorf("Delivery must bind exact Codex result bundle digest")
	}
	if bundleArtifact.MediaType != "" && bundleArtifact.MediaType != CodexBundleArtifactType {
		return empty, fmt.Errorf("Codex result bundle artifact has wrong media type")
	}
	digest, size, err := immutableFileDigest(req.BundlePath, 1<<30)
	if err != nil {
		return empty, err
	}
	if digest != change.BundleDigest || size != change.BundleSize {
		return empty, fmt.Errorf("local result bundle bytes do not match retained Codex receipt")
	}
	return core.EvidenceRef{
		ID:                req.EvidenceID,
		DeliveryReceiptID: req.Delivery.ID,
		RequirementID:     req.RequirementID,
		SubjectDigest:     req.Delivery.SubjectDigest,
		Issuer:            CodexIssuer,
		Procedure:         CodexProcedure,
		Result:            "PASS",
		ArtifactRefs:      []string{receiptArtifact.ID, bundleArtifact.ID},
		Applicable:        true,
	}, nil
}

func validateCodexReceipt(receipt codexexec.Receipt) error {
	if receipt.Kind != codexexec.Kind || !receipt.Token.Valid() || receipt.Worker == "" ||
		!canonical.ValidDigest(receipt.PreparationDigest) || !canonical.ValidDigest(receipt.ResultDigest) ||
		receipt.ReceivedAt.IsZero() {
		return fmt.Errorf("invalid Core-bound Codex receipt")
	}
	resultDigest, err := canonical.Digest(receipt.Result)
	if err != nil || resultDigest != receipt.ResultDigest {
		return fmt.Errorf("Core-bound Codex result digest mismatch")
	}
	result := receipt.Result
	if !canonical.ValidDigest(result.PromptIdentityDigest) || result.Codex.Validate() != nil {
		return fmt.Errorf("invalid Codex model-turn receipt")
	}
	change := result.Change
	if change.Recipe != workspace.FinalizeRecipe ||
		len(change.BaseCommit) != 40 || len(change.BaseTree) != 40 ||
		!canonical.ValidDigest(change.BaseSourceDigest) ||
		len(change.ResultCommit) != 40 || len(change.ResultTree) != 40 ||
		!canonical.ValidDigest(change.ResultSourceDigest) ||
		!canonical.ValidDigest(change.BundleDigest) || change.BundleSize <= 0 ||
		change.BaseCommit == change.ResultCommit || change.BaseTree == change.ResultTree ||
		change.BaseSourceDigest == change.ResultSourceDigest {
		return fmt.Errorf("invalid Codex changed-tree facts")
	}
	return nil
}

func immutableFileDigest(path string, limit int64) (string, int64, error) {
	if path == "" || limit <= 0 {
		return "", 0, fmt.Errorf("bounded artifact path required")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		before.Size() <= 0 || before.Size() > limit {
		return "", 0, fmt.Errorf("bounded regular artifact file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return "", 0, fmt.Errorf("artifact changed before read")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil || n != before.Size() || n > limit {
		return "", 0, fmt.Errorf("artifact digest read failed")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		return "", 0, fmt.Errorf("artifact changed during read")
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), n, nil
}
