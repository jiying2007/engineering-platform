// Package rcdelivery binds one exact successful main CI receipt to repository
// governance/status bytes. It is an internal RC delivery provenance envelope,
// never production qualification, provider evidence or an independent review.
package rcdelivery

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/cievidence"
	"github.com/jiying2007/engineering-platform/internal/production"
)

const (
	Version = 1
	Status  = "INTERNAL_RC_DELIVERY_BOUND_EXTERNAL_GATES_OPEN"
)

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Delivery struct {
	Version                         int                 `json:"version"`
	Status                          string              `json:"status"`
	Repository                      string              `json:"repository"`
	SourceSHA                       string              `json:"source_sha"`
	SourceTree                      string              `json:"source_tree"`
	CIRunID                         int64               `json:"ci_run_id"`
	CIRunAttempt                    int64               `json:"ci_run_attempt"`
	CIEvidenceDigest                string              `json:"ci_evidence_digest"`
	BinaryArtifact                  cievidence.Artifact `json:"binary_artifact"`
	CodexQualificationArtifact      cievidence.Artifact `json:"codex_qualification_artifact"`
	ImplementationStatusDigest      string              `json:"implementation_status_digest"`
	RetainedEvidenceManifestDigest  string              `json:"retained_evidence_manifest_digest"`
	RetainedPrototypeManifestDigest string              `json:"retained_prototype_manifest_digest"`
	TerminalPlanDigest              string              `json:"terminal_plan_digest"`
	TerminalRequiredGates           []string            `json:"terminal_required_gates"`
	ProviderLiveExecuted            bool                `json:"provider_live_executed"`
	HumanReviewPerformed            bool                `json:"human_review_performed"`
	ProductionQualified             bool                `json:"production_qualified"`
}

type Envelope struct {
	Delivery       Delivery `json:"delivery"`
	DeliveryDigest string   `json:"delivery_digest"`
}

func selectArtifacts(receipt cievidence.Receipt) (cievidence.Artifact, cievidence.Artifact, error) {
	var binaries, codex cievidence.Artifact
	for _, artifact := range receipt.Artifacts {
		switch {
		case strings.HasPrefix(artifact.Name, "engineering-binaries-"):
			if binaries.Name != "" {
				return binaries, codex, fmt.Errorf("ambiguous binary artifact")
			}
			binaries = artifact
		case strings.HasPrefix(artifact.Name, "codex-compatibility-qualification-"):
			if codex.Name != "" {
				return binaries, codex, fmt.Errorf("ambiguous Codex qualification artifact")
			}
			codex = artifact
		}
	}
	if binaries.Name != "engineering-binaries-"+receipt.SourceSHA ||
		codex.Name != "codex-compatibility-qualification-"+receipt.SourceSHA {
		return binaries, codex, fmt.Errorf("source-bound binary and Codex artifacts required")
	}
	return binaries, codex, nil
}

func Build(ci cievidence.Envelope, sourceTree string, implementationStatus, retainedEvidence, retainedPrototypes []byte) (Envelope, error) {
	var zero Envelope
	if err := ci.Verify(); err != nil {
		return zero, err
	}
	receipt := ci.Receipt
	if receipt.Event != "push" || receipt.Repository != cievidence.TrustedRepository ||
		receipt.SourceSHA != receipt.TestedSHA || receipt.BaseSHA != "" ||
		!sha40.MatchString(receipt.SourceSHA) || !sha40.MatchString(sourceTree) {
		return zero, fmt.Errorf("exact successful main push CI identity required")
	}
	if len(implementationStatus) == 0 || len(implementationStatus) > 1<<20 ||
		!strings.HasPrefix(string(implementationStatus), "# Implementation Status\n") ||
		len(retainedEvidence) == 0 || len(retainedEvidence) > 1<<20 ||
		len(retainedPrototypes) == 0 || len(retainedPrototypes) > 1<<20 {
		return zero, fmt.Errorf("bounded repository governance bytes required")
	}
	binaries, codex, err := selectArtifacts(receipt)
	if err != nil {
		return zero, err
	}
	terminal, err := production.BuildTerminalPlan()
	if err != nil {
		return zero, err
	}
	delivery := Delivery{
		Version: Version, Status: Status, Repository: receipt.Repository,
		SourceSHA: receipt.SourceSHA, SourceTree: sourceTree,
		CIRunID: receipt.RunID, CIRunAttempt: receipt.RunAttempt,
		CIEvidenceDigest: ci.ReceiptDigest,
		BinaryArtifact:   binaries, CodexQualificationArtifact: codex,
		ImplementationStatusDigest:      canonical.BytesDigest(implementationStatus),
		RetainedEvidenceManifestDigest:  canonical.BytesDigest(retainedEvidence),
		RetainedPrototypeManifestDigest: canonical.BytesDigest(retainedPrototypes),
		TerminalPlanDigest:              terminal.PlanDigest,
		TerminalRequiredGates:           append([]string(nil), terminal.Plan.RequiredGates...),
	}
	digest, err := canonical.Digest(delivery)
	if err != nil {
		return zero, err
	}
	envelope := Envelope{Delivery: delivery, DeliveryDigest: digest}
	if err := envelope.Verify(); err != nil {
		return zero, err
	}
	return envelope, nil
}

func (e Envelope) Verify() error {
	d := e.Delivery
	if d.Version != Version || d.Status != Status || d.Repository != cievidence.TrustedRepository ||
		!sha40.MatchString(d.SourceSHA) || !sha40.MatchString(d.SourceTree) ||
		d.CIRunID <= 0 || d.CIRunAttempt <= 0 || !canonical.ValidDigest(d.CIEvidenceDigest) ||
		!canonical.ValidDigest(d.ImplementationStatusDigest) ||
		!canonical.ValidDigest(d.RetainedEvidenceManifestDigest) ||
		!canonical.ValidDigest(d.RetainedPrototypeManifestDigest) ||
		!canonical.ValidDigest(d.TerminalPlanDigest) ||
		d.ProviderLiveExecuted || d.HumanReviewPerformed || d.ProductionQualified {
		return fmt.Errorf("invalid or overclaimed RC delivery")
	}
	if d.BinaryArtifact.Name != "engineering-binaries-"+d.SourceSHA ||
		d.CodexQualificationArtifact.Name != "codex-compatibility-qualification-"+d.SourceSHA ||
		d.BinaryArtifact.ID <= 0 || d.CodexQualificationArtifact.ID <= 0 ||
		d.BinaryArtifact.Size <= 0 || d.CodexQualificationArtifact.Size <= 0 ||
		!canonical.ValidDigest(d.BinaryArtifact.Digest) || !canonical.ValidDigest(d.CodexQualificationArtifact.Digest) {
		return fmt.Errorf("invalid source-bound delivery artifacts")
	}
	terminal, err := production.BuildTerminalPlan()
	if err != nil || d.TerminalPlanDigest != terminal.PlanDigest ||
		len(d.TerminalRequiredGates) != len(terminal.Plan.RequiredGates) {
		return fmt.Errorf("terminal acceptance contract drift")
	}
	for i := range terminal.Plan.RequiredGates {
		if d.TerminalRequiredGates[i] != terminal.Plan.RequiredGates[i] {
			return fmt.Errorf("terminal gate order drift")
		}
	}
	digest, err := canonical.Digest(d)
	if err != nil || digest != e.DeliveryDigest {
		return fmt.Errorf("RC delivery digest mismatch")
	}
	return nil
}
