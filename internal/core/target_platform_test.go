package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTargetPlatformBindsTaskDigestWithoutChangingHistoricalEmptyField(t *testing.T) {
	task := TaskContract{
		ID: "task-1", WorkItemID: "work-1", TaskType: "RELEASE",
		CapabilityIDs: []string{"embedded.verification"},
		SkillIDs:      []string{"material-readiness", "verification-plan-builder"},
		Repository:    "repo", BaseCommit: strings.Repeat("a", 40),
		TargetID: "target-a", AcceptanceCriteria: []string{"test passes"},
		VerificationPlanID: "plan-1", VerificationPlanDigest: "sha256:" + strings.Repeat("b", 64), Revision: 1,
	}
	oldDigest, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(task)
	if err != nil || strings.Contains(string(raw), "target_platform") {
		t.Fatal("historical empty field must remain absent", err)
	}
	for _, platform := range []string{"linux-bsp", "mcu-rtos"} {
		task.TargetPlatform = platform
		newDigest, err := task.Digest()
		if err != nil || newDigest == oldDigest {
			t.Fatal("typed platform did not bind Task digest", platform, err)
		}
	}
	task.TargetPlatform = ""
	actual, err := task.Digest()
	if err != nil || actual != oldDigest {
		t.Fatal("historical Task digest changed with empty optional platform", err)
	}
}
