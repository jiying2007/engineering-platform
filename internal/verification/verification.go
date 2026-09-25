package verification

import (
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

type EvidenceRequirement struct {
	ID        string `json:"requirement_id"`
	Procedure string `json:"procedure"`
	Issuer    string `json:"issuer,omitempty"`
}

type Criterion struct {
	ID           string                `json:"criterion_id"`
	Statement    string                `json:"statement"`
	Requirements []EvidenceRequirement `json:"evidence_requirements"`
}

type Plan struct {
	ID       string      `json:"verification_plan_id"`
	Criteria []Criterion `json:"criteria"`
}

func (p Plan) Digest() (string, error) {
	return canonical.Digest(p)
}

type CriterionResult struct {
	CriterionID string `json:"criterion_id"`
	Result      string `json:"result"`
	Reason      string `json:"reason,omitempty"`
}

type Report struct {
	ID                     string            `json:"verification_report_id"`
	DeliveryReceiptID      string            `json:"delivery_receipt_id"`
	PlanID                 string            `json:"verification_plan_id"`
	VerificationPlanDigest string            `json:"verification_plan_digest"`
	SubjectDigest          string            `json:"subject_digest"`
	Result                 string            `json:"result"`
	Verifier               string            `json:"verifier,omitempty"`
	Criteria               []CriterionResult `json:"criteria"`
	EvidenceIDs            []string          `json:"evidence_ids"`
	CreatedAt              time.Time         `json:"created_at"`
}

func ValidatePlan(plan Plan, acceptanceCriteria []string) bool {
	if plan.ID == "" || len(plan.Criteria) == 0 || len(plan.Criteria) != len(acceptanceCriteria) {
		return false
	}
	criterionIDs := make(map[string]bool, len(plan.Criteria))
	requirementIDs := map[string]bool{}
	expected := make(map[string]int, len(acceptanceCriteria))
	for _, ac := range acceptanceCriteria {
		if ac == "" {
			return false
		}
		expected[ac]++
	}
	for _, criterion := range plan.Criteria {
		if criterion.ID == "" || criterionIDs[criterion.ID] || criterion.Statement == "" || len(criterion.Requirements) == 0 {
			return false
		}
		criterionIDs[criterion.ID] = true
		if expected[criterion.Statement] == 0 {
			return false
		}
		expected[criterion.Statement]--
		for _, req := range criterion.Requirements {
			if req.ID == "" || requirementIDs[req.ID] || req.Procedure == "" {
				return false
			}
			requirementIDs[req.ID] = true
		}
	}
	for _, remaining := range expected {
		if remaining != 0 {
			return false
		}
	}
	return true
}

func Evaluate(plan Plan, subjectDigest string, evidence []core.EvidenceRef) Report {
	evidenceIDs := make([]string, 0, len(evidence))
	for _, item := range evidence {
		evidenceIDs = append(evidenceIDs, item.ID)
	}

	report := Report{
		PlanID:        plan.ID,
		SubjectDigest: subjectDigest,
		Result:        "PASS",
		EvidenceIDs:   evidenceIDs,
	}
	if subjectDigest == "" || len(plan.Criteria) == 0 {
		report.Result = "FAIL"
		return report
	}

	for _, criterion := range plan.Criteria {
		result := CriterionResult{CriterionID: criterion.ID, Result: "PASS"}
		if criterion.ID == "" || criterion.Statement == "" || len(criterion.Requirements) == 0 {
			result.Result = "FAIL"
			result.Reason = "criterion is incomplete"
		} else {
			for _, requirement := range criterion.Requirements {
				if !requirementSatisfied(requirement, subjectDigest, evidence) {
					result.Result = "FAIL"
					result.Reason = "required evidence requirement was not satisfied"
					break
				}
			}
		}
		if result.Result != "PASS" {
			report.Result = "FAIL"
		}
		report.Criteria = append(report.Criteria, result)
	}
	return report
}

func requirementSatisfied(requirement EvidenceRequirement, subjectDigest string, evidence []core.EvidenceRef) bool {
	if requirement.ID == "" || requirement.Procedure == "" {
		return false
	}
	for _, item := range evidence {
		if !item.Applicable || item.SubjectDigest != subjectDigest || item.Result != "PASS" {
			continue
		}
		if item.RequirementID != requirement.ID || item.Procedure != requirement.Procedure {
			continue
		}
		if requirement.Issuer != "" && item.Issuer != requirement.Issuer {
			continue
		}
		return true
	}
	return false
}
