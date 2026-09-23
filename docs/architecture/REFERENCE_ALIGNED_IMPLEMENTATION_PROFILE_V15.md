# Reference-Aligned Implementation Profile v15

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V14.md

Research basis:
- Open Source Reference Review Rounds 1–15
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v15 adds selective formal-method support for:
- engineering-platform distributed-state invariants;
- selected A3/A4 product code properties.

Normal M1 execution remains unchanged.

---

## 2. FormalSpecificationArtifact

Reserve immutable:

~~~text
FormalSpecificationArtifact
~~~

Metadata:
- language/tool;
- specification version;
- subject/source refs;
- digest;
- assumptions;
- bounds/limitations;
- generator/tool version.

Examples:
- TLA+;
- Alloy;
- ACSL;
- CBMC harness;
- Kani proof harness.

---

## 3. FormalVerificationEvidence

Evidence specialization:

~~~text
FormalVerificationEvidence
~~~

Includes:
- exact FormalSpecificationArtifact/source;
- property IDs;
- tool/version;
- configuration;
- bounds/unwind limits;
- assumptions;
- result;
- proof/check logs;
- counterexample refs;
- trusted issuer.

Result vocabulary:
- PROVED_WITHIN_DECLARED_SEMANTICS;
- NO_COUNTEREXAMPLE_WITHIN_BOUND;
- COUNTEREXAMPLE;
- INCONCLUSIVE;
- TOOL_ERROR.

---

## 4. Platform invariant modeling

Preferred first model:
- Run Attempt epoch ownership;
- stale Worker callback;
- Pause/Resume;
- Human Takeover;
- Abort.

Primary invariant:

~~~text
Only the current execution epoch may mutate authoritative Run state.
~~~

Second model:
- DB/outbox;
- external side effect;
- lost response;
- UNKNOWN;
- reconciliation;
- recovery epoch.

Primary invariants:
- no blind retry of UNKNOWN;
- no duplicate confirmed irreversible side effect;
- recovery does not resume irreversible action before reconciliation.

---

## 5. Counterexample feedback

Counterexample is an immutable Artifact.

It may become:
- Finding;
- ReproductionCase;
- TestDefinition;
- property/invariant test;
- Incident input.

This connects formal modeling to the existing feedback loop.

---

## 6. Product-code formal verification

Selected candidates:
- CBMC for targeted C/C++;
- Kani for Rust;
- Frama-C for critical C analysis/proof;
- VeriFast for modular memory/concurrency properties.

Use only when Assurance Profile/property justifies it.

No blanket formalization of BSP/application code.

---

## 7. Formal Evidence semantics

Formal Evidence participates normally:

~~~text
FormalSpecification
 -> FormalVerificationEvidence
 -> Verification
 -> Review
 -> optional Assurance Case
 -> Decision / Release
~~~

It never replaces:
- HIL;
- physical verification;
- functional integration;
- production authority.

---

## 8. Traceability

TraceLink can bind:

~~~text
Requirement / Safety Property
 -> FormalSpecificationArtifact
 -> FormalVerificationEvidence
 -> Verification
~~~

Formal-coverage gaps can appear in assurance projections.

---

## 9. Assurance Profile

Policy may define:

~~~text
A0-A2
  optional formal analysis

A3
  selected formal property checks

A4
  formal claims/evidence as part of structured Assurance Case where needed
~~~

No hard-coded regulatory claim is implied.

---

## 10. M0/M1 strategy

### M0
- freeze schemas;
- run one small TLA+ development spike for execution ownership/reconciliation;
- derive property tests from it.

### M1
- no TLA+/CBMC/Kani/Frama-C/VeriFast runtime dependency.

### Later
- add product-code formal Evidence selectively.

---

## 11. Final rules added by v15

1. Formal results expose exact assumptions and bounds.
2. Bounded check success is not mislabeled unlimited proof.
3. Formal Evidence binds exact source/spec/tool.
4. Counterexamples are retained and feed ordinary engineering workflows.
5. Platform distributed-state models are high-value early targets.
6. Formal Evidence complements physical/integration Evidence.
7. Formal methods never grant Release Authority directly.

Architecture v1.2 remains canonical.
Profile v15 is the current implementation companion.
