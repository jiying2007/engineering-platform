package embedded

import "testing"

func TestIntegrationSkillsBelongToSubsystemCapabilities(t *testing.T) {
	want := map[string]string{
		"linux-bsp-integration": "embedded.linux-bsp",
		"mcu-rtos-integration":  "embedded.mcu-rtos",
	}
	for _, skill := range Skills() {
		if owner, ok := want[skill.ID]; ok {
			if skill.OwnerCapability != owner {
				t.Fatalf("skill %q owner = %q, want %q", skill.ID, skill.OwnerCapability, owner)
			}
			delete(want, skill.ID)
		}
	}
	for skill := range want {
		t.Errorf("missing integration skill %q", skill)
	}
}

func TestSubsystemCapabilitiesListIntegrationSkills(t *testing.T) {
	want := map[string]string{
		"embedded.linux-bsp": "linux-bsp-integration",
		"embedded.mcu-rtos":  "mcu-rtos-integration",
	}
	for _, capability := range Capabilities() {
		skill, ok := want[capability.ID]
		if !ok {
			continue
		}
		for _, skillID := range capability.SkillIDs {
			if skillID == skill {
				delete(want, capability.ID)
				break
			}
		}
	}
	for capability := range want {
		t.Errorf("capability %q does not list its integration skill", capability)
	}
}
