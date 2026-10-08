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

func TestSkillContractsAreExplicitAndNotAutoPromoted(t *testing.T) {
	seen := map[string]bool{}
	owners := map[string]bool{}
	for _, c := range Capabilities() {
		owners[c.ID] = true
	}
	for _, s := range Skills() {
		if seen[s.ID] || !owners[s.OwnerCapability] || s.Version != 1 ||
			s.Purpose == "" || len(s.InputContract) == 0 ||
			len(s.RequiredMaterial) == 0 || len(s.Method) == 0 ||
			len(s.OutputContract) == 0 || len(s.BlockConditions) == 0 ||
			s.AllowedActions == nil || len(s.AllowedActions) != 0 ||
			len(s.ProhibitedActions) == 0 || s.EvaluationMethod == "" ||
			s.Maturity != "DEFINED" {
			t.Fatalf("incomplete or falsely promoted Skill: %#v", s)
		}
		seen[s.ID] = true
	}
	if len(seen) != 10 {
		t.Fatalf("unexpected number of defined Skill contracts: %d", len(seen))
	}
}

func TestCatalogReturnValuesAreIndependentDeepCopies(t *testing.T) {
	originalSkills, originalCaps := Skills(), Capabilities()
	a, b := Skills(), Capabilities()
	a[0].InputContract[0] = "tampered"
	a[0].Method[0] = "tampered"
	a[0].RequiredMaterial[0] = "tampered"
	a[0].OutputContract[0] = "tampered"
	a[0].ProhibitedActions[0] = "tampered"
	b[0].SkillIDs[0] = "tampered"
	a[0].Maturity = "PROVEN"
	if got := Skills(); got[0].InputContract[0] != originalSkills[0].InputContract[0] ||
		got[0].Method[0] != originalSkills[0].Method[0] ||
		got[0].RequiredMaterial[0] != originalSkills[0].RequiredMaterial[0] ||
		got[0].OutputContract[0] != originalSkills[0].OutputContract[0] ||
		got[0].ProhibitedActions[0] != originalSkills[0].ProhibitedActions[0] ||
		got[0].Maturity != originalSkills[0].Maturity {
		t.Fatal("global Skill catalog mutated through returned value")
	}
	if got := Capabilities(); got[0].SkillIDs[0] != originalCaps[0].SkillIDs[0] {
		t.Fatal("global Capability catalog mutated through returned value")
	}
}
