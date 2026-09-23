# M0 Reference Adoption Plan

Date: 2026-09-23
Status: **Execution checklist**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V1.md

## Goal

Turn open-source references into bounded engineering spikes before M1 implementation.

A spike may result in:
- ADOPT
- INTEGRATE
- REFERENCE-ONLY
- REJECT

No dependency is adopted only because it is popular or feature-rich.

---

## Spike 1 — Temporal workflow conventions

Decision target: **ADOPT**

Validate:
- Go SDK;
- Signal/Update handling;
- Activity heartbeats;
- cancellation;
- long human waits;
- workflow versioning;
- Continue-As-New;
- PostgreSQL outbox integration.

Acceptance:
- simulated Control API/Worker restart does not lose formal workflow progress;
- PostgreSQL remains authoritative;
- duplicate signal/activity cannot duplicate business side effects.

Output:
- ADR PostgreSQL/Temporal boundary;
- workflow coding conventions;
- sample Work/Run workflow.

---

## Spike 2 — OpenHands SDK + SWE-ReX runtime/session comparison

Decision target: **REFERENCE / selective reuse**

Prototype:
- one local Codex-like interactive process;
- one remote execution backend;
- REST command path;
- WebSocket attach;
- multiple shell sessions;
- reconnect.

Compare:
- OpenHands Agent Server contracts;
- SWE-ReX execution/session interfaces;
- our required Session Supervisor semantics.

Must prove:
- formal Run/Attempt remains platform-owned;
- execution_epoch cannot be delegated to external session ID;
- structured Steering survives reconnect;
- Human Takeover is enforceable independently of provider session.

Output:
- RuntimeBackend interface;
- ExecutionBackend interface;
- Session Gateway protocol ADR.

---

## Spike 3 — in-toto/SLSA/Sigstore attestation compatibility

Decision target: **ALIGN + INTEGRATE**

Create:
- one build Artifact;
- one test Evidence;
- one Transform Receipt;
- one Release/subject signature.

Validate:
- exact digest binding;
- trusted issuer;
- offline verification;
- projection from platform-native record to standard attestation;
- no loss of Run/Target/Procedure references.

Must prove:
- in-toto/SLSA projection does not become mutable business authority;
- cosign/PKI implementation can be replaced behind AttestationBackend.

Output:
- AttestationBackend ADR;
- statement/predicate mapping;
- Trust Root / Issuer schema;
- signing PoC.

---

## Spike 4 — labgrid Device/HIL integration

Decision target: **INTEGRATE if fit**

Representative pilot:
- motor MCU/HIL lane.

Include:
- remote exporter/coordinator;
- serial;
- power/reset;
- flash command;
- one measurement/log source;
- pytest or equivalent procedure execution.

engineering-platform must still own:
- Device ID;
- Target Revision;
- lease/fencing token;
- Procedure Revision;
- Artifact digest;
- Evidence issuer/applicability.

Must prove:
- stale engineering lease cannot operate hardware even if labgrid resource exists;
- exact flashed Artifact can be tied to raw Evidence;
- resource loss produces quarantine/reconciliation semantics.

Output:
- DeviceLabBackend ADR;
- LabgridAdapter PoC;
- M2 integration decision.

---

## Spike 5 — Coder workspace/worker governance study

Decision target: **REFERENCE-ONLY for M1; future backend option**

Study/prototype only:
- worker identity;
- secure remote connectivity;
- workspace template/lifecycle;
- model credential isolation;
- resource inventory.

Must answer:
- which patterns improve Native Ubuntu Worker?
- can Coder become a future WorkspaceBackend without changing Run semantics?
- what is incompatible with attached embedded devices/local SDK assumptions?

Output:
- Worker/Workspace ADR;
- future CoderBackend compatibility notes.

---

## Spike 6 — Dagger controlled-build backend

Decision target: **OPTIONAL BACKEND**

Choose one container-friendly repository.

Validate:
- typed build inputs;
- content-addressed outputs;
- local/CI parity;
- OTel;
- exact Artifact digest/provenance.

Compare with NativeLegacyBuildBackend.

Output:
- BuildBackend interface;
- reproducibility-profile policy;
- go/no-go criteria for moving a project to Dagger.

---

## Spike 7 — RAUC embedded Linux OTA backend

Decision target: **OPTIONAL BACKEND**

Use only on a compatible Linux test target.

Validate:
- signed bundle;
- compatibility;
- A/B or recovery behavior where target supports it;
- UBI/NAND-relevant integration constraints;
- rollback/update interruption;
- HSM/PKCS#11 signing path if applicable.

engineering-platform remains authority for:
- Release Manifest;
- Target;
- Human Approval;
- final Verification;
- rollback Decision.

Output:
- ReleaseBackend interface;
- RAUC adapter feasibility;
- target/product fit matrix.

---

## Spike 8 — Backstage catalog/template concept extraction

Decision target: **REFERENCE-ONLY**

No Backstage deployment.

Extract only:
- entity metadata/relation patterns;
- owner/lifecycle labels;
- template parameter/step concepts.

Output:
- optional future Catalog projection schema;
- M4 Skill/Task template notes.

---

## Dependency admission gate

A candidate becomes a hard dependency only if all are true:

- active maintenance;
- acceptable license;
- acceptable security/update process;
- stable enough API/boundary;
- adapter replaceability;
- no hidden business authority;
- failure/degraded behavior defined;
- upgrade compatibility test exists;
- migration/exit path exists.

---

## M0 exit

M0 reference work is complete when:

- Temporal decision is implemented in ADR/conventions;
- RuntimeBackend/ExecutionBackend contracts are frozen;
- AttestationBackend and standard mapping are frozen;
- labgrid integration decision is made from a real HIL spike;
- Worker/Workspace boundary is frozen;
- BuildBackend and ReleaseBackend interfaces are frozen;
- external dependencies have explicit adapter and failure semantics;
- no selected dependency owns Requirement/Run/Verification/Release authority.

After that, proceed to M1 without further broad open-source surveying.
