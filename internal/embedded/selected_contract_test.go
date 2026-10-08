package embedded

import "testing"

func TestRoutedSkillContractsReturnExactIndependentRecords(t *testing.T) {
	ids := []string{"material-readiness", "log-triage", "linux-bsp-debug"}
	selected, digest, err := RoutedSkillContracts(ids)
	want, werr := RoutedSkillContractDigest(ids)
	if err != nil || werr != nil || digest != want || len(selected) != len(ids) {
		t.Fatalf("selected method identity drift: %v %v", err, werr)
	}
	for i, skill := range selected {
		if skill.ID != ids[i] || skill.Version != 1 || len(skill.Method) == 0 ||
			skill.EvaluationMethod == "" || skill.Maturity != "DEFINED" {
			t.Fatalf("method selection/maturity not exact: %#v", selected)
		}
	}
	selected[0].Method[0] = "untrusted mutation"
	selected[1].BlockConditions[0] = "untrusted mutation"
	selected[2].Maturity = "PROVEN"
	again, unchanged, err := RoutedSkillContracts(ids)
	if err != nil || unchanged != want ||
		again[0].Method[0] == "untrusted mutation" ||
		again[1].BlockConditions[0] == "untrusted mutation" ||
		again[2].Maturity != "DEFINED" {
		t.Fatal("caller mutated global Skill catalog", err)
	}
	for _, bad := range [][]string{nil, {}, {"unknown"}, {"log-triage", "log-triage"}} {
		if _, _, err := RoutedSkillContracts(bad); err == nil {
			t.Fatal("invalid selected method set admitted", bad)
		}
	}
}
