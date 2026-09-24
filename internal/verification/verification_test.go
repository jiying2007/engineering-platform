package verification

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
)

func TestVerificationRejectsStaleSubjectEvidence(t *testing.T) {
	plan := Plan{
		ID:            "vp-1",
		SubjectDigest: "sha256:new",
		Criteria: []Criterion{{
			ID:               "ac-1",
			RequiredEvidence: []string{"ev-1"},
		}},
	}
	evidence := []core.EvidenceRef{{
		ID:            "ev-1",
		SubjectDigest: "sha256:old",
		Result:        "PASS",
		Applicable:    true,
	}}
	got := Evaluate(plan, evidence)
	if got.Result != "FAIL" {
		t.Fatalf("expected FAIL, got %s", got.Result)
	}
}

func TestVerificationPassesExactApplicableEvidence(t *testing.T) {
	plan := Plan{
		ID:            "vp-1",
		SubjectDigest: "sha256:subject",
		Criteria: []Criterion{{
			ID:               "ac-1",
			RequiredEvidence: []string{"ev-1"},
		}},
	}
	evidence := []core.EvidenceRef{{
		ID:            "ev-1",
		SubjectDigest: "sha256:subject",
		Result:        "PASS",
		Applicable:    true,
	}}
	got := Evaluate(plan, evidence)
	if got.Result != "PASS" {
		t.Fatalf("expected PASS, got %s", got.Result)
	}
}

func TestCriterionWithoutEvidenceCannotPass(t *testing.T) {
	plan := Plan{
		ID:            "vp-empty",
		SubjectDigest: "sha256:subject",
		Criteria:      []Criterion{{ID: "ac-empty"}},
	}
	got := Evaluate(plan, nil)
	if got.Result != "FAIL" {
		t.Fatalf("expected FAIL for criterion without evidence, got %s", got.Result)
	}
}
