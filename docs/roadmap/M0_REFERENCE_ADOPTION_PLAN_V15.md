# M0 Reference Adoption Plan v15

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V14.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V15.md

## 1. Principle

Formal methods are a development/assurance aid, not an M1 runtime dependency.

M0 reserves formal artifacts and runs one targeted model-check spike against the hardest platform invariants.

---

## 2. Schema freeze

Add:
- FormalSpecificationArtifact;
- FormalVerificationEvidence;
- formal result vocabulary;
- assumptions/bounds fields;
- counterexample Artifact refs.

---

## 3. TLA+ spike A — Run ownership

Model:
- Attempt A epoch 1;
- disconnect;
- Attempt B epoch 2;
- A late callback;
- Pause;
- Takeover;
- Abort.

Prove/check:
- only current epoch mutates Run;
- takeover excludes Runtime writes;
- aborted Run cannot be resurrected;
- stale callback is harmless.

Convert discovered interleavings/counterexamples into property/failure tests.

---

## 4. TLA+ spike B — External side effects

Model:
- DB transaction/outbox;
- dispatch;
- lost response;
- UNKNOWN;
- reconciliation;
- safe retry;
- recovery epoch.

Prove/check:
- UNKNOWN is not blindly retried;
- confirmed irreversible action is not intentionally duplicated;
- recovery freezes irreversible work until reconciliation;
- completion requires confirmed/observed reality.

---

## 5. Formal Evidence fixture

Create one synthetic FormalVerificationEvidence with:
- exact model digest;
- tool/version;
- properties;
- bounded/unbounded semantics;
- result;
- log/counterexample ref.

Prove UI/API cannot flatten:
- NO_COUNTEREXAMPLE_WITHIN_BOUND
into
- PROVED_WITHIN_DECLARED_SEMANTICS.

---

## 6. Existing M0 path remains

Still required:
- all v14 domain/security/device fixtures;
- canonicalization;
- TraceLink;
- ML lineage;
- ToolProfile/runtime qualification;
- Incident/reproduction;
- Run/Build/Integration chains;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Verification;
- Release Admission;
- OTel/CDEvents;
- failure injection.

---

## 7. M0 exit additions

M0 additionally requires:
- formal schemas frozen;
- at least one platform-state TLA+ model executed or explicitly documented as skipped with rationale;
- model/check properties mapped to executable property/failure tests;
- bounded-result semantics preserved.

M0 does not require product-code CBMC/Kani/Frama-C/VeriFast.

---

## 8. M1 target

M1 remains operationally unchanged.

Formal modeling reduces design risk before implementation without increasing production topology.
