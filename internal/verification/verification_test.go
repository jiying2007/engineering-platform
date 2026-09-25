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

func TestEvidenceArtifactRefsMustBelongToExactDelivery(t *testing.T) {
	delivery := core.DeliveryReceipt{Artifacts: []core.ArtifactRef{
		{ID: "firmware-a", Digest: "sha256:a"},
		{ID: "log-a", Digest: "sha256:b"},
	}}
	for _, tc := range []struct {
		name string
		refs []string
		want bool
	}{
		{name: "none", refs: nil, want: true},
		{name: "subset", refs: []string{"firmware-a"}, want: true},
		{name: "all", refs: []string{"firmware-a", "log-a"}, want: true},
		{name: "foreign", refs: []string{"firmware-b"}, want: false},
		{name: "duplicate", refs: []string{"firmware-a", "firmware-a"}, want: false},
		{name: "blank", refs: []string{""}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EvidenceArtifactsBelongToDelivery(delivery, core.EvidenceRef{ArtifactRefs: tc.refs})
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	duplicateDelivery := delivery
	duplicateDelivery.Artifacts = append(duplicateDelivery.Artifacts, core.ArtifactRef{ID: "firmware-a", Digest: "sha256:other"})
	if EvidenceArtifactsBelongToDelivery(duplicateDelivery, core.EvidenceRef{}) {
		t.Fatal("ambiguous duplicate delivery artifact IDs accepted")
	}
}


func TestGitHubActionsProcedureRequiresExactIssuer(t *testing.T) {
	plan := Plan{
		ID: "vp-github",
		Criteria: []Criterion{{
			ID:        "ac-github",
			Statement: "CI provenance verified",
			Requirements: []EvidenceRequirement{{
				ID:        "req-github",
				Procedure: GitHubActionsProcedure,
				Issuer:    GitHubActionsIssuer,
			}},
		}},
	}
	if !ValidatePlan(plan, []string{"CI provenance verified"}) {
		t.Fatal("exact GitHub Actions authority rejected")
	}
	plan.Criteria[0].Requirements[0].Issuer = ""
	if ValidatePlan(plan, []string{"CI provenance verified"}) {
		t.Fatal("GitHub Actions procedure accepted without frozen issuer")
	}
	plan.Criteria[0].Requirements[0].Issuer = "other-ci"
	if ValidatePlan(plan, []string{"CI provenance verified"}) {
		t.Fatal("GitHub Actions procedure accepted under another issuer")
	}
}
