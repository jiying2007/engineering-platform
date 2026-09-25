package review

import (
	"fmt"
	"strings"
	"time"
)

const (
	Pass   = "PASS"
	Reject = "REJECT"
)

type Report struct {
	ID                   string    `json:"review_report_id"`
	DeliveryReceiptID    string    `json:"delivery_receipt_id"`
	VerificationReportID string    `json:"verification_report_id"`
	SubjectDigest        string    `json:"subject_digest"`
	Reviewer             string    `json:"reviewer"`
	Result               string    `json:"result"`
	Findings             []string  `json:"findings,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

func Validate(report Report) error {
	for name, value := range map[string]string{
		"review_report_id":       report.ID,
		"delivery_receipt_id":     report.DeliveryReceiptID,
		"verification_report_id": report.VerificationReportID,
		"subject_digest":          report.SubjectDigest,
		"reviewer":                report.Reviewer,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 512 {
			return fmt.Errorf("%s is required and bounded", name)
		}
	}
	if report.Result != Pass && report.Result != Reject {
		return fmt.Errorf("review result must be PASS or REJECT")
	}
	if len(report.Findings) > 64 {
		return fmt.Errorf("review findings exceed limit")
	}
	for _, finding := range report.Findings {
		if strings.TrimSpace(finding) == "" || len(finding) > 2048 {
			return fmt.Errorf("review finding must be nonblank and bounded")
		}
	}
	if report.Result == Reject && len(report.Findings) == 0 {
		return fmt.Errorf("REJECT review requires at least one finding")
	}
	if report.CreatedAt.IsZero() {
		return fmt.Errorf("review created_at is required")
	}
	return nil
}
