package material

import "testing"

func TestDebugRequiresEvidence(t *testing.T) {
	m := Manifest{
		TaskType:           "DEBUG",
		Repository:         "repo",
		BaseCommit:         "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"root cause supported by evidence"},
	}
	r := Evaluate(m)
	if r.Status != Blocked {
		t.Fatalf("expected BLOCKED, got %s", r.Status)
	}
	m.HasAuthoritativeLog = true
	r = Evaluate(m)
	if r.Status != Ready {
		t.Fatalf("expected READY, got %s: %v", r.Status, r.Reasons)
	}
}

func TestDebugDegradationRequiresApproverAndReason(t *testing.T) {
	m := Manifest{
		TaskType:           "DEBUG",
		Repository:         "repo",
		BaseCommit:         "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"collect enough evidence to decide next step"},
	}
	if got := Evaluate(m).Status; got != Blocked {
		t.Fatalf("expected BLOCKED without degradation approval, got %s", got)
	}
	m.DegradationApprovedBy = "tech-lead"
	if got := Evaluate(m).Status; got != Blocked {
		t.Fatalf("expected BLOCKED without degradation reason, got %s", got)
	}
	m.DegradationReason = "original device is unavailable; diagnostic run is explicitly exploratory"
	if got := Evaluate(m).Status; got != Degraded {
		t.Fatalf("expected DEGRADED with auditable approval, got %s", got)
	}
}

func TestDeviceTestRequiresExactDeviceAndFirmware(t *testing.T) {
	m := Manifest{
		TaskType:           "DEVICE_TEST",
		Repository:         "repo",
		BaseCommit:         "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"passes HIL"},
	}
	if got := Evaluate(m).Status; got != Blocked {
		t.Fatalf("expected BLOCKED, got %s", got)
	}
	m.DeviceID = "dut-001"
	m.FirmwareIdentity = "sha256:abc"
	if got := Evaluate(m).Status; got != Ready {
		t.Fatalf("expected READY, got %s", got)
	}
}

func TestDegradationCannotBypassExactSourceIdentity(t *testing.T) {
	m := Manifest{
		TaskType:              "FEATURE",
		Repository:            "repo",
		BaseCommit:            "main",
		AcceptanceCriteria:    []string{"build passes"},
		DegradationApprovedBy: "tech-lead",
		DegradationReason:     "exploratory",
	}
	got := Evaluate(m)
	if got.Status != Blocked {
		t.Fatalf("expected hard BLOCKED despite degradation approval, got %s", got.Status)
	}
}
