package rcdelivery

import (
	"strings"
	"testing"
)

// Also used by the existing delivery fixture tests after the schema-v2 change.
func readinessTestStatus() []byte {
	return []byte("# Implementation Status\n\n" + readinessStart + `{"version":1,"internal_readiness":"BLOCKED","internal_blockers":[228,229],"external_qualification_gates":[105],"deferred_credential_gates":[103,106]}` + readinessEnd + "\n")
}
func TestReadinessSnapshotCarriesInternalBlockers(t *testing.T) {
	s, err := ReadinessFromStatus(readinessTestStatus())
	if err != nil {
		t.Fatal(err)
	}
	if s.InternalReadiness != "BLOCKED" || len(s.InternalBlockers) != 2 || s.InternalBlockers[0] != 228 || s.InternalBlockers[1] != 229 || len(s.ExternalQualificationGates) != 1 || s.ExternalQualificationGates[0] != 105 {
		t.Fatal("lost internal/qualification distinction")
	}
}
func TestReadinessNeverInfersInternalClosure(t *testing.T) {
	valid := string(readinessTestStatus())
	cases := []string{
		"# Implementation Status\ncurrent\n", valid + valid,
		strings.Replace(valid, readinessEnd, "", 1),
		strings.Replace(valid, `"BLOCKED"`, `"READY"`, 1),
		strings.Replace(valid, `"BLOCKED"`, `"CANDIDATE_ONLY"`, 1),
		strings.Replace(valid, `[228,229]`, `[]`, 1),
		strings.Replace(valid, `[228,229]`, `null`, 1),
		strings.Replace(valid, `[228,229]`, `[229,228]`, 1),
		strings.Replace(valid, `[228,229]`, `[228,228]`, 1),
		strings.Replace(valid, `[228,229]`, `[0,229]`, 1),
		strings.Replace(valid, `[105]`, `[]`, 1),
		strings.Replace(valid, `[105]`, `[228]`, 1),
		strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(valid, `"internal_readiness"`, `"Internal_Readiness"`, 1),
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
	}
	for i, status := range cases {
		if _, err := ReadinessFromStatus([]byte(status)); err == nil {
			t.Fatalf("invalid readiness %d accepted", i)
		}
	}
	candidate := strings.Replace(strings.Replace(valid, `"BLOCKED"`, `"CANDIDATE_ONLY"`, 1), `[228,229]`, `[]`, 1)
	s, err := ReadinessFromStatus([]byte(candidate))
	if err != nil || s.InternalReadiness != "CANDIDATE_ONLY" {
		t.Fatalf("explicit candidate invalid: %+v %v", s, err)
	}
}
