package review

import (
	"strings"
	"testing"
	"time"
)

func validReport() Report {
	return Report{
		ID:                   "review-1",
		DeliveryReceiptID:    "delivery-1",
		VerificationReportID: "verification-1",
		TaskContractDigest:   "sha256:" + strings.Repeat("a", 64),
		SubjectDigest:        "sha256:" + strings.Repeat("b", 64),
		Reviewer:             "urn:engineering-platform:reviewer:one",
		Result:               ResultPass,
		KnownLimits:          []string{"host remains trusted"},
		CreatedAt:            time.Unix(1, 0).UTC(),
	}
}

func TestReportValidationAndDigest(t *testing.T) {
	report := validReport()
	digest, err := report.Digest()
	if err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("valid report rejected: %s %v", digest, err)
	}
	report.Findings = []Finding{{ID: "block", Severity: SeverityBlocking, Summary: "must fix"}}
	if err := report.Validate(); err == nil {
		t.Fatal("PASS review accepted a blocking finding")
	}
	report.Result = ResultFail
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	report.Findings = nil
	if err := report.Validate(); err == nil {
		t.Fatal("FAIL review accepted without blocking finding")
	}
}

func TestReportRejectsAmbiguousCollections(t *testing.T) {
	report := validReport()
	report.Findings = []Finding{
		{ID: "same", Severity: SeverityInfo, Summary: "one"},
		{ID: "same", Severity: SeverityWarning, Summary: "two"},
	}
	if err := report.Validate(); err == nil {
		t.Fatal("duplicate finding accepted")
	}
	report = validReport()
	report.Reviewer = " reviewer "
	if err := report.Validate(); err == nil {
		t.Fatal("ambiguous reviewer accepted")
	}
}
