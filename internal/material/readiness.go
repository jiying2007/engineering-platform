package material

import (
	"fmt"
	"strings"
)

type Status string

const (
	Ready    Status = "READY"
	Blocked  Status = "BLOCKED"
	Degraded Status = "DEGRADED"
)

type Manifest struct {
	TaskType             string   `json:"task_type"`
	Repository           string   `json:"repository"`
	BaseCommit           string   `json:"base_commit"`
	TargetID             string   `json:"target_id,omitempty"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	HasAuthoritativeLog  bool     `json:"has_authoritative_log,omitempty"`
	HasReproduction      bool     `json:"has_reproduction,omitempty"`
	RequiresDevice       bool     `json:"requires_device,omitempty"`
	DeviceID             string   `json:"device_id,omitempty"`
	FirmwareIdentity     string   `json:"firmware_identity,omitempty"`
	DegradationApproved  bool     `json:"degradation_approved,omitempty"`
}

type Result struct {
	Status  Status   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
}

func Evaluate(m Manifest) Result {
	var reasons []string
	if m.Repository == "" {
		reasons = append(reasons, "missing repository")
	}
	if !isFullCommit(m.BaseCommit) {
		reasons = append(reasons, "missing exact full commit SHA")
	}
	if len(m.AcceptanceCriteria) == 0 {
		reasons = append(reasons, "missing acceptance criteria")
	}
	switch strings.ToUpper(m.TaskType) {
	case "DEBUG":
		if !m.HasAuthoritativeLog && !m.HasReproduction {
			reasons = append(reasons, "debug requires authoritative log or reproduction")
		}
	case "DEVICE_TEST":
		if m.DeviceID == "" {
			reasons = append(reasons, "device test requires exact device identity")
		}
		if m.FirmwareIdentity == "" {
			reasons = append(reasons, "device test requires exact firmware identity")
		}
	}
	if m.RequiresDevice && m.DeviceID == "" {
		reasons = append(reasons, "task requires exact device identity")
	}
	if len(reasons) == 0 {
		return Result{Status: Ready}
	}
	if m.DegradationApproved {
		return Result{Status: Degraded, Reasons: reasons}
	}
	return Result{Status: Blocked, Reasons: reasons}
}

func isFullCommit(v string) bool {
	if len(v) != 40 {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func (r Result) Error() error {
	if r.Status == Ready {
		return nil
	}
	return fmt.Errorf("%s: %s", r.Status, strings.Join(r.Reasons, "; "))
}
