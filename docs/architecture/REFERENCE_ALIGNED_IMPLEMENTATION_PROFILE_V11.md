# Reference-Aligned Implementation Profile v11

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V10.md

Research basis:
- Open Source Reference Review Rounds 1–11
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v11 adds:
- derived engineering metrics;
- progressive promotion semantics;
- runtime configuration identity;
- curated AI/MCP tool profiles;
- runtime qualification.

None becomes new M1 infrastructure.

---

## 2. Engineering metrics are projections

Add versioned:

~~~text
MetricDefinition
~~~

Fields:
- metric ID/version;
- population/cohort;
- window;
- source objects/events;
- formula;
- exclusions;
- freshness;
- interpretation notes.

Measurements are derived from authoritative records.

They never alter Work/Run/Verification/Release state.

---

## 3. Platform metrics

Preferred metric families:

### Flow
- cycle time;
- queue/wait time;
- time to first Evidence;
- merge/release lead time.

### Quality
- first-pass Verification;
- rework;
- stale Evidence;
- defect escape;
- regression/incident rate.

### Agent/runtime
- resume success;
- Steering;
- Human Takeover;
- retry/failure;
- autonomous completion within policy.

### Resource/cost
- model/API usage;
- CI compute;
- device/HIL occupancy;
- resource wait.

No automatic individual performance score is derived from model/token/code-volume activity.

---

## 4. RuntimeConfigurationSnapshot

Behavior-affecting runtime configuration is explicitly identifiable.

~~~text
RuntimeConfigurationSnapshot
  provider/source
  environment
  configuration/flag identities
  resolved values or immutable snapshot
  content digest
  effective_at
~~~

For behavior-affecting configuration, deployment/test subject includes the configuration digest.

Artifact bytes alone do not fully identify running behavior.

---

## 5. PromotionPlan

Add immutable/versioned:

~~~text
PromotionPlan
~~~

Contains:
- Release Manifest digest;
- target environment/channel;
- staged cohorts;
- blast radius;
- stage entry rules;
- observation window;
- required operational Evidence/metrics;
- manual checkpoints;
- pause/abort/rollback rules;
- rollback Release/Bundle.

Release authorization and PromotionPlan are distinct but linked.

---

## 6. PromotionStage

Lifecycle:

~~~text
PENDING
 -> ADMITTED
 -> DEPLOYING
 -> OBSERVING
 -> PROMOTED
    | PAUSED
    | ROLLED_BACK
    | FAILED
    | UNKNOWN
~~~

Each stage binds:
- exact Release subject;
- RuntimeConfigurationSnapshot where applicable;
- cohort/target set;
- observation Evidence;
- Decisions;
- actual reconciled state.

---

## 7. Progressive-delivery metrics are operational Evidence

Canary/rollout health can use:
- service SLO/SLI;
- install success;
- crash/error rate;
- device telemetry;
- smoke/functional checks.

Missing/unknown measurement is not success.

Pre-authorized policy may automatically pause/rollback.

Automated rollout controller does not gain general Human Release Authority.

---

## 8. ToolProfile

Formal Runtime tool availability is frozen in:

~~~text
ToolProfile
~~~

Includes:
- approved tool/MCP server identities;
- exact package/image/version/digest where available;
- allowed tools;
- denied tools;
- tool schema/description digest;
- credential policy;
- network/data policy;
- trust/data classification;
- profile digest/version.

Run Input Manifest references exact ToolProfile.

Dynamic discovery cannot expand authority beyond the profile.

---

## 9. MCP/tool registry semantics

A registry/catalog is discovery only.

~~~text
listed/discovered
!=
approved
~~~

Approval to enter ToolProfile may require:
- provenance/source review;
- version pin;
- capability review;
- confidentiality/provider compatibility;
- adversarial/security qualification.

Tool descriptions and schemas are untrusted Runtime context.

---

## 10. RuntimeQualificationProfile

Reserve:

~~~text
RuntimeQualificationProfile
~~~

Includes:
- RuntimeProvider/model/profile;
- Runtime Adapter;
- ToolProfile;
- sandbox/action policy;
- Context Resolver version;
- task/eval corpus;
- adversarial corpus;
- required invariants;
- acceptance thresholds;
- confidentiality/assurance scope.

Output:

~~~text
RuntimeQualificationEvidence
~~~

A qualified runtime profile may be eligible for specified Formal Run classes.

---

## 11. Qualification triggers

Requalification is required by policy after material changes to:
- model/provider profile;
- Runtime Adapter;
- instructions;
- ToolProfile/server/schema;
- sandbox;
- Action Gateway;
- Context Resolver;
- provider data handling.

Qualification does not replace Run-level Verification.

---

## 12. MCP gateway adoption path

M1:
- fixed Codex tool/capability set;
- no generic MCP platform required.

M2/M3 when tool ecosystem grows:
- evaluate ToolHive/Docker MCP Gateway;
- centralized catalog;
- tool isolation;
- tool allowlist;
- auth/secrets/audit.

engineering-platform still owns ToolProfile/policy authority.

---

## 13. Provider/LLM gateway

Optional provider gateways can provide:
- routing;
- quota;
- retry/fallback;
- cost;
- provider connectivity.

Rules:
- provider route must satisfy confidentiality policy;
- only qualified Runtime profiles may serve Formal Runs;
- fallback that changes runtime/model identity creates/records the appropriate Run identity semantics.

---

## 14. Operational objectives

Platform runtime reliability can use vendor-neutral SLO concepts.

Potential OperationalObjectives:
- Session availability;
- Worker dispatch latency;
- Artifact finalize success;
- device queue target;
- Release confirmation time.

Operational reliability metrics never replace product Verification.

---

## 15. Milestone mapping

### M0
Reserve/freeze:
- MetricDefinition;
- ToolProfile;
- RuntimeQualificationProfile;
- RuntimeConfigurationSnapshot;
- PromotionPlan/Stage.

### M1
Implement:
- fixed ToolProfile for Codex;
- basic derived metrics from domain/OTel;
- no progressive production rollout required.

### M2/M3
Add when needed:
- curated MCP gateway/catalog;
- runtime qualification pipeline;
- runtime/feature configuration snapshots;
- staged promotion;
- operational SLO metrics.

---

## 16. Final rules added by v11

1. Engineering/productivity metrics are derived, versioned analytics.
2. AI activity is not an automatic individual performance score.
3. AI speed metrics are paired with quality/assurance metrics.
4. Runtime behavior-affecting configuration participates in deployed Subject identity.
5. Progressive deployment is governed by immutable PromotionPlan and stage state.
6. Tool/MCP discovery is not ToolProfile approval.
7. Formal Run tool authority is frozen and digest-bound.
8. Tool descriptions/schema changes may trigger Runtime requalification.
9. AI eval/security qualification governs Runtime eligibility, not engineering result correctness.
10. Provider gateways remain below confidentiality/assurance policy.

Architecture v1.2 remains canonical.
Profile v11 is the current implementation companion.
