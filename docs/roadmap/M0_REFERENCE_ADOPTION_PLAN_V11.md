# M0 Reference Adoption Plan v11

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V10.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V11.md

## 1. Principle

Round 11 adds governance schemas, not new M1 services.

M0 freezes enough structure for:
- tool authority;
- runtime qualification;
- future progressive promotion;
- derived metrics.

---

## 2. ToolProfile fixture

Create one fixed M1 Codex ToolProfile.

Prove:
- exact tool set is content-addressed;
- tool description/schema changes change profile digest;
- dynamic tool discovery cannot exceed profile;
- tool credentials remain governed separately.

---

## 3. Runtime qualification fixture

Create a minimal RuntimeQualificationProfile for the M1 Codex profile.

Include:
- smoke task corpus;
- permission-bypass cases;
- prompt/tool-output injection cases;
- secret-exfiltration attempt;
- stale-tool-schema case.

M0 only proves the schema and test harness shape.

It does not require a production AI-eval platform.

---

## 4. MetricDefinition fixtures

Define a few derived platform metrics:
- Run cycle time;
- resume success rate;
- Verification first-pass rate;
- Human Takeover rate;
- HIL wait time.

Prove:
- source lineage is explicit;
- formula/version is frozen;
- recalculation does not mutate source records.

Do not implement individual productivity ranking.

---

## 5. Runtime configuration fixture

Freeze RuntimeConfigurationSnapshot schema.

Use a simple example:
- one behavior-affecting configuration/feature setting;
- exact environment;
- digest.

Prove deployment/test subject changes when behavior-affecting config changes.

No feature-flag platform required.

---

## 6. PromotionPlan reservation

Freeze:
- stage;
- cohort/blast radius;
- observation window;
- entry/promote/rollback rules;
- rollback target;
- stage result.

M1 may use only a single mock/non-production stage.

No Argo Rollouts/Flagger dependency.

---

## 7. Existing M0 critical path remains

Still required:
- canonical schemas/digests;
- TraceLink;
- Run Input / ExecutionSpec / Receipt;
- Build Definition / Receipt;
- Integration Subject / Eligibility;
- Session Grant;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Attestation/Verification;
- Release Admission/Reconciliation;
- OTel/CDEvents;
- TestReport/BOM/Finding;
- failure injection.

---

## 8. M0 exit additions

M0 additionally verifies:
- M1 Run Input binds one exact ToolProfile;
- ToolProfile changes cannot be silent;
- Runtime qualification schema/test fixtures exist;
- MetricDefinition recalculates from immutable source data;
- RuntimeConfigurationSnapshot is representable;
- PromotionPlan can represent staged rollback without new infrastructure.

M0 does not require:
- DevLake/GrimoireLab;
- OpenFeature/Unleash/Flipt;
- Argo Rollouts/Flagger;
- ToolHive/Docker MCP Gateway;
- Promptfoo;
- Langfuse.

---

## 9. M1 target

M1 remains the same formal engineering loop with one fixed qualified Codex tool profile and derived operational metrics.
