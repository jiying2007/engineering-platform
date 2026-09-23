# Reference-Aligned Implementation Profile v21

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V20.md

Research basis:
- Open Source Reference Review Rounds 1–21
- Open Source Reference Synthesis Optimization
- Open Source Reference Synthesis Rounds 11–17
- Architecture v1.2

## 1. Goal

Profile v21 retains all v20 semantics and adds empirical AI-adoption governance:

- AIUsagePolicy;
- RuntimeInstructionProfile;
- MeasurementStudyDefinition;
- optional AIAssistedChangeProvenance;
- end-to-end paired AI outcome metrics.

No new mandatory M1 runtime service is introduced.

---

## 2. M1 stack remains unchanged

Required:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy for tests
- Git/CI
- Codex
- native Ubuntu Worker
- enterprise PKI or lightweight short-lived mTLS

No DevLake/DORA/METR/OpenSSF runtime dependency is introduced.

---

## 3. AIUsagePolicy

Add versioned:

~~~text
AIUsagePolicy
~~~

Defines:
- approved RuntimeProfiles;
- allowed task classes;
- confidentiality/data restrictions;
- ToolProfile constraints;
- required Reviewer/Verification level;
- prohibited autonomous actions;
- retention/telemetry policy;
- qualification requirements;
- escalation.

Policy can compile into OPA/Control Plane evaluation.

It is an organizational engineering policy, not a prompt.

---

## 4. RuntimeInstructionProfile

Add immutable:

~~~text
RuntimeInstructionProfile
~~~

Includes:
- coding/security instructions;
- domain constraints;
- prohibited patterns;
- dependency/supply-chain guidance;
- testing expectations;
- secure defaults;
- content digest/version.

Run Input Manifest / RuntimeQualificationProfile bind exact instruction profile.

Instruction changes can trigger requalification.

---

## 5. Instruction does not grant or restrict real authority by itself

Strict rule:

~~~text
RuntimeInstructionProfile
!=
Capability Enforcement
~~~

Real enforcement remains:
- OPA;
- ToolProfile;
- sandbox;
- Action Gateway;
- Credential Broker;
- network/filesystem/device policy.

Instruction text is defense-in-depth and behavior guidance.

---

## 6. AIAssistedChangeProvenance

Optional analytics/provenance record:

~~~text
AIAssistedChangeProvenance
  NONE
  AI_ASSISTED
  AI_PRIMARY
  UNKNOWN
~~~

May reference:
- Run/RuntimeProfile;
- human Steering;
- Human Takeover;
- confidence/provenance.

It is useful for:
- analytics;
- reviewer routing;
- reproducibility.

It is not a quality label.

---

## 7. MeasurementStudyDefinition

For formal productivity/quality experiments, add:

~~~text
MeasurementStudyDefinition
~~~

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
- model/runtime versions;
- protocol version.

This is separate from MetricDefinition.

---

## 8. AIAdoptionCohort

Analytics-only:

~~~text
AIAdoptionCohort
~~~

Potential dimensions:
- team/workstream;
- task class;
- repository/domain;
- RuntimeProfile/model;
- Assurance Profile;
- time window;
- usage level.

It cannot be used as a capability/authorization primitive.

---

## 9. Paired outcome metrics

AI acceleration must be interpreted with quality.

Examples:

~~~text
Run cycle time
+
first-pass Verification
+
rework
+
defect escape

coding time
+
review time
+
verification time

automation completion
+
Human Takeover
+
Incident/rollback
~~~

No standalone:
- token count;
- line count;
- prompt count;
- PR count

is treated as engineering productivity.

---

## 10. End-to-end cost

Total AI engineering cost may include:

~~~text
Model/API
+ generation effort
+ review effort
+ verification effort
+ rework
+ incident/escape
+ CI/HIL/resource cost
~~~

A locally cheaper model can be more expensive system-wide if it creates extra review/rework.

---

## 11. Small-batch and flow health

AI can increase code production faster than review/verification capacity.

Track:
- change size;
- candidate batch size;
- review queue;
- Verification queue;
- release batch size.

Assurance policy may limit change size/batch for higher-risk work.

---

## 12. Measurement causality discipline

Do not treat observational correlation as causal evidence.

Formal studies record:
- selection;
- non-participation;
- dropout;
- tool/version changes;
- task assignment;
- time effects.

AI adoption metrics may be descriptive without causal claim.

---

## 13. Time/version bounded AI conclusions

Any Runtime/reviewer productivity or safety conclusion binds:
- provider/model/profile;
- Runtime Adapter;
- ToolProfile;
- RuntimeInstructionProfile;
- context strategy;
- date range.

New model/profile versions require new qualification/measurement as policy determines.

---

## 14. AI governance feedback loop

~~~text
AIUsagePolicy
 -> Runtime/Reviewer Qualification
 -> Formal Runs
 -> Review / Verification / Metrics / Incident
 -> Measurement Study
 -> Policy change proposal
 -> Review/Approval
 -> new AIUsagePolicy version
~~~

Governance is evidence-driven and versioned.

---

## 15. Security guidance integration

Security-focused assistant guidance belongs in:
- RuntimeInstructionProfile;
- ReviewerQualification corpus;
- RuntimeQualification corpus.

Examples:
- secret handling;
- dependency/supply-chain checks;
- input validation;
- secure defaults;
- unsafe API patterns.

Security prompts are evaluated through qualification/failure replay.

---

## 16. Manufacturing/fleet semantics from v20 remain

Retain:
- ManufacturingExecutionRef;
- StationProfile;
- EquipmentCalibrationRecord;
- ProductionRecipeRevision;
- ManufacturingResultReceipt;
- CohortSnapshot;
- DevicePromotionAttempt;
- SystemUpdateManifest;
- UpdatePathEvidence.

No semantic changes.

---

## 17. Knowledge semantics from v20 remain

Retain:
- KnowledgeItem;
- KnowledgeFreshnessPolicy;
- KnowledgeValidationReceipt;
- STALE/INVALID/SUPERSEDED lifecycle;
- provenance/scope/trust/freshness-aware retrieval.

No semantic changes.

---

## 18. AI Review semantics from v20 remain

Retain:
- ReviewExecution;
- ReviewFinding;
- ReviewResolution;
- ReviewVerdict;
- ReviewContextProfile;
- ReviewerQualificationProfile/Evidence;
- independence levels R0–R4.

Reviewer evaluation participates in the broader AIUsagePolicy feedback loop.

---

## 19. M0/M1 mapping

### M0
Freeze/reserve:
- AIUsagePolicy;
- RuntimeInstructionProfile;
- MeasurementStudyDefinition;
- AIAdoptionCohort;
- optional AIAssistedChangeProvenance.

### M1
Implement:
- one Codex AIUsagePolicy;
- one exact RuntimeInstructionProfile;
- one fixed ToolProfile;
- local flow/quality metrics;
- no universal AI productivity claim.

---

## 20. Final rules added by v21

1. AI use has no assumed positive or negative productivity sign.
2. AI impact is measured across end-to-end delivery, not code generation alone.
3. AI usage metrics are paired with quality and assurance metrics.
4. AI provenance is analytics/routing input, not automatic quality judgment.
5. Measurement design/version/confounders are explicit for causal claims.
6. AIUsagePolicy is explicit, versioned and enforceable.
7. RuntimeInstructionProfile is immutable and qualification-bound.
8. Instructions never replace capability enforcement.
9. AI review/verification/rework is part of total AI cost.
10. AI capability conclusions are time/model/profile scoped.
11. Governance evolves through observed Evidence and reviewed policy changes.
12. Manufacturing, Knowledge and AI Review semantics from v20 remain unchanged.

Architecture v1.2 remains canonical.
Profile v21 is the current implementation companion.
