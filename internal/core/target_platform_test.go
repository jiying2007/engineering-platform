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
	raw, err = json.Marshal(task)
	if err != nil || strings.Contains(string(raw), "skill_contract_digest") {
		t.Fatal("historical Skill contract field must remain absent", err)
	}
	task.SkillContractDigest = "sha256:" + strings.Repeat("c", 64)
	bound, err := task.Digest()
	if err != nil || bound == oldDigest {
		t.Fatal("selected Skill contract did not bind Task digest", err)
	}
	task.SkillContractDigest = ""
	restored, err := task.Digest()
	if err != nil || restored != oldDigest {
		t.Fatal("historical Task digest was not restored", err)
	}
	// Existing typed Tasks were created before Skill-guidance injection was
	// introduced. The optional version field MUST not alter their identity.
	raw, err = json.Marshal(task)
	if err != nil || strings.Contains(string(raw), "skill_guidance_version") {
		t.Fatal("historical guidance version must be omitted", err)
	}
	task.SkillGuidanceVersion = 1
	newDigest, err := task.Digest()
	if err != nil || newDigest == oldDigest {
		t.Fatal("new guidance delivery version did not bind Task digest", err)
	}
	task.SkillGuidanceVersion = 0
	back, err := task.Digest()
	if err != nil || back != oldDigest {
		t.Fatal("historical Task guidance identity changed", err)
	}
}
