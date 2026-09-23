# M0 Reference Adoption Plan v6

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V5.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V6.md

## 1. Principle

M0 remains focused on the minimum trustworthy M1 loop.

Round 5 adds only:
- ExecutionSpec;
- Build Definition Manifest;
- Approval Carry-forward Decision;
- Worker compatibility checks.

FFDC and scaled Worker scheduling remain M2/scale work.

---

## 2. Phase 1 — Core schemas

Freeze:
1. Common schema envelope.
2. Canonical digest profile.
3. Requirement/Task/Run/Artifact/Evidence/Verification.
4. Run Input Manifest.
5. ExecutionSpec.
6. Run Receipt.
7. Build Definition Manifest.
8. Build Receipt.
9. Integration Subject / Eligibility.
10. Command/Event/Steering/Session Grant.
11. SchedulerAttribute provenance.
12. Resource allocation/health facets.
13. TestDefinition/TestVariant/TestAttempt/Verdict/Exoneration reservations.
14. Resolved Verification Execution Plan.
15. ApprovalCarryForwardDecision.
16. Release Admission / Promotion Reconciliation.
17. UNKNOWN external-operation semantics.

---

## 3. Phase 2 — Worker compatibility

One Ubuntu Worker must advertise:
- control protocol version;
- ExecutionSpec versions;
- Codex adapter version;
- sandbox capabilities;
- supported report/output contracts.

Tests:
- unsupported ExecutionSpec rejected before workspace mutation;
- stale execution_epoch rejected;
- malformed capability grant rejected;
- unknown output contract rejected.

---

## 4. Phase 3 — Exact execution path

Implement:

~~~text
Run Input Manifest
 -> ExecutionSpec
 -> Worker
 -> Attempt
 -> Run Receipt
~~~

Prove:
- ExecutionSpec references exact Run Input digest;
- Attempt is fenced;
- actual execution metadata reaches Run Receipt;
- resume creates a new Attempt/epoch but keeps explicit lineage.

---

## 5. Phase 4 — Exact build path

Implement:

~~~text
Build Definition Manifest
 -> Native Build
 -> Build Receipt
 -> Artifact
 -> Attestation
~~~

Prove:
- build command/config/toolchain inputs are frozen before execution;
- actual output digest is not accepted until Artifact finalization;
- Build Receipt records runtime-discovered deviations;
- provenance points to Definition + Receipt.

M1 accepts RECORDED_LEGACY where required.

---

## 6. Phase 5 — Integration / Verification

Continue v5 requirements:
- exact candidate subject;
- CI/test attempt model;
- Resolved Verification Execution Plan;
- Evidence/Verification;
- Integration Eligibility;
- merge reconciliation.

Add:
- approval carry-forward fixtures;
- default no-carry rule.

---

## 7. Phase 6 — Policy / workflow / artifact infrastructure

Required:
- PostgreSQL + outbox;
- Temporal;
- OPA;
- S3/MinIO-compatible ArtifactStore;
- Session Gateway;
- OpenTelemetry.

No change from v5.

---

## 8. Phase 7 — Failure injection

Use Toxiproxy/fakes to test:
- Worker disconnect during Attempt;
- result arrives after epoch replacement;
- build upload timeout;
- merge response loss;
- object-store timeout;
- Temporal connectivity loss.

Verify exact reconciliation behavior.

---

## 9. M2 — Failure diagnostics

During real HIL pilot implement:
- Procedure failure_capture;
- collectors;
- FailureDiagnosticBundle.

Acceptance:
- failure automatically preserves enough raw state for later diagnosis;
- diagnostics are tied to exact TestAttempt;
- collector failure does not rewrite test result;
- size/time limits are enforced.

---

## 10. Scale trigger — Worker queue

Do not implement in M1.

Trigger when direct assignment becomes limiting.

Then add:
- pending execution records;
- Worker pull/claim;
- claim lease/reclaim;
- atomic state + claimability;
- fleet manager separate from scheduler.

Taskcluster is the design reference.

---

## 11. M0 exit criteria

M0 exits when:
- ExecutionSpec is versioned/canonical;
- Worker compatibility negotiation works;
- exact Run Input -> ExecutionSpec -> Receipt chain works;
- Build Definition -> Build Receipt -> Artifact chain works;
- approval carry-forward defaults fail-closed;
- existing v5 schema/integration/reliability criteria still pass.

M0 does not require:
- distributed Worker queue;
- Worker autoscaling;
- Nix;
- OpenBMC test framework;
- Robot Framework;
- FFDC implementation.

---

## 12. M1 target

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

This is the smallest loop that captures both engineering intent and execution reality.
