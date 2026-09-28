# Agent Engineering Platform Reference — Round 66 (Skill Qualification and Promotion)

Date: 2026-09-29  
Status: **Research / future Skill lifecycle reference; not M1 Core authority**

## 1. Why this matters

Portable Agent Skills solve packaging and discovery. They do not prove that a skill is useful, safe, correctly routed or mature.

Round 66 surveys current skill-evaluation practice and maps it onto engineering-platform's stronger Skill maturity model.

## 2. Real-harness skill evaluation

### agent-skill-eval

Reference:
- https://github.com/tardigrde/agent-skill-eval

Important patterns:
- run the real Claude Code, Codex and OpenCode CLIs rather than a raw model API;
- install the skill into a fresh workspace per case;
- paired with-skill vs without-skill baseline;
- repeated runs / pass@k because single stochastic runs are weak evidence;
- deterministic state-diff assertions plus optional model rubric;
- negative should-not-trigger controls;
- exact recorded harness/model/reasoning/version configuration;
- token/cost/duration budgets;
- re-grade retained outputs without rerunning the agent;
- scoped cleanup that does not delete unrelated branches/PRs.

These align strongly with engineering-platform RuntimeQualificationProfile and retained Evidence principles.

## 3. Structural, routing and behavioral tests

### AMD Skillscope

Reference:
- https://github.com/amd/skillscope

Skillscope separates:
- structural validation;
- routing/trigger validation, including interference among multiple installed skills;
- end-to-end behavioral validation.

This distinction should be kept. A structurally valid skill may trigger incorrectly; a correctly routed skill may still perform badly.

## 4. Safety, reliability and cost

### AWS sample skill evaluation framework

Reference:
- https://github.com/aws-samples/sample-agent-skill-eval

Useful dimensions:
- safety / unsafe installation and permissions;
- functional quality;
- trigger reliability;
- cost efficiency;
- version regression and lifecycle comparison.

Do not copy its weighted grade as an authority model. engineering-platform should retain the underlying dimensional facts and policy thresholds.

## 5. Signed and benchmarked skill distribution

### NVIDIA Agent Skills

Reference:
- https://github.com/NVIDIA/skills

NVIDIA's catalog demonstrates a stronger publication posture:
- portable Agent Skills packages;
- skill identity/governance card;
- detached signature artifact;
- evaluation dataset;
- generated benchmark report / uplift data;
- centralized catalog synchronization.

This suggests a future SkillRelease package:

    SkillContract digest
      + portable SKILL.md projection
      + executable/reference resource digests
      + qualification dataset identity
      + qualification report
      + compatible RuntimeProfile envelope
      + owner/reviewer identity
      + signature/attestation where required

## 6. engineering-platform Skill qualification model

Recommended stages:

    DRAFT
      -> STRUCTURALLY_VALID
      -> ROUTING_QUALIFIED
      -> BEHAVIOR_QUALIFIED
      -> PROJECT_REPLAY_QUALIFIED
      -> RETAINED_ENGINEERING_PROVEN

Meaning:

### STRUCTURALLY_VALID
- canonical SkillContract/schema valid;
- portable projection/spec valid if exported;
- references/resources resolve;
- executable resources are enumerated/digested.

### ROUTING_QUALIFIED
- positive trigger cases;
- negative near-miss cases;
- conflict/interference cases with neighboring skills.

### BEHAVIOR_QUALIFIED
- real agent harness, not only raw API;
- exact RuntimeProfile/model/tool versions;
- deterministic assertions where possible;
- repeated stochastic runs;
- with-skill vs baseline/ablation comparison;
- cost/time/action-side-effect budgets.

### PROJECT_REPLAY_QUALIFIED
- real historical failures/tasks from the embedded project;
- frozen source/material/acceptance identity;
- regression replay across skill revisions;
- target/tool/environment scope explicitly recorded.

### RETAINED_ENGINEERING_PROVEN
- used successfully in real Formal Runs;
- exact Evidence/Verification retained;
- no maturity inference from benchmark score alone;
- maturity is scope-bound, not universal.

## 7. Skill promotion is policy, not a score

A single weighted score must not decide production maturity.

Instead:

    QualificationFacts
       |
       +-- routing pass/fail
       +-- behavioral assertions
       +-- baseline uplift
       +-- cost/latency
       +-- safety findings
       +-- project replay
       +-- retained real engineering evidence
       |
    SkillPromotionPolicy
       -> allowed maturity state / supported envelope

This makes policy changes auditable without rewriting historical evaluation facts.

## 8. Important negative controls

Every serious Skill eval set should include:
- should-trigger;
- should-not-trigger;
- neighboring-skill ambiguity;
- missing-material/BLOCK case;
- action-above-ceiling case;
- intentionally wrong expected result;
- failure/noise/infrastructure distinction where relevant;
- regression case from a real past incident.

## 9. Adoption recommendation

After M1 retained pilots:
1. choose one existing embedded Skill such as log-triage or linux-bsp-debug;
2. create a small deterministic trigger + behavioral eval set;
3. run the same SkillContract projection through exact Codex and one second compatible runtime;
4. add at least one historical project failure replay;
5. retain qualification outputs as Evidence but keep maturity/promotion as explicit platform policy;
6. only then generalize a Skill qualification service/catalog.
