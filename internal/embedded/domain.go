package embedded

type Capability struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SkillIDs    []string `json:"skill_ids"`
}

type Skill struct {
	ID                string   `json:"id"`
	Version           int      `json:"version"`
	OwnerCapability   string   `json:"owner_capability"`
	Purpose           string   `json:"purpose"`
	InputContract     []string `json:"input_contract"`
	RequiredMaterial  []string `json:"required_material"`
	Method            []string `json:"method"`
	OutputContract    []string `json:"output_contract"`
	BlockConditions   []string `json:"block_conditions"`
	AllowedActions    []string `json:"allowed_actions"`
	ProhibitedActions []string `json:"prohibited_actions"`
	EvaluationMethod  string   `json:"evaluation_method"`
	Maturity          string   `json:"maturity"`
}

var skills = []Skill{
	{
		ID:                "material-readiness",
		Version:           1,
		OwnerCapability:   "embedded.verification",
		Purpose:           "Validate required engineering material before formal execution.",
		InputContract:     []string{"WorkItem", "TaskContract", "MaterialManifest"},
		RequiredMaterial:  []string{"repository", "full source SHA", "acceptance criteria"},
		Method:            []string{"compare required material by task and device policy", "emit READY/BLOCKED/approved DEGRADED with missing facts"},
		OutputContract:    []string{"material readiness result", "missing-material reasons"},
		BlockConditions:   []string{"missing exact source", "missing acceptance criteria"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "curated ready, blocked and approved-degraded scenarios",
		Maturity:          "DEFINED",
	},
	{
		ID:                "architecture-impact-analysis",
		Version:           1,
		OwnerCapability:   "embedded.architecture",
		Purpose:           "Analyze subsystem, interface, resource and verification impact of a change.",
		InputContract:     []string{"requirement change", "target architecture"},
		RequiredMaterial:  []string{"source baseline", "target identity", "acceptance criteria"},
		Method:            []string{"trace affected components and interfaces", "separate observed architecture from inferred dependencies"},
		OutputContract:    []string{"impact report", "verification scope"},
		BlockConditions:   []string{"missing baseline", "unknown ownership of affected interface"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "review impact against independently checked interfaces and scope",
		Maturity:          "DEFINED",
	},
	{
		ID:                "interface-contract-review",
		Version:           1,
		OwnerCapability:   "embedded.architecture",
		Purpose:           "Review lifecycle, compatibility, error and version contracts.",
		InputContract:     []string{"old and new interface contract"},
		RequiredMaterial:  []string{"producer and consumer identities", "interface version"},
		Method:            []string{"compare signatures and wire lifecycle", "identify breaking transitions and rollback risks"},
		OutputContract:    []string{"compatibility findings", "migration requirements"},
		BlockConditions:   []string{"no producer or consumer identity"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "golden compatibility cases and negative lifecycle tests",
		Maturity:          "DEFINED",
	},
	{
		ID:                "linux-bsp-debug",
		Version:           1,
		OwnerCapability:   "embedded.linux-bsp",
		Purpose:           "Diagnose boot, kernel, storage, driver and system failures with evidence.",
		InputContract:     []string{"Linux symptoms", "logs and kernel context"},
		RequiredMaterial:  []string{"target BSP and kernel identity", "authoritative log or reproduction"},
		Method:            []string{"locate last observed state and subsystem layer", "generate discriminating experiment for hypotheses"},
		OutputContract:    []string{"layer findings", "hypothesis evidence"},
		BlockConditions:   []string{"no authoritative symptom material"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "replay against independent boot, UBI and kernel fixtures",
		Maturity:          "DEFINED",
	},
	{
		ID:                "linux-bsp-integration",
		Version:           1,
		OwnerCapability:   "embedded.linux-bsp",
		Purpose:           "Review Linux/BSP/storage/driver integration for changes and bring-up.",
		InputContract:     []string{"Linux feature or bring-up diff"},
		RequiredMaterial:  []string{"source base SHA", "BSP and target identity"},
		Method:            []string{"check boot and driver lifecycle", "analyze storage, resource and upgrade impact"},
		OutputContract:    []string{"integration impact", "build and verification plan"},
		BlockConditions:   []string{"missing source base"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "compare cross-target build and interface regression Evidence",
		Maturity:          "DEFINED",
	},
	{
		ID:                "mcu-rtos-debug",
		Version:           1,
		OwnerCapability:   "embedded.mcu-rtos",
		Purpose:           "Diagnose MCU startup, ISR, memory, timing and RTOS faults.",
		InputContract:     []string{"MCU symptom and code"},
		RequiredMaterial:  []string{"firmware and target identity", "map, register trace or reproduction"},
		Method:            []string{"separate ISR/task/timing observations from hypotheses", "design bounded discriminating test"},
		OutputContract:    []string{"fault hypothesis registry", "required measurements"},
		BlockConditions:   []string{"no authoritative MCU evidence"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "compare reproducible fault injection and restored register evidence",
		Maturity:          "DEFINED",
	},
	{
		ID:                "mcu-rtos-integration",
		Version:           1,
		OwnerCapability:   "embedded.mcu-rtos",
		Purpose:           "Review MCU/RTOS/control and firmware integration.",
		InputContract:     []string{"firmware change", "build target"},
		RequiredMaterial:  []string{"target MCU and toolchain identity", "source base SHA"},
		Method:            []string{"inspect startup/linker/ISR contracts", "assess control timing and memory budgets"},
		OutputContract:    []string{"integration impact", "firmware verification plan"},
		BlockConditions:   []string{"missing target or toolchain"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "independent ELF/map checks and target-specific build Evidence",
		Maturity:          "DEFINED",
	},
	{
		ID:                "driver-integration-review",
		Version:           1,
		OwnerCapability:   "embedded.driver-component",
		Purpose:           "Review component integration, lifecycle and error recovery.",
		InputContract:     []string{"driver diff", "interface requirements"},
		RequiredMaterial:  []string{"component and target identity", "source baseline"},
		Method:            []string{"compare init and shutdown sequences", "trace error paths and fallback policy"},
		OutputContract:    []string{"integration findings", "interface test scope"},
		BlockConditions:   []string{"no component interface contract"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "fault-path coverage and versioned interface fixtures",
		Maturity:          "DEFINED",
	},
	{
		ID:                "log-triage",
		Version:           1,
		OwnerCapability:   "embedded.debug-reliability",
		Purpose:           "Derive an observed timeline and evidence gaps from immutable raw logs.",
		InputContract:     []string{"raw log or trace"},
		RequiredMaterial:  []string{"log bytes", "timebase provenance"},
		Method:            []string{"normalize without rewriting raw data", "classify observed versus inferred events"},
		OutputContract:    []string{"timeline", "hypothesis candidates"},
		BlockConditions:   []string{"missing raw log"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "lineage-checked replay with ambiguous timestamp cases",
		Maturity:          "DEFINED",
	},
	{
		ID:                "verification-plan-builder",
		Version:           1,
		OwnerCapability:   "embedded.verification",
		Purpose:           "Map acceptance criteria to independent engineering Evidence.",
		InputContract:     []string{"TaskContract", "acceptance criteria"},
		RequiredMaterial:  []string{"frozen criteria", "target identity", "risk class"},
		Method:            []string{"map each criterion to test and issuer", "state negative and blocked outcomes explicitly"},
		OutputContract:    []string{"VerificationPlan", "independent Evidence requirements"},
		BlockConditions:   []string{"unverifiable acceptance criterion"},
		AllowedActions:    []string{},
		ProhibitedActions: []string{"invent-evidence", "privileged-action-without-Action-Gateway"},
		EvaluationMethod:  "plan coverage, mismatch rejection and independent verifier review",
		Maturity:          "DEFINED",
	},
}

var capabilities = []Capability{
	{ID: "embedded.architecture", Name: "Embedded Architecture", Description: "System boundaries, interfaces, compatibility and engineering impact.", SkillIDs: []string{"architecture-impact-analysis", "interface-contract-review"}},
	{ID: "embedded.linux-bsp", Name: "Linux / BSP", Description: "Boot, kernel, BSP, storage, driver and Linux system integration.", SkillIDs: []string{"linux-bsp-debug", "linux-bsp-integration"}},
	{ID: "embedded.mcu-rtos", Name: "MCU / RTOS", Description: "Startup, memory, concurrency, timing, control and MCU firmware.", SkillIDs: []string{"mcu-rtos-debug", "mcu-rtos-integration"}},
	{ID: "embedded.driver-component", Name: "Driver / Component", Description: "Peripheral and component integration, lifecycle and recovery.", SkillIDs: []string{"driver-integration-review"}},
	{ID: "embedded.debug-reliability", Name: "Debug / Reliability", Description: "Evidence-driven failure analysis and regression prevention.", SkillIDs: []string{"log-triage"}},
	{ID: "embedded.verification", Name: "Verification", Description: "Material readiness, acceptance-to-evidence planning and independent verification.", SkillIDs: []string{"material-readiness", "verification-plan-builder"}},
}

// Catalog callers must never be able to mutate global source-of-truth slices.
func cloneCatalogStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func Capabilities() []Capability {
	out := make([]Capability, len(capabilities))
	copy(out, capabilities)
	for i := range out {
		out[i].SkillIDs = cloneCatalogStrings(out[i].SkillIDs)
	}
	return out
}

func Skills() []Skill {
	out := make([]Skill, len(skills))
	copy(out, skills)
	for i := range out {
		out[i].InputContract = cloneCatalogStrings(out[i].InputContract)
		out[i].RequiredMaterial = cloneCatalogStrings(out[i].RequiredMaterial)
		out[i].Method = cloneCatalogStrings(out[i].Method)
		out[i].OutputContract = cloneCatalogStrings(out[i].OutputContract)
		out[i].BlockConditions = cloneCatalogStrings(out[i].BlockConditions)
		out[i].AllowedActions = cloneCatalogStrings(out[i].AllowedActions)
		out[i].ProhibitedActions = cloneCatalogStrings(out[i].ProhibitedActions)
	}
	return out
}
