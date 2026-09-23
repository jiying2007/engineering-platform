# M0 Reference Adoption Plan v21

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V20.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V21.md

## 1. Principle

Round 21 adds AI-governance measurement/policy fixtures without adding new M1 services.

M0 proves:
- explicit AIUsagePolicy;
- immutable RuntimeInstructionProfile;
- paired flow/quality metrics;
- measurement-study schema;
- governance feedback loop.

---

## 2. AIUsagePolicy fixture

Create one M1 Codex policy.

Include:
- allowed task classes;
- allowed data/confidentiality class;
- fixed ToolProfile;
- required human review level;
- prohibited privileged actions;
- telemetry/retention;
- escalation.

Prove:
- policy is versioned/digest-bound;
- Run Input references the effective policy/profile;
- OPA/Control Plane can deny a disallowed task/action.

---

## 3. RuntimeInstructionProfile fixture

Create one immutable instruction profile with:
- coding expectations;
- secure coding instructions;
- dependency/supply-chain constraints;
- testing expectations;
- prohibited patterns.

Prove:
- instruction digest is bound to RuntimeQualificationProfile;
- instruction change causes a distinct qualification subject;
- instruction cannot grant a tool/capability absent from ToolProfile/OPA.

---

## 4. Paired metric fixture

Define and calculate:
- Run cycle time;
- first-pass Verification rate;
- Review/Verification time;
- rework count;
- Human Takeover rate.

Prove:
- AI usage is reported alongside quality/assurance;
- token/line/prompt count is not displayed as standalone productivity score.

---

## 5. MeasurementStudyDefinition fixture

Create one synthetic internal study:

~~~text
Hypothesis:
  qualified Codex assistance reduces end-to-end cycle time
  without increasing rework/defect escape
~~~

Include:
- population;
- cohort assignment;
- inclusion/exclusion;
- metrics;
- confounders;
- missing-data handling;
- Runtime/model versions;
- time window.

Prove:
- study result is distinct from MetricDefinition;
- observational dashboard cannot be mislabeled as causal study.

---

## 6. AIAssistedChangeProvenance fixture

Create examples:
- HUMAN_ONLY;
- AI_ASSISTED;
- AI_PRIMARY;
- UNKNOWN.

Bind:
- implementation Run;
- Steering/Takeover;
- RuntimeProfile.

Prove:
- provenance may influence review routing;
- provenance alone cannot pass/fail Integration or quality.

---

## 7. AI governance feedback fixture

Simulate:
- Runtime qualification result;
- several Runs;
- Reviewer findings;
- one Incident;
- metric projection;
- proposed AIUsagePolicy revision.

Prove:
- policy change requires explicit review/approval;
- runtime behavior does not mutate policy automatically.

---

## 8. Retain all v20 M0 work

Still required:
- manufacturing station/calibration/recipe/result fixtures;
- fleet cohort/per-device/multi-component fixtures;
- Knowledge freshness/revalidation;
- ReviewExecution/Finding/Resolution/Qualification;
- all v17 and earlier core domain/security/runtime/release work.

---

## 9. M0 exit additions

M0 additionally requires:
- AIUsagePolicy schema/fixture;
- RuntimeInstructionProfile schema/fixture;
- instruction != capability enforcement test;
- paired metric example;
- MeasurementStudyDefinition fixture;
- AI provenance cannot act as automatic quality gate;
- governance feedback loop requires reviewed policy change.

No DevLake/DORA/METR/OpenSSF service is required.

---

## 10. M1 target

M1 remains the same narrow formal engineering loop, now with an explicit Codex usage policy and instruction profile plus end-to-end flow/quality measurement.
