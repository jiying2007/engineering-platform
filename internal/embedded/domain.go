package embedded

type Capability struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SkillIDs    []string `json:"skill_ids"`
}

type Skill struct {
	ID              string   `json:"id"`
	OwnerCapability string   `json:"owner_capability"`
	Purpose         string   `json:"purpose"`
	BlockConditions []string `json:"block_conditions,omitempty"`
	AllowedActions  []string `json:"allowed_actions,omitempty"`
	Maturity        string   `json:"maturity"`
}

var skills = []Skill{
	{ID: "material-readiness", OwnerCapability: "embedded.verification", Purpose: "Validate that required engineering material exists before formal execution.", BlockConditions: []string{"missing exact source identity", "missing required acceptance criteria"}, Maturity: "DEFINED"},
	{ID: "architecture-impact-analysis", OwnerCapability: "embedded.architecture", Purpose: "Analyze subsystem, interface, resource and verification impact of a change.", Maturity: "DEFINED"},
	{ID: "interface-contract-review", OwnerCapability: "embedded.architecture", Purpose: "Review lifecycle, compatibility, error and version contracts.", Maturity: "DEFINED"},
	{ID: "linux-bsp-debug", OwnerCapability: "embedded.linux-bsp", Purpose: "Diagnose boot, kernel, storage, driver and system-layer failures using evidence.", Maturity: "DEFINED"},
	{ID: "mcu-rtos-debug", OwnerCapability: "embedded.mcu-rtos", Purpose: "Diagnose MCU startup, fault, memory, concurrency and timing failures.", Maturity: "DEFINED"},
	{ID: "driver-integration-review", OwnerCapability: "embedded.driver-component", Purpose: "Review component integration, lifecycle, recovery and compatibility.", Maturity: "DEFINED"},
	{ID: "log-triage", OwnerCapability: "embedded.debug-reliability", Purpose: "Build an observed timeline and evidence gaps from raw logs/traces.", Maturity: "DEFINED"},
	{ID: "verification-plan-builder", OwnerCapability: "embedded.verification", Purpose: "Map acceptance criteria to concrete evidence and verification procedures.", Maturity: "DEFINED"},
}

var capabilities = []Capability{
	{ID: "embedded.architecture", Name: "Embedded Architecture", Description: "System boundaries, interfaces, compatibility and engineering impact.", SkillIDs: []string{"architecture-impact-analysis", "interface-contract-review"}},
	{ID: "embedded.linux-bsp", Name: "Linux / BSP", Description: "Boot, kernel, BSP, storage, driver and Linux system integration.", SkillIDs: []string{"linux-bsp-debug"}},
	{ID: "embedded.mcu-rtos", Name: "MCU / RTOS", Description: "Startup, memory, concurrency, timing, control and MCU firmware.", SkillIDs: []string{"mcu-rtos-debug"}},
	{ID: "embedded.driver-component", Name: "Driver / Component", Description: "Peripheral and component integration, lifecycle and recovery.", SkillIDs: []string{"driver-integration-review"}},
	{ID: "embedded.debug-reliability", Name: "Debug / Reliability", Description: "Evidence-driven failure analysis and regression prevention.", SkillIDs: []string{"log-triage"}},
	{ID: "embedded.verification", Name: "Verification", Description: "Material readiness, acceptance-to-evidence planning and independent verification.", SkillIDs: []string{"material-readiness", "verification-plan-builder"}},
}

func Capabilities() []Capability {
	out := make([]Capability, len(capabilities))
	copy(out, capabilities)
	return out
}

func Skills() []Skill {
	out := make([]Skill, len(skills))
	copy(out, skills)
	return out
}
