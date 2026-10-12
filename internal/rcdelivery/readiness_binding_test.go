package rcdelivery

import (
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestReadinessMustMatchOriginalStatusBytes(t *testing.T) {
	status := readinessTestStatus()
	e, err := Build(fixture(t), strings.Repeat("b", 40), status, []byte("{}\n"), []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.VerifyStatusBytes(status); err != nil {
		t.Fatal(err)
	}
	// A self-consistent digest cannot substitute another declared posture for
	// the original status document. Byte readback remains a separate gate.
	e.Delivery.Readiness.InternalReadiness = "CANDIDATE_ONLY"
	e.Delivery.Readiness.InternalBlockers = []int{}
	e.DeliveryDigest, err = canonical.Digest(e.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	if e.VerifyStatusBytes(status) == nil {
		t.Fatal("self-consistent but substituted readiness accepted")
	}
}
