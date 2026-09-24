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
	TaskType              string   `json:"task_type"`
	Repository            string   `json:"repository"`
	BaseCommit            string   `json:"base_commit"`
	TargetID              string   `json:"target_id,omitempty"`
	AcceptanceCriteria    []string `json:"acceptance_criteria"`
	HasAuthoritativeLog   bool     `json:"has_authoritative_log,omitempty"`
	HasReproduction       bool     `json:"has_reproduction,omitempty"`
	RequiresDevice        bool     `json:"requires_device,omitempty"`
	DeviceID              string   `json:"device_id,omitempty"`
	FirmwareIdentity      string   `json:"firmware_identity,omitempty"`
	DegradationApprovedBy string   `json:"degradation_approved_by,omitempty"`
	DegradationReason     string   `json:"degradation_reason,omitempty"`
}

type Result struct {
	Status  Status   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
}

func Evaluate(m Manifest) Result {
	var hard []string
	var degradable []string

	if m.Repository == "" {
		hard = append(hard, "missing repository")
	}
	if !isFullCommit(m.BaseCommit) {
		hard = append(hard, "missing exact full commit SHA")
	}
	if len(m.AcceptanceCriteria) == 0 {
		hard = append(hard, "missing acceptance criteria")
	}

	switch strings.ToUpper(m.TaskType) {
	case "DEBUG":
		if !m.HasAuthoritativeLog && !m.HasReproduction {
			degradable = append(degradable, "debug requires authoritative log or reproduction")
		}
	case "DEVICE_TEST":
		if m.DeviceID == "" {
			hard = append(hard, "device test requires exact device identity")
		}
		if m.FirmwareIdentity == "" {
			hard = append(hard, "device test requires exact firmware identity")
		}
	}
	if m.RequiresDevice && m.DeviceID == "" {
		hard = append(hard, "task requires exact device identity")
	}

	if len(hard) > 0 {
		return Result{Status: Blocked, Reasons: append(hard, degradable...)}
	}
	if len(degradable) == 0 {
		return Result{Status: Ready}
	}
	if m.DegradationApprovedBy != "" && m.DegradationReason != "" {
		return Result{Status: Degraded, Reasons: degradable}
	}
	return Result{Status: Blocked, Reasons: append(degradable, "degradation requires approver and reason")}
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
