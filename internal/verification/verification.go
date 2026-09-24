package verification

import (
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
)

type Criterion struct {
	ID               string   `json:"criterion_id"`
	Statement        string   `json:"statement"`
	RequiredEvidence []string `json:"required_evidence_ids"`
}

type Plan struct {
	ID            string      `json:"verification_plan_id"`
	SubjectDigest string      `json:"subject_digest"`
	Criteria      []Criterion `json:"criteria"`
}

type CriterionResult struct {
	CriterionID string `json:"criterion_id"`
	Result      string `json:"result"`
	Reason      string `json:"reason,omitempty"`
}

type Report struct {
	ID            string            `json:"verification_report_id"`
	PlanID        string            `json:"verification_plan_id"`
	SubjectDigest string            `json:"subject_digest"`
	Result        string            `json:"result"`
	Verifier      string            `json:"verifier,omitempty"`
	Criteria      []CriterionResult `json:"criteria"`
	EvidenceIDs   []string          `json:"evidence_ids"`
	CreatedAt     time.Time         `json:"created_at"`
}

func Evaluate(plan Plan, evidence []core.EvidenceRef) Report {
	byID := make(map[string]core.EvidenceRef, len(evidence))
	evidenceIDs := make([]string, 0, len(evidence))
	for _, item := range evidence {
		byID[item.ID] = item
		evidenceIDs = append(evidenceIDs, item.ID)
	}

	report := Report{
		PlanID:        plan.ID,
		SubjectDigest: plan.SubjectDigest,
		Result:        "PASS",
		EvidenceIDs:   evidenceIDs,
	}
	if plan.SubjectDigest == "" || len(plan.Criteria) == 0 {
		report.Result = "FAIL"
		return report
	}

	for _, criterion := range plan.Criteria {
		result := CriterionResult{CriterionID: criterion.ID, Result: "PASS"}
		if criterion.ID == "" {
			result.Result = "FAIL"
			result.Reason = "criterion_id is required"
		} else if len(criterion.RequiredEvidence) == 0 {
			result.Result = "FAIL"
			result.Reason = "criterion has no required evidence"
		}

		for _, evidenceID := range criterion.RequiredEvidence {
			item, ok := byID[evidenceID]
			if !ok {
				result.Result = "FAIL"
				result.Reason = "required evidence missing"
				break
			}
			if !item.Applicable || item.SubjectDigest != plan.SubjectDigest {
				result.Result = "FAIL"
				result.Reason = "evidence is stale or bound to a different subject"
				break
			}
			if item.Result != "PASS" {
				result.Result = "FAIL"
				result.Reason = "required evidence did not pass"
				break
			}
		}
		if result.Result != "PASS" {
			report.Result = "FAIL"
		}
		report.Criteria = append(report.Criteria, result)
	}
	return report
}
