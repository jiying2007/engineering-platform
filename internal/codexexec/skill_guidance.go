package codexexec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/routing"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// selectedSkillsForTask enforces the explicit new v1 delivery contract.
// Older typed Tasks may already contain a Skill-contract digest but were
// created before guidance injection existed. Version 0 MUST preserve their
// exact name-only historical prompt without retroactive requalification.
func selectedSkillsForTask(task core.TaskContract) ([]embedded.Skill, error) {
	if task.SkillGuidanceVersion == 0 {
		return nil, nil
	}
	if task.SkillGuidanceVersion != 1 {
		return nil, fmt.Errorf("%w: unknown Skill guidance delivery version", workerqueue.ErrIdentity)
	}
	if !canonical.ValidDigest(task.SkillContractDigest) ||
		task.TargetID == "" ||
		(task.TargetPlatform != routing.PlatformLinuxBSP && task.TargetPlatform != routing.PlatformMCURTOS) {
		return nil, fmt.Errorf("%w: invalid typed Skill contract identity", workerqueue.ErrIdentity)
	}
	// Core routes from a reviewed TargetContext. Before any new model turn,
	// independently re-derive that route from frozen TaskType and platform.
	// A self-consistent Task/Intent/Preparation and an otherwise valid Skill
	// digest must not smuggle another platform's methods into this Task.
	route, err := routing.ResolveForTarget(task.TaskType, "", &routing.TargetContext{
		TargetID: task.TargetID, Platform: task.TargetPlatform,
	})
	if err != nil || !slices.Equal(task.CapabilityIDs, route.CapabilityIDs) ||
		!slices.Equal(task.SkillIDs, route.SkillIDs) {
		return nil, fmt.Errorf("%w: typed Task Capability/Skill route differs from frozen TargetContext", workerqueue.ErrIdentity)
	}
	skills, digest, err := embedded.RoutedSkillContracts(task.SkillIDs)
	if err != nil || digest != task.SkillContractDigest {
		return nil, fmt.Errorf("%w: selected Skill methods differ from frozen Task: %v", workerqueue.ErrIdentity, err)
	}
	return skills, nil
}

// appendSelectedSkillMethods places bounded, host-owned engineering methodology
// inside the actual Codex text turn; Skill text is guidance, never a model tool
// grant. Core, preparation, ToolProfile and Action Gateway remain authoritative.
func appendSelectedSkillMethods(b *strings.Builder, task core.TaskContract, skills []embedded.Skill) {
	fmt.Fprintf(b, "\nSelected Skill methods (contract v%d; frozen digest: %s):\n", embedded.SkillContractDigestVersion, task.SkillContractDigest)
	fmt.Fprintf(b, "Use these as engineering reasoning and reporting methods, not permission to use tools.\n")
	fmt.Fprintf(b, "When any required material or blocking condition is unverified, state the gap and do not fabricate evidence or assert verification success.\n")
	fmt.Fprintf(b, "The frozen Task, approved context and existing tool/action policies remain the only execution constraints.\n")
	for i, skill := range skills {
		fmt.Fprintf(b, "\n%d) Skill %s v%d [%s; %s]\n", i+1, skill.ID, skill.Version, skill.OwnerCapability, skill.Maturity)
		fmt.Fprintf(b, "Purpose: %s\n", skill.Purpose)
		fmt.Fprintf(b, "Inputs: %s\n", strings.Join(skill.InputContract, "; "))
		fmt.Fprintf(b, "Required materials: %s\n", strings.Join(skill.RequiredMaterial, "; "))
		fmt.Fprintf(b, "Method: %s\n", strings.Join(skill.Method, "; "))
		fmt.Fprintf(b, "Expected outputs: %s\n", strings.Join(skill.OutputContract, "; "))
		fmt.Fprintf(b, "Block conditions: %s\n", strings.Join(skill.BlockConditions, "; "))
		fmt.Fprintf(b, "Prohibited actions: %s\n", strings.Join(skill.ProhibitedActions, "; "))
		fmt.Fprintf(b, "Evaluation method (not a result): %s\n", skill.EvaluationMethod)
	}
}
