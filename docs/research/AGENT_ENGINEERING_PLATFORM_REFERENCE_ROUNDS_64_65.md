# Agent Engineering Platform Reference — Rounds 64–65

Date: 2026-09-29  
Status: **Research / interoperability reference; not M1 Core implementation authority**

## 1. Round 64 — Portable Agent Skills packaging

### Agent Skills open standard

References:
- https://github.com/agentskills/agentskills
- https://agentskills.io/specification
- https://github.com/MicrosoftDocs/Agent-Skills

Agent Skills defines a lightweight, vendor-neutral filesystem package:

    skill-name/
      SKILL.md
      scripts/       (optional)
      references/    (optional)
      assets/        (optional)

SKILL.md carries YAML metadata such as name/description plus Markdown instructions. Clients use progressive disclosure: load lightweight metadata for discovery, load full instructions when the skill matches, then load supporting resources only when needed.

The format is now useful as an interoperability/distribution surface across multiple coding-agent products, including Codex- and Claude-compatible environments.

### Relationship to engineering-platform Skill

The two concepts must not be collapsed.

Agent Skills format primarily answers:
- how a runtime discovers a reusable instruction package;
- how the package is loaded without flooding context;
- how scripts/references/assets travel with instructions.

engineering-platform Skill must additionally answer:
- which Embedded Capability owns it;
- exact required inputs/material;
- typed/expected outputs;
- BLOCK and DEGRADED conditions;
- action ceiling and privileged-action requirements;
- Verification/Evidence contract;
- supported target/tool/environment envelope;
- maturity/qualification state;
- version/digest/provenance;
- owner/reviewer/change policy.

Recommended direction:

    canonical SkillContract
         |
         +-- runtime projection/export
                 |
                 +-- Agent Skills SKILL.md package
                 +-- provider-specific adapter if required

Do not make arbitrary SKILL.md frontmatter the source of engineering authority.

### Progressive disclosure is worth adopting

The loading model is directly useful for a large embedded skill catalog:

    Level 1: id/name/description/capability/trigger metadata
    Level 2: method/instructions/constraints
    Level 3: target-specific references/scripts/templates

This avoids putting all Linux/BSP/MCU/RTOS/debug/HIL knowledge into every Runtime context.

### Executable resources require separate trust

A skill may bundle scripts, but installation/discovery does not grant execution authority.

A Formal Run should bind:

    SkillContract digest
      + runtime projection digest
      + executable resource digests
      + ToolProfile
      + allowed action ceiling

Bundled scripts that perform Git/device/network/release mutation still pass through the normal privilege boundary.

## 2. Round 65 — Spec-driven and repo-local operating contracts

### GitHub Spec Kit

References:
- https://github.com/github/spec-kit
- https://github.github.io/spec-kit/

Spec Kit provides provider-neutral processes such as:

    Constitution
      -> Specify
      -> Plan
      -> Tasks
      -> Implement
      -> Converge

It also separates bug assessment/fix/test and evidence-backed idea assessment.

Useful engineering-platform lessons:
- intent should be refined before implementation;
- specification, technical plan and executable tasks are distinct artifacts;
- clarification and consistency checks are first-class;
- bug diagnosis should stay separate from repair and verification;
- provider-neutral process skills can sit above Codex/Claude.

### Authority boundary

Spec Kit artifacts are excellent Experience/authoring inputs, but a Markdown spec/task list is not automatically a Formal Task.

Recommended flow:

    WorkBuddy / Spec authoring / Spec Kit-like process
            -> source requirement/spec artifacts
            -> Material readiness
            -> validated TaskContract
            -> VerificationPlan
            -> Formal Run

The Core freezes the exact accepted revision/digest and normalizes required invariants.

### AgentSpec

Reference:
- https://github.com/yimwoo/agent-spec

AgentSpec is a useful comparison because it stores a persistent repo-local operating contract: accepted requirements, scoped tasks, allowed paths, iteration limits, verification commands, review evidence and handoff state.

This overlaps with engineering-platform concepts but has a different authority placement.

Useful ideas:
- durable handoff independent of chat history;
- allowed-path/scope projection;
- iteration budgets;
- explicit verification commands;
- tracked blocked/paused states;
- provider-native adapter kept thin.

Do not move central engineering authority into repo-local mutable files merely for portability.

Potential projection:

    Core TaskContract / VerificationPlan / Run state
        -> signed or digest-bound repo-local handoff pack
        -> Codex/Claude/native runtime

Repo-local state can support disconnected/offline developer workflows and human inspection, but reconciliation back into Core remains explicit.

## 3. Combined Skill and Task authoring model

Long-term separation:

    Capability
       owns
    SkillContract  <--- engineering method / authority constraints
       |
       +-- Agent Skills package  <--- portable runtime instructions/resources
       |
       +-- qualification profile
       |
       +-- evidence expectations

    Requirement / issue / WorkBuddy
       |
       +-- spec-driven authoring
       |
       v
    TaskContract  <--- immutable accepted execution contract
       |
       +-- repo-local/runtime projection
       v
    Run

This preserves interoperability without weakening the existing domain model.

## 4. Adoption recommendation

For M1:
- keep explicit Skill routing;
- keep current TaskContract/Core authority;
- do not introduce another spec database or workflow engine.

After retained pilots:
1. test exporting one existing embedded Skill as a standards-compatible SKILL.md package;
2. prove the same canonical SkillContract can drive Codex and a second Agent Skills client;
3. compare Spec Kit authoring artifacts against actual WorkBuddy requirements and TaskContract creation;
4. add a repo-local handoff projection only if disconnected/CLI usage demonstrates a real need.
