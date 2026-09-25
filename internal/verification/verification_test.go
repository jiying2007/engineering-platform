package verification

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
)

func testPlan(procedure string) Plan {
	return Plan{
		ID: "vp-1",
		Criteria: []Criterion{{
			ID:        "ac-1",
			Statement: "tests pass",
			Requirements: []EvidenceRequirement{{
				ID:        "req-1",
				Procedure: procedure,
			}},
		}},
	}
}

func TestPlanDigestChangesWhenRequirementsChange(t *testing.T) {
	p1 := testPlan("ci.test")
	p2 := testPlan("hil.test")
	d1, _ := p1.Digest()
	d2, _ := p2.Digest()
	if d1 == d2 {
		t.Fatal("verification plan digest must change when evidence requirements change")
	}
}

func TestValidatePlanRequiresAcceptanceCoverage(t *testing.T) {
	if !ValidatePlan(testPlan("ci.test"), []string{"tests pass"}) {
		t.Fatal("expected plan to cover acceptance criteria")
	}
	if ValidatePlan(testPlan("ci.test"), []string{"different criterion"}) {
		t.Fatal("plan must not cover a different acceptance criterion")
	}
}

func TestVerificationRejectsStaleSubjectEvidence(t *testing.T) {
	plan := testPlan("ci.test")
	evidence := []core.EvidenceRef{{
		ID:            "ev-1",
		RequirementID: "req-1",
		SubjectDigest: "sha256:old",
		Procedure:     "ci.test",
		Result:        "PASS",
		Applicable:    true,
	}}
	got := Evaluate(plan, "sha256:new", evidence)
	if got.Result != "FAIL" {
		t.Fatalf("expected FAIL, got %s", got.Result)
	}
}

func TestVerificationPassesExactApplicableEvidence(t *testing.T) {
	plan := testPlan("ci.test")
	evidence := []core.EvidenceRef{{
		ID:            "ev-1",
		RequirementID: "req-1",
		SubjectDigest: "sha256:subject",
		Procedure:     "ci.test",
		Result:        "PASS",
		Applicable:    true,
	}}
	got := Evaluate(plan, "sha256:subject", evidence)
	if got.Result != "PASS" {
		t.Fatalf("expected PASS, got %s", got.Result)
	}
}

func TestWrongProcedureCannotSatisfyFrozenPlan(t *testing.T) {
	plan := testPlan("ci.test")
	evidence := []core.EvidenceRef{{
		ID:            "ev-1",
		RequirementID: "req-1",
		SubjectDigest: "sha256:subject",
		Procedure:     "runtime.claim",
		Result:        "PASS",
		Applicable:    true,
	}}
	got := Evaluate(plan, "sha256:subject", evidence)
	if got.Result != "FAIL" {
		t.Fatalf("expected FAIL for wrong procedure, got %s", got.Result)
	}
}


func TestVerificationRequirementIDCannotBeSubstitutedBySameProcedure(t *testing.T) {
	plan := Plan{ID: "vp-two", Criteria: []Criterion{{
		ID: "ac-two", Statement: "two facts", Requirements: []EvidenceRequirement{
			{ID: "req-a", Procedure: "ci.test"},
			{ID: "req-b", Procedure: "ci.test"},
		},
	}}}
	evidence := []core.EvidenceRef{{
		ID:            "ev-a",
		RequirementID: "req-a",
		SubjectDigest: "sha256:subject",
		Procedure:     "ci.test",
		Result:        "PASS",
		Applicable:    true,
	}}
	got := Evaluate(plan, "sha256:subject", evidence)
	if got.Result != "FAIL" {
		t.Fatalf("one exact requirement must not satisfy another with the same procedure: %#v", got)
	}
}

func TestValidatePlanRejectsDuplicateCriterionAndRequirementIDs(t *testing.T) {
	duplicateRequirement := Plan{ID: "vp", Criteria: []Criterion{
		{ID: "a", Statement: "one", Requirements: []EvidenceRequirement{{ID: "same", Procedure: "ci.one"}}},
		{ID: "b", Statement: "two", Requirements: []EvidenceRequirement{{ID: "same", Procedure: "ci.two"}}},
	}}
	if ValidatePlan(duplicateRequirement, []string{"one", "two"}) {
		t.Fatal("duplicate requirement ID accepted")
	}
	duplicateCriterion := Plan{ID: "vp", Criteria: []Criterion{
		{ID: "same", Statement: "one", Requirements: []EvidenceRequirement{{ID: "r1", Procedure: "ci.one"}}},
		{ID: "same", Statement: "two", Requirements: []EvidenceRequirement{{ID: "r2", Procedure: "ci.two"}}},
	}}
	if ValidatePlan(duplicateCriterion, []string{"one", "two"}) {
		t.Fatal("duplicate criterion ID accepted")
	}
}
