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
		return Route{
			TaskType:      task,
			CapabilityIDs: []string{"embedded.architecture", "embedded.driver-component"},
			SkillIDs:      []string{"material-readiness", "architecture-impact-analysis", "interface-contract-review", "driver-integration-review"},
		}, nil

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
