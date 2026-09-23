# Open Source / Public Practice Review — Round 21: Empirical AI Coding Governance

Date: 2026-09-24
Status: **Archived research / implementation input**

Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V20.md

## 1. Scope

Round 21 reviewed current public research/practice:
- DORA State of AI-assisted Software Development 2025
- DORA AI Capabilities Model
- DORA 2026 follow-up insights
- METR 2025 randomized controlled productivity study
- METR 2026 experiment-design update
- OpenSSF Security-Focused Guide for AI Code Assistant Instructions
- OpenSSF guidance on securing open source in the age of AI

Focus:
- empirical productivity measurement;
- organizational/system effects;
- governance stance;
- AI instruction/security policy;
- measurement bias;
- avoiding simplistic AI ROI metrics.

---

## 2. AI impact is contextual, not a universal constant

Public research currently shows materially different outcomes depending on:
- task;
- developer familiarity;
- tool/model generation;
- repository/context;
- organization;
- review/verification burden;
- platform maturity.

### Decision

engineering-platform must not encode assumptions such as:

~~~text
AI-assisted = faster
AI-generated = lower quality
more AI usage = better productivity
~~~

All such claims are measurements/hypotheses to be evaluated on local cohorts.

---

## 3. AI acts at multiple levels

Distinguish outcome layers:

~~~text
Individual task
Team flow
Delivery system
Product quality
Operational stability
Business outcome
~~~

A local task may get faster while:
- review queue increases;
- batch size grows;
- rework rises;
- incidents increase.

Therefore AI effectiveness must be evaluated end-to-end.

---

## 4. AI adoption cohort

Add analytics-only:

~~~text
AIAdoptionCohort
~~~

Potential dimensions:
- team/workstream;
- RuntimeProfile;
- model/provider;
- task type;
- repository/domain;
- assurance class;
- time window;
- AI usage level.

Used only for analytics/experiments.

It does not grant capabilities or label developers.

---

## 5. AIAssistedChangeProvenance

Optionally record for a change/Run:

~~~text
AIAssistedChangeProvenance
  NONE
  AI_ASSISTED
  AI_PRIMARY
  UNKNOWN
~~~

plus:
- Runtime/Run refs;
- human takeover/steering;
- provenance confidence.

Purpose:
- analyze workflow outcomes;
- review routing where policy requires;
- reproduce automation behavior.

Do not treat provenance as an automatic quality score.

---

## 6. Paired outcome measurement

AI usage metrics are meaningful only when paired.

Examples:

~~~text
cycle time
WITH
first-pass verification / rework / defect escape

coding time
WITH
review time / validation time

automation rate
WITH
Human Takeover / rollback / Incident

model cost
WITH
end-to-end delivered outcome
~~~

The platform should expose paired dashboards, not one-dimensional AI adoption leaderboards.

---

## 7. Small-batch behavior matters

DORA research reinforces that system-level results depend on delivery fundamentals.

### Platform implication

Track:
- change size;
- candidate batch size;
- review wait;
- verification queue;
- release batch size.

AI-generated code must not silently encourage larger unreviewable Integration Subjects.

Policy may warn/block oversized changes for high-risk work.

---

## 8. Measurement design is versioned

Add:

~~~text
MeasurementStudyDefinition
~~~

for formal internal productivity/quality studies.

Fields:
- hypothesis;
- population;
- cohort assignment;
- inclusion/exclusion;
- time window;
- metrics;
- confounders;
- missing-data handling;
- statistical method;
- tool/model versions;
- protocol version.

This is separate from normal MetricDefinition.

---

## 9. Selection bias / adoption bias

The METR follow-up illustrates that participants may self-select based on willingness to work with/without AI.

### Decision

Internal studies should record:
- enrollment;
- dropouts;
- non-participation;
- policy/tool changes;
- task selection.

Do not present observational AI usage correlation as causal proof.

---

## 10. AI stance should be explicit

DORA emphasizes a clear, communicated AI stance.

Add policy/document artifact:

~~~text
AIUsagePolicy
~~~

Defines:
- allowed task classes;
- approved RuntimeProfiles;
- data/confidentiality restrictions;
- ToolProfile rules;
- human review requirements;
- prohibited autonomous actions;
- retention/telemetry;
- qualification requirements;
- escalation.

It compiles into OPA/Control Plane decisions where applicable.

---

## 11. RuntimeInstructionProfile

OpenSSF guidance reinforces that security expectations should be explicit in coding-assistant instructions.

Add immutable:

~~~text
RuntimeInstructionProfile
~~~

Includes:
- coding/security instructions;
- project/domain constraints;
- prohibited patterns;
- dependency/supply-chain guidance;
- testing expectations;
- secure defaults.

Run Input / RuntimeQualification binds exact instruction profile digest.

Instruction change can trigger requalification.

---

## 12. Instruction is not enforcement

Important boundary:

~~~text
InstructionProfile
!=
Capability enforcement
~~~

Security prompts can reduce mistakes but cannot replace:
- sandbox;
- ToolProfile;
- OPA;
- Action Gateway;
- secret isolation;
- Verification.

"Tell the agent not to do X" is never the sole control for privileged behavior.

---

## 13. Verification work is part of AI cost

AI may shift effort from generation to:
- audit;
- review;
- test;
- debugging;
- cleanup.

Therefore cost metrics should include:

~~~text
Generation effort
+ Review effort
+ Verification effort
+ Rework effort
+ Incident/escape cost
~~~

Model/token cost alone is not total cost.

---

## 14. Capability changes over time

AI models/tools improve rapidly.

Any productivity/security conclusion is time-bounded.

Metric/Study records bind:
- model/provider/version/profile;
- Runtime Adapter;
- ToolProfile;
- InstructionProfile;
- date range.

Old studies do not automatically authorize new Runtime versions.

---

## 15. Governance feedback loop

Use:

~~~text
AIUsagePolicy
 -> RuntimeQualification
 -> Formal Runs
 -> Metrics / Review / Verification / Incident
 -> MeasurementStudy / analysis
 -> policy change proposal
 -> review/approval
 -> new policy version
~~~

This avoids governance based purely on intuition or vendor claims.

---

## 16. Security guidance

OpenSSF practice suggests explicitly instructing coding assistants about:
- secure coding;
- supply-chain safety;
- dependency choice;
- secret handling;
- validation/error handling;
- language/platform security.

These become part of RuntimeInstructionProfile and reviewer qualification corpus.

Security instructions are tested, not only documented.

---

## 17. M0/M1 impact

### M0
Freeze/reserve:
- AIUsagePolicy;
- RuntimeInstructionProfile;
- MeasurementStudyDefinition;
- optional AIAssistedChangeProvenance.

### M1
- one explicit AIUsagePolicy for Codex;
- one instruction profile;
- local flow/quality metrics;
- no ROI claim from usage alone.

No DORA/METR service dependency.

---

## 18. M2/M3 impact

As usage grows:
- compare workstream/cohort outcomes;
- study review/verification displacement;
- calibrate Runtime/Reviewer profiles;
- revise AI policy from observed data.

---

## 19. New invariants

1. AI assistance has no hard-coded productivity or quality sign.
2. AI outcomes are measured across the whole delivery system.
3. AI usage metrics are paired with quality/assurance metrics.
4. AI provenance supports analysis/routing, not automatic judgment.
5. Measurement studies expose cohort/selection/confounding assumptions.
6. Observational correlation is not labeled causal without study design.
7. AIUsagePolicy is explicit/versioned.
8. Runtime instructions are immutable qualification inputs.
9. Instruction text never replaces capability enforcement.
10. AI verification/review/rework cost is part of total engineering cost.
11. AI capability conclusions are version/time/profile specific.
12. Governance evolves through measured evidence and explicit policy revisions.

---

## 20. Conclusion

Round 21 changes the AI-governance question from:

> "How much AI are engineers using?"

to:

> **"For this exact workflow, Runtime profile and risk class, does AI improve end-to-end flow while maintaining or improving engineering quality and safety?"**
