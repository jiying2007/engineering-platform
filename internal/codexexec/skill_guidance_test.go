package codexexec

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// Rebind every nested frozen identity to reproduce an internally self-consistent
// malicious assignment. This distinguishes Skill-metadata rejection from
// ordinary TaskDigest or preparation/intent mismatches.
func rebindTypedPermit(t *testing.T, permit *Permit) {
	t.Helper()
	a := &permit.Assignment
	taskDigest, err := a.Task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	a.Input.TaskContractDigest = taskDigest
	inputDigest, err := a.Input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	a.Intent.TaskDigest = taskDigest
	a.Intent.InputDigest = inputDigest
	intentDigest, err := a.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	a.IntentDigest = intentDigest
	prep := &permit.Preparation
	prep.Facts.TaskDigest = taskDigest
	prep.Facts.InputDigest = inputDigest
	prep.Facts.IntentDigest = intentDigest
	prep.Facts.Context.RunInputDigest = inputDigest
	bundle, err := json.Marshal(prep.Facts.Context)
	if err != nil {
		t.Fatal(err)
	}
	prep.Facts.BundleDigest = canonical.BytesDigest(bundle)
	v, err := workerqueue.Validate(*a)
	if err != nil {
		t.Fatal(err)
	}
	prep.Admission.Validation = v
	prep.FactsDigest, err = canonical.Digest(prep.Facts)
	if err != nil {
		t.Fatal(err)
	}
}

func typedSkillPermit(t *testing.T) Permit {
	t.Helper()
	_, p := contractFixture(t)
	p.Assignment.Task.TargetID = "ssc305"
	p.Assignment.Task.TargetPlatform = "linux-bsp"
	p.Assignment.Task.CapabilityIDs = []string{"embedded.debug-reliability", "embedded.linux-bsp"}
	p.Assignment.Task.SkillIDs = []string{"material-readiness", "log-triage", "linux-bsp-debug"}
	digest, err := embedded.RoutedSkillContractDigest(p.Assignment.Task.SkillIDs)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignment.Task.SkillContractDigest = digest
	p.Assignment.Task.SkillGuidanceVersion = 1
	rebindTypedPermit(t, &p)
	return p
}

func TestTypedPromptIncludesFrozenEngineeringMethodsAndBlocks(t *testing.T) {
	p := typedSkillPermit(t)
	prompt, digest, err := Prompt(p.Assignment, p.Preparation, "/host-approved/context-bundle")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Selected Skill methods (contract v1; frozen digest:",
		p.Assignment.Task.SkillContractDigest,
		"Skill linux-bsp-debug v1",
		"locate last observed state and subsystem layer",
		"no authoritative symptom material",
		"normalize without rewriting raw data",
		"Evaluation method (not a result):",
		"Use these as engineering reasoning and reporting methods, not permission",
		"Block conditions:",
		"Prohibited actions:",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("typed prompt omitted selected method fact %q", required)
		}
	}
	if strings.Contains(prompt, "Skill mcu-rtos-debug") ||
		strings.Contains(prompt, "Skill mcu-rtos-integration") ||
		strings.Contains(prompt, "automatic Skill PROVEN") {
		t.Fatal("typed prompt contains unselected Skills or fake maturity")
	}
	other, otherDigest, err := Prompt(p.Assignment, p.Preparation, "/another/approved/bundle")
	if err != nil || other == prompt || otherDigest != digest {
		t.Fatal("prompt locator leaked into identity", otherDigest, digest, err)
	}
	expected, err := PromptIdentityDigest(p.Assignment, p.Preparation)
	if err != nil || expected != digest {
		t.Fatal("typed method identity not bound to Prompt receipt", err)
	}
}

func TestTypedPromptRejectsSelfConsistentCatalogMismatch(t *testing.T) {
	for _, mutation := range []func(*Permit){
		func(p *Permit) { p.Assignment.Task.SkillContractDigest = "sha256:" + strings.Repeat("0", 64) },
		func(p *Permit) { p.Assignment.Task.SkillContractDigest = "invalid" },
		func(p *Permit) { p.Assignment.Task.SkillContractDigest = "" },
		func(p *Permit) { p.Assignment.Task.SkillGuidanceVersion = 2 },
		func(p *Permit) { p.Assignment.Task.TargetPlatform = "unknown" },
		func(p *Permit) { p.Assignment.Task.TargetID = "" },
		func(p *Permit) { p.Assignment.Task.SkillIDs[1] = p.Assignment.Task.SkillIDs[0] },
		func(p *Permit) { p.Assignment.Task.SkillIDs[2] = "unknown-skill" },
	} {
		p := typedSkillPermit(t)
		mutation(&p)
		rebindTypedPermit(t, &p)
		// Archived receipt identity is derived solely from retained data.
		// It must remain independently auditable even when current catalog
		// verification would reject a NEW model turn.
		identity, err := PromptIdentityDigest(p.Assignment, p.Preparation)
		if err != nil || !canonical.ValidDigest(identity) {
			t.Fatalf("frozen prompt identity cannot be verified: %v", err)
		}
		if _, _, err := Prompt(p.Assignment, p.Preparation, "/approved"); !errors.Is(err, workerqueue.ErrIdentity) {
			t.Fatalf("malformed typed Skill method NEW turn accepted: %v", err)
		}
	}
}

func TestLegacyTypedTaskAndUntypedTaskKeepOriginalPrompt(t *testing.T) {
	_, legacy := contractFixture(t)
	prompt, digest, err := Prompt(legacy.Assignment, legacy.Preparation, "/approved")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "Selected Skill methods") || strings.Contains(prompt, "contract v1; frozen digest") {
		t.Fatal("untyped legacy prompt was silently extended")
	}
	expected := "You are executing one frozen embedded-software TaskContract.\n" +
		"Modify only the current repository workspace. Do not commit, push, fetch, install packages, or use network access.\n" +
		"Use local tools/tests as needed. Finish with a concise summary of changes and tests.\n\n" +
		"Run: run\nTask type: FEATURE\nRepository: repo\nBase commit: " +
		strings.Repeat("1", 40) + "\n" +
		"Approved context bundle: /approved\nContext bundle digest: " +
		legacy.Preparation.Facts.BundleDigest + "\n\n" +
		"Acceptance criteria:\n1. change code\n2. tests pass\n\n" +
		"Expected outputs:\n- source change\n"
	if prompt != expected {
		t.Fatalf("byte-stable legacy prompt changed:\n%s", prompt)
	}
	legacy.Assignment.Task.TargetID = "ssc305"
	legacy.Assignment.Task.TargetPlatform = "linux-bsp"
	legacy.Assignment.Task.SkillIDs = []string{"material-readiness", "log-triage", "linux-bsp-debug"}
	rebindTypedPermit(t, &legacy)
	before, historicalDigest, err := Prompt(legacy.Assignment, legacy.Preparation, "/approved")
	if err != nil || !strings.Contains(before, "Skills: material-readiness, log-triage, linux-bsp-debug") ||
		strings.Contains(before, "Selected Skill methods") || historicalDigest == digest {
		t.Fatal("historical #203 typed contract was upgraded without its Skill digest", err)
	}
	// Between #207 and this change, Core already froze SkillContractDigest
	// while the Codex turn still received name-only guidance. No silent prompt
	// upgrade or old PromptIdentity key is allowed for those stored Tasks.
	legacySkillDigest, err := embedded.RoutedSkillContractDigest(legacy.Assignment.Task.SkillIDs)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Assignment.Task.SkillContractDigest = legacySkillDigest
	rebindTypedPermit(t, &legacy)
	after, afterIdentity, err := Prompt(legacy.Assignment, legacy.Preparation, "/approved")
	if err != nil || afterIdentity == historicalDigest ||
		!strings.Contains(after, "Skills: material-readiness, log-triage, linux-bsp-debug") ||
		strings.Contains(after, "Selected Skill methods") {
		t.Fatal("existing typed-with-digest Task unexpectedly received new methods", err)
	}
	// Adding the old Skill-contract digest changes frozen Task/Context digests,
	// so the literal bundle SHA in the prompt legitimately changes. What must
	// remain unchanged is the old name-only guidance and identity JSON schema.
	identity, _, err := promptIdentity(legacy.Assignment, legacy.Preparation)
	raw, jsonErr := json.Marshal(identity)
	if err != nil || jsonErr != nil || strings.Contains(string(raw), "skill_guidance_version") ||
		strings.Contains(string(raw), "skill_contract_digest") {
		t.Fatal("historical typed-with-digest PromptIdentity was silently upgraded", err, jsonErr)
	}
}
