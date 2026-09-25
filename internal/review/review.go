package review

import (
	"fmt"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const (
	ResultPass = "PASS"
	ResultFail = "FAIL"

	SeverityInfo     = "INFO"
	SeverityWarning  = "WARNING"
	SeverityBlocking = "BLOCKING"
)

type Finding struct {
	ID       string `json:"finding_id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

type Report struct {
	ID                   string    `json:"review_report_id"`
	DeliveryReceiptID    string    `json:"delivery_receipt_id"`
	VerificationReportID string    `json:"verification_report_id"`
	TaskContractDigest   string    `json:"task_contract_digest"`
	SubjectDigest        string    `json:"subject_digest"`
	Reviewer             string    `json:"reviewer"`
	Result               string    `json:"result"`
	Findings             []Finding `json:"findings,omitempty"`
	KnownLimits          []string  `json:"known_limits,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

func (r Report) Validate() error {
	if !bounded(r.ID, 256) || !bounded(r.DeliveryReceiptID, 256) || !bounded(r.VerificationReportID, 256) ||
		!canonical.ValidDigest(r.TaskContractDigest) || !canonical.ValidDigest(r.SubjectDigest) ||
		!bounded(r.Reviewer, 256) || (r.Result != ResultPass && r.Result != ResultFail) || r.CreatedAt.IsZero() {
		return fmt.Errorf("invalid review identity")
	}
	if len(r.Findings) > 64 || len(r.KnownLimits) > 64 {
		return fmt.Errorf("review collection limit exceeded")
	}
	seen := map[string]bool{}
	blocking := 0
	for _, finding := range r.Findings {
		if !bounded(finding.ID, 256) || !bounded(finding.Summary, 4096) || seen[finding.ID] {
			return fmt.Errorf("invalid or duplicate review finding")
		}
		seen[finding.ID] = true
		switch finding.Severity {
		case SeverityInfo, SeverityWarning:
		case SeverityBlocking:
			blocking++
		default:
			return fmt.Errorf("invalid review severity")
		}
	}
	for _, limit := range r.KnownLimits {
		if !bounded(limit, 4096) {
			return fmt.Errorf("invalid review known limit")
		}
	}
	if r.Result == ResultPass && blocking != 0 {
		return fmt.Errorf("PASS review cannot contain blocking findings")
	}
	if r.Result == ResultFail && blocking == 0 {
		return fmt.Errorf("FAIL review requires a blocking finding")
	}
	return nil
}

func (r Report) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(r)
}

func bounded(value string, max int) bool {
	return value != "" && len(value) <= max && strings.TrimSpace(value) == value
}
