package routing

import (
	"fmt"
	"strings"
)

type Route struct {
	TaskType      string   `json:"task_type"`
	CapabilityIDs []string `json:"capability_ids"`
	SkillIDs      []string `json:"skill_ids"`
}

func Resolve(taskType, subsystem string) (Route, error) {
	task := strings.ToUpper(taskType)
	sub := strings.ToLower(subsystem)

	switch task {
	case "DEBUG":
		if strings.Contains(sub, "linux") || strings.Contains(sub, "bsp") || strings.Contains(sub, "storage") || strings.Contains(sub, "ubi") {
			return Route{
				TaskType:      task,
				CapabilityIDs: []string{"embedded.debug-reliability", "embedded.linux-bsp"},
				SkillIDs:      []string{"material-readiness", "log-triage", "linux-bsp-debug"},
			}, nil
		}
		if strings.Contains(sub, "mcu") || strings.Contains(sub, "rtos") || strings.Contains(sub, "motor") {
			return Route{
				TaskType:      task,
				CapabilityIDs: []string{"embedded.debug-reliability", "embedded.mcu-rtos"},
				SkillIDs:      []string{"material-readiness", "log-triage", "mcu-rtos-debug"},
			}, nil
		}
		return Route{
			TaskType:      task,
			CapabilityIDs: []string{"embedded.debug-reliability"},
			SkillIDs:      []string{"material-readiness", "log-triage"},
		}, nil

	case "FEATURE", "BRINGUP", "COMPATIBILITY":
		route := Route{
			TaskType:      task,
			CapabilityIDs: []string{"embedded.architecture", "embedded.driver-component"},
			SkillIDs:      []string{"material-readiness", "architecture-impact-analysis", "interface-contract-review", "driver-integration-review"},
		}
		if strings.Contains(sub, "linux") || strings.Contains(sub, "bsp") || strings.Contains(sub, "storage") || strings.Contains(sub, "ubi") {
			route.CapabilityIDs = append(route.CapabilityIDs, "embedded.linux-bsp")
			route.SkillIDs = append(route.SkillIDs, "linux-bsp-integration")
		} else if strings.Contains(sub, "mcu") || strings.Contains(sub, "rtos") || strings.Contains(sub, "motor") {
			route.CapabilityIDs = append(route.CapabilityIDs, "embedded.mcu-rtos")
			route.SkillIDs = append(route.SkillIDs, "mcu-rtos-integration")
		}
		return route, nil

	case "DEVICE_TEST", "RELEASE":
		return Route{
			TaskType:      task,
			CapabilityIDs: []string{"embedded.verification"},
			SkillIDs:      []string{"material-readiness", "verification-plan-builder"},
		}, nil

	case "PERFORMANCE", "REFACTOR":
		return Route{
			TaskType:      task,
			CapabilityIDs: []string{"embedded.debug-reliability", "embedded.verification"},
			SkillIDs:      []string{"material-readiness", "verification-plan-builder"},
		}, nil
	default:
		return Route{}, fmt.Errorf("unsupported task type %q", taskType)
	}
}

// TargetContext is a caller-declared classification for a frozen material TargetID.
// It is not inferred from marketing part numbers and does not grant Device authority.
type TargetContext struct {
	TargetID string `json:"target_id"`
	Platform string `json:"platform"`
}

const (
	PlatformLinuxBSP = "linux-bsp"
	PlatformMCURTOS  = "mcu-rtos"
)

// ResolveForTarget augments the existing legacy-free, explicit routing contract.
// If a typed target is supplied, it must be unambiguous and must not conflict
// with explicit subsystem hints. The caller MUST also compare TargetID with
// the immutable MaterialManifest TargetID before creating the Task.
func ResolveForTarget(taskType, subsystem string, target *TargetContext) (Route, error) {
	if target == nil {
		return Resolve(taskType, subsystem)
	}
	if target.TargetID == "" || len(target.TargetID) > 128 ||
		target.TargetID != strings.TrimSpace(target.TargetID) ||
		strings.ContainsAny(target.TargetID, "\x00\r\n\t") {
		return Route{}, fmt.Errorf("target_context requires bounded exact target_id")
	}
	if target.Platform != PlatformLinuxBSP && target.Platform != PlatformMCURTOS {
		return Route{}, fmt.Errorf("unsupported target_context platform %q", target.Platform)
	}
	sub := strings.ToLower(subsystem)
	linux := strings.Contains(sub, "linux") || strings.Contains(sub, "bsp") ||
		strings.Contains(sub, "storage") || strings.Contains(sub, "ubi")
	mcu := strings.Contains(sub, "mcu") || strings.Contains(sub, "rtos") ||
		strings.Contains(sub, "motor")
	if linux && mcu {
		return Route{}, fmt.Errorf("ambiguous subsystem families for declared target")
	}
	if (linux && target.Platform != PlatformLinuxBSP) ||
		(mcu && target.Platform != PlatformMCURTOS) {
		return Route{}, fmt.Errorf("subsystem conflicts with declared target platform")
	}
	// Route through the same frozen Task/Capability/Skill contract. Opaque chip
	// labels are never parsed, and the Task digest still binds selected Skills.
	return Resolve(taskType, target.Platform)
}
