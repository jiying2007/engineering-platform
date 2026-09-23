# M0 Reference Adoption Plan v7

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V6.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V7.md

## 1. Principle

M0 remains the smallest trustworthy contract/proof phase.

Round 6 adds:
- CodeIntelligenceArtifact schema;
- TelemetryProfile;
- OTel correlation attributes;
- CDEvents/CloudEvents projection mapping.

None adds a new mandatory service.

---

## 2. Phase 1 — Core schema freeze

Continue v6 and add:

1. CodeIntelligenceArtifact.
2. TelemetryProfile.
3. engineering.* OTel correlation attribute registry.
4. IntegrationEvent projection schema.
5. CDEvents mapping registry.

CodeIntelligenceArtifact must include:
- source tree digest;
- kind;
- generator/version;
- content digest;
- coverage metadata.

---

## 3. Phase 2 — Telemetry conventions spike

Instrument a single M1 Run path with OpenTelemetry:

~~~text
Control API
 -> Temporal
 -> Worker
 -> Session Supervisor
 -> Codex
 -> local command/tool
 -> Git/CI
 -> Artifact
 -> Verification
~~~

Prove:
- Run/Attempt/correlation navigation end-to-end;
- standard GenAI/tool fields used where supported;
- platform domain IDs use engineering.* namespace;
- full prompt/source content is absent under default profile;
- telemetry loss does not affect domain state/audit.

Deliver:
- Telemetry ADR;
- attribute registry;
- redaction/content-capture tests.

---

## 4. Phase 3 — Integration event projection spike

Implement a small projector:

~~~text
Domain Event
 -> CDEvents mapping where available
 -> CloudEvents envelope
 -> test consumer
~~~

Initial mappings:
- build started/finished;
- artifact packaged/published;
- test run finished/skipped;
- Verification completed as engineering-platform custom event when no standard mapping fits.

Prove:
- duplicate event delivery is harmless;
- event loss does not change domain truth;
- stable IDs/correlation survive projection;
- no sensitive authority payload is leaked.

---

## 5. Phase 4 — Code intelligence schema only

Do not block M1 on SCIP generation.

Freeze the CodeIntelligenceArtifact contract and create one lightweight RepositoryMap fixture bound to a Git tree digest.

Optional spike:
- generate a SCIP index for one Go/C/C++ repo if tooling is straightforward.

Prove:
- stale tree invalidates context artifact;
- Run Input Manifest may reference exact context digest;
- generated context cannot modify policy/authority.

---

## 6. Existing M0 critical path remains

Still required:
- canonical schema/digest fixtures;
- PostgreSQL/outbox;
- Temporal;
- OPA;
- ArtifactStore;
- Session Grant/Gateway;
- RunInput -> ExecutionSpec -> Attempt -> Receipt;
- BuildDefinition -> BuildReceipt -> Artifact;
- Integration Subject / Eligibility;
- Attestation Controller;
- Evidence/Verification;
- Release Admission model;
- Toxiproxy failure tests.

---

## 7. M0 exit additions

M0 now also requires:
- TelemetryProfile defaults are tested;
- OTel trace correlates one full Run without exposing full prompt/source by default;
- CDEvents/CloudEvents projection is demonstrated;
- CodeIntelligenceArtifact schema is tree-bound and content-addressed.

M0 does not require:
- SCIP service/index database;
- Kythe;
- OpenInference/OpenLLMetry backend;
- CDEvents broker;
- code graph database.

---

## 8. M1 target remains unchanged

~~~text
WorkBuddy
 -> Requirement READY
 -> Task Revision
 -> Run Input Manifest
 -> ExecutionSpec
 -> Codex Run
 -> Run Receipt
 -> Integration Subject
 -> Build Definition
 -> Build Receipt / Artifact
 -> CI/Test Evidence
 -> Verification
 -> Integration Eligibility
 -> Merge/Reconcile
 -> Closure Manifest
 -> WorkBuddy
~~~

Observed with OpenTelemetry and projected outward with standard event semantics, while authority remains internal.
