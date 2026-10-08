package embedded

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestRoutedSkillDigestBindsSelectedVersionsMethodsAndEvaluation(t *testing.T) {
	ids := []string{"material-readiness", "log-triage", "linux-bsp-debug"}
	base, err := RoutedSkillContractDigest(ids)
	if err != nil || !canonical.ValidDigest(base) {
		t.Fatal("invalid routed digest", base, err)
	}
	catalog := Skills()
	again, err := digestRoutedSkills(ids, catalog)
	if err != nil || again != base {
		t.Fatal("non-deterministic catalog digest", again, err)
	}
	mutations := []func(*Skill){
		func(s *Skill) { s.Version++ },
		func(s *Skill) { s.Method[0] += " changed" },
		func(s *Skill) { s.RequiredMaterial[0] += " changed" },
		func(s *Skill) { s.EvaluationMethod += " changed" },
		func(s *Skill) { s.Maturity = "PROVEN" },
		func(s *Skill) { s.ProhibitedActions[0] += " changed" },
	}
	for i, mutate := range mutations {
		copyCatalog := Skills()
		mutate(&copyCatalog[0])
		next, err := digestRoutedSkills(ids, copyCatalog)
		if err != nil || next == base {
			t.Fatalf("selected catalog drift %d not detected: %s %v", i, next, err)
		}
	}
	unrelated := Skills()
	for i := range unrelated {
		if unrelated[i].ID == "mcu-rtos-integration" {
			unrelated[i].Method[0] += " unrelated"
		}
	}
	unchanged, err := digestRoutedSkills(ids, unrelated)
	if err != nil || unchanged != base {
		t.Fatal("unselected Skill incorrectly changes selected digest", unchanged, err)
	}
	reordered, err := RoutedSkillContractDigest([]string{"log-triage", "material-readiness", "linux-bsp-debug"})
	if err != nil || reordered == base {
		t.Fatal("route order drift not detected", err)
	}
}

func TestRoutedSkillDigestRejectsAmbiguousOrInvalidCatalog(t *testing.T) {
	for _, ids := range [][]string{
		nil, {}, {"not-a-real-skill"},
		{"material-readiness", "material-readiness"},
	} {
		if got, err := RoutedSkillContractDigest(ids); err == nil {
			t.Fatal("invalid route admitted", ids, got)
		}
	}
	catalog := Skills()
	catalog = append(catalog, catalog[0])
	if _, err := digestRoutedSkills([]string{"material-readiness"}, catalog); err == nil {
		t.Fatal("duplicate catalog identity admitted")
	}
	catalog = Skills()
	catalog[0].Method = nil
	if _, err := digestRoutedSkills([]string{"material-readiness"}, catalog); err == nil {
		t.Fatal("missing Skill method admitted")
	}
}
