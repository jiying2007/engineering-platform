package engineeringevidence

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

const (
	OfflineIssuer       = access.WorkerEvidenceIssuer
	OfflineProcedure    = access.WorkerEvidenceProcedure
	OfflineImporter     = access.WorkerEvidenceImporterSubject
	OfflineArtifactType = "application/vnd.engineering-platform.worker-offline-execution+json"
)

var logicalIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,191}$`)
var containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type OfflineImportRequest struct {
	EvidenceID         string
	RequirementID      string
	EvidenceArtifactID string
	Delivery           core.DeliveryReceipt
	Preparation        preparation.Receipt
	Execution          offline.Status
}

func VerifyOfflineImport(req OfflineImportRequest) (core.EvidenceRef, error) {
	var empty core.EvidenceRef
	if !validLogicalID(req.EvidenceID) || !validLogicalID(req.RequirementID) || !validLogicalID(req.EvidenceArtifactID) {
		return empty, fmt.Errorf("bounded evidence, requirement and artifact IDs required")
	}
	if err := verifyDelivery(req.Delivery); err != nil {
		return empty, err
	}
	if err := verifyPreparation(req.Preparation, req.Delivery); err != nil {
		return empty, err
	}
	receipt, err := verifyOfflineStatus(req.Execution, req.Delivery, req.Preparation)
	if err != nil {
		return empty, err
	}
	receiptDigest, err := canonical.Digest(receipt)
	if err != nil {
		return empty, err
	}
	artifact, ok := exactArtifact(req.Delivery, req.EvidenceArtifactID)
	if !ok || artifact.Digest != receiptDigest {
		return empty, fmt.Errorf("delivery must bind exact offline receipt digest")
	}
	if artifact.MediaType != "" && artifact.MediaType != OfflineArtifactType {
		return empty, fmt.Errorf("offline receipt delivery artifact has wrong media type")
	}
	result := "FAIL"
	if receipt.Result.ExitCode == 0 {
		result = "PASS"
	}
	return core.EvidenceRef{
		ID:                req.EvidenceID,
		DeliveryReceiptID: req.Delivery.ID,
		RequirementID:     req.RequirementID,
		SubjectDigest:     req.Delivery.SubjectDigest,
		Issuer:            OfflineIssuer,
		Procedure:         OfflineProcedure,
		Result:            result,
		ArtifactRefs:      []string{artifact.ID},
		Applicable:        true,
	}, nil
}

func OfflineReceiptDigest(receipt offline.Receipt) (string, error) {
	if err := validateOfflineReceipt(receipt); err != nil {
		return "", err
	}
	return canonical.Digest(receipt)
}

func verifyDelivery(delivery core.DeliveryReceipt) error {
	if delivery.ID == "" || delivery.RunID == "" || !canonical.ValidDigest(delivery.TaskContractDigest) || !canonical.ValidDigest(delivery.SubjectDigest) {
		return fmt.Errorf("complete delivery identity required")
	}
	calculated, err := delivery.CalculateSubjectDigest()
	if err != nil || calculated != delivery.SubjectDigest {
		return fmt.Errorf("delivery subject digest mismatch")
	}
	return nil
}

func verifyPreparation(receipt preparation.Receipt, delivery core.DeliveryReceipt) error {
	if receipt.Kind != preparation.Kind || receipt.ReceivedAt.IsZero() || !canonical.ValidDigest(receipt.FactsDigest) {
		return fmt.Errorf("invalid preparation receipt")
	}
	digest, err := canonical.Digest(receipt.Facts)
	if err != nil || digest != receipt.FactsDigest {
		return fmt.Errorf("preparation facts digest mismatch")
	}
	if receipt.Facts.TaskDigest != delivery.TaskContractDigest ||
		receipt.Facts.BaseCommit != strings.ToLower(delivery.BaseCommit) ||
		receipt.Facts.ExecutionStarted || receipt.Facts.OSIsolated ||
		receipt.Admission.Worker == "" {
		return fmt.Errorf("preparation does not bind delivery")
	}
	return nil
}

func verifyOfflineStatus(status offline.Status, delivery core.DeliveryReceipt, prep preparation.Receipt) (offline.Receipt, error) {
	if status.State != offline.Finished || status.Receipt == nil {
		return offline.Receipt{}, fmt.Errorf("offline execution is not FINISHED")
	}
	receipt := *status.Receipt
	if receipt.Token != status.Token ||
		receipt.Token.RunID != delivery.RunID ||
		receipt.Worker != prep.Admission.Worker ||
		receipt.PreparationDigest != prep.FactsDigest {
		return offline.Receipt{}, fmt.Errorf("offline execution identity does not bind delivery/preparation")
	}
	if err := validateOfflineReceipt(receipt); err != nil {
		return offline.Receipt{}, err
	}
	return receipt, nil
}

func validateOfflineReceipt(receipt offline.Receipt) error {
	if receipt.Kind != offline.Kind || !receipt.Token.Valid() || receipt.Worker == "" ||
		!canonical.ValidDigest(receipt.PreparationDigest) || !canonical.ValidDigest(receipt.ResultDigest) ||
		receipt.ReceivedAt.IsZero() {
		return fmt.Errorf("invalid offline execution receipt")
	}
	resultDigest, err := canonical.Digest(receipt.Result)
	if err != nil || resultDigest != receipt.ResultDigest {
		return fmt.Errorf("offline execution result digest mismatch")
	}
	result := receipt.Result
	if result.Recipe != sandbox.Recipe || result.ProfileDigest != receipt.Token.ProfileDigest ||
		!containerIDPattern.MatchString(result.ContainerID) || result.ExitCode < 0 || result.ExitCode > 255 ||
		result.ExitCode == 122 || result.UserID < 1 || len(result.Stdout) > sandbox.OutputLimit || len(result.Stderr) > sandbox.OutputLimit ||
		sandbox.Hash(result.Stdout) != result.StdoutDigest || sandbox.Hash(result.Stderr) != result.StderrDigest {
		return fmt.Errorf("offline execution result is structurally invalid")
	}
	return nil
}

func exactArtifact(delivery core.DeliveryReceipt, id string) (core.ArtifactRef, bool) {
	var found core.ArtifactRef
	ok := false
	for _, artifact := range delivery.Artifacts {
		if artifact.ID != id {
			continue
		}
		if ok {
			return core.ArtifactRef{}, false
		}
		found, ok = artifact, true
	}
	return found, ok
}

func validLogicalID(value string) bool {
	return logicalIDPattern.MatchString(value) && strings.TrimSpace(value) == value
}
