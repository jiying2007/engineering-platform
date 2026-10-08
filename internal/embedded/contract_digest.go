package embedded

import (
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// SkillContractDigestVersion is a content identity, not a maturity rating or
// execution permission. It binds only the Skills selected by the frozen route.
const SkillContractDigestVersion = 1

type skillContractSet struct {
	Version int     `json:"version"`
	Skills  []Skill `json:"skills"`
}

// RoutedSkillContractDigest freezes exact v1 Skill methods, inputs, outputs,
// negative conditions, evaluation policy and DEFINED maturity in route order.
// It does not authorize tools; Core / Action Gateway remain sole authorities.
func RoutedSkillContractDigest(ids []string) (string, error) {
	return digestRoutedSkills(ids, Skills())
}

// RoutedSkillContracts returns defensive copies of the exact ordered records
// whose metadata and version are already bound in TaskContract. It does not
// install a model Skill, grant tool authority or attest engineering quality.
func RoutedSkillContracts(ids []string) ([]Skill, string, error) {
	return selectedSkillContracts(ids, Skills())
}

func digestRoutedSkills(ids []string, catalog []Skill) (string, error) {
	_, digest, err := selectedSkillContracts(ids, catalog)
	return digest, err
}

func selectedSkillContracts(ids []string, catalog []Skill) ([]Skill, string, error) {
	if len(ids) == 0 || len(catalog) == 0 || len(ids) > len(catalog) || len(catalog) > 256 {
		return nil, "", fmt.Errorf("bounded nonempty selected Skill contract set required")
	}
	known := make(map[string]Skill, len(catalog))
	for _, skill := range catalog {
		if skill.ID == "" || skill.Version <= 0 || skill.OwnerCapability == "" ||
			skill.Purpose == "" || skill.Maturity == "" || skill.EvaluationMethod == "" ||
			len(skill.InputContract) == 0 || len(skill.RequiredMaterial) == 0 ||
			len(skill.Method) == 0 || len(skill.OutputContract) == 0 ||
			len(skill.BlockConditions) == 0 || len(skill.ProhibitedActions) == 0 {
			return nil, "", fmt.Errorf("invalid catalog Skill contract")
		}
		if _, exists := known[skill.ID]; exists {
			return nil, "", fmt.Errorf("duplicate catalog Skill %q", skill.ID)
		}
		known[skill.ID] = skill
	}
	selected := make([]Skill, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		skill, exists := known[id]
		if !exists || seen[id] {
			return nil, "", fmt.Errorf("unknown or repeated routed Skill %q", id)
		}
		seen[id] = true
		selected = append(selected, skill)
	}
	digest, err := canonical.Digest(skillContractSet{Version: SkillContractDigestVersion, Skills: selected})
	if err != nil {
		return nil, "", err
	}
	return selected, digest, nil
}
