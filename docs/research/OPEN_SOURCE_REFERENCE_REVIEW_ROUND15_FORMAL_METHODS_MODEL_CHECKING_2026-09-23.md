# Open Source Reference Review — Round 15: Formal Methods and Model Checking

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V14.md

## 1. Scope

Round 15 reviewed:
- TLA+
- Alloy
- CBMC
- Kani
- Frama-C
- VeriFast
- related formal/static verification practices

Focus:
- platform state-machine correctness;
- concurrency/fencing/reconciliation invariants;
- critical embedded code verification;
- how formal proof/model-check evidence fits the existing assurance model.

---

## 2. Two formal-method use cases

### Platform protocol/state-machine verification

Strong candidates:
- Run / Attempt fencing;
- Session owner transitions;
- Worker lease/reclaim;
- outbox / External Operation Ledger;
- UNKNOWN / reconciliation;
- approval / admission ordering;
- Device lease / fencing;
- Promotion state machine.

TLA+/Alloy-style model checking is particularly suitable.

### Product code verification

Strong candidates:
- critical C/C++ memory and pointer safety;
- bootloader/update state code;
- arithmetic/range assertions;
- unsafe/concurrent Rust;
- protocol/state-machine implementations.

CBMC/Kani/Frama-C/VeriFast are candidate tools.

---

## 3. TLA+ for Run ownership

Recommended first platform model:

~~~text
Run ownership
  CREATED
  -> Attempt A owns epoch 1
  -> A disconnects
  -> Attempt B owns epoch 2
  -> A returns late
~~~

Invariant:

~~~text
At most one current execution epoch may mutate authoritative Run state.
~~~

Model actions:
- dispatch;
- heartbeat;
- disconnect;
- retry/resume;
- pause;
- Human Takeover;
- abort;
- stale callback;
- duplicate command.

Counterexamples should be converted into implementation/property/failure-injection tests.

---

## 4. TLA+ for external side effects and reconciliation

Second recommended model:

~~~text
DB state
 -> transactional outbox
 -> dispatch external action
 -> response lost
 -> external action may or may not have happened
 -> UNKNOWN
 -> reconcile
 -> CONFIRMED or SAFE_TO_RETRY
~~~

Important invariants:
- a confirmed irreversible side effect is never intentionally executed twice;
- UNKNOWN is never blindly retried;
- DB recovery epoch cannot resume irreversible actions before reconciliation;
- completion cannot be asserted before observed/confirmed external reality;
- stale commands cannot bypass recovery mode.

---

## 5. FormalSpecificationArtifact

Reserve immutable:

~~~text
FormalSpecificationArtifact
~~~

Metadata:
- language/tool;
- specification/model version;
- source/subject references;
- content digest;
- assumptions;
- boundedness/limitations;
- tool/generator version.

Examples:
- TLA+ model;
- Alloy model;
- ACSL contracts;
- Kani proof harnesses;
- CBMC property harnesses.

---

## 6. FormalVerificationEvidence

Add Evidence specialization:

~~~text
FormalVerificationEvidence
~~~

Binds:
- exact source/specification Artifact;
- tool/version;
- assumptions/config;
- bounds/unwind limits;
- property IDs;
- result;
- proof/model-check log;
- counterexample Artifact if any;
- issuer.

Recommended result vocabulary:

~~~text
PROVED_WITHIN_DECLARED_SEMANTICS
NO_COUNTEREXAMPLE_WITHIN_BOUND
COUNTEREXAMPLE
INCONCLUSIVE
TOOL_ERROR
~~~

Do not label bounded model-check success simply as unlimited PROVED.

---

## 7. CBMC

Repository:
- https://github.com/diffblue/cbmc

CBMC can check C/C++ for:
- array bounds;
- pointer safety;
- exceptions;
- assertions;
- bounded behavioral properties.

Good targeted embedded uses:
- packet/protocol parsers;
- boot/update state transitions;
- buffer handling;
- arithmetic range assumptions;
- small security-critical libraries.

Do not attempt to formalize an entire vendor BSP by default.

---

## 8. Kani

Repository:
- https://github.com/model-checking/kani

Kani is useful for Rust:
- undefined behavior;
- panics;
- arithmetic overflow;
- assertions/contracts;
- unsafe code.

Potential future use:
- Rust Device Agent;
- security-sensitive parsers;
- trusted low-level platform components.

No current requirement to introduce Rust into MCU projects.

---

## 9. Frama-C and VeriFast

References:
- Frama-C
- VeriFast

Frama-C provides:
- abstract interpretation;
- weakest-precondition proof;
- slicing;
- impact/dependency analysis;
- temporal/runtime verification.

VeriFast supports modular verification of C/Rust/Java including memory/thread safety and concurrency properties.

Use selectively for high-assurance modules.

Do not make either an ordinary build dependency.

---

## 10. Formal methods produce Evidence, not authority

Formal flow:

~~~text
Requirement / Property
 -> FormalSpecificationArtifact
 -> Formal Verification Procedure
 -> FormalVerificationEvidence
 -> Verification
 -> optional AssuranceCaseArtifact
 -> Review / Decision / Release
~~~

Formal proof/model checking does not by itself prove:
- compiler correctness;
- toolchain correctness;
- real hardware timing/electrical behavior;
- omitted environment assumptions;
- whole-product integration.

Assumptions and scope remain visible.

---

## 11. Counterexamples are first-class artifacts

When a tool finds a counterexample:
- preserve exact trace/input;
- store as immutable Artifact;
- link to Finding or Incident;
- convert to ReproductionCase/TestDefinition where useful.

Formal failures should become executable engineering feedback, not disposable CI text.

---

## 12. Traceability integration

Use TraceLink:

~~~text
Requirement / Safety Property
 -> FormalSpecificationArtifact
 -> FormalVerificationEvidence
 -> Verification
~~~

Formal coverage can therefore appear alongside test/HIL Evidence in the same traceability projections.

---

## 13. Assurance Profile integration

Possible policy:

~~~text
A0-A2
  formal methods optional

A3
  selected critical state/code properties may require formal verification

A4
  selected formal claims/evidence may be required in Assurance Case
~~~

Exact requirements remain product/project policy.

---

## 14. Recommended M0 development spike

Recommended, not runtime-critical:

~~~text
TLA+ model
  Run/Attempt execution_epoch
  Human Takeover
  stale callback
  External Operation UNKNOWN
  reconciliation
~~~

Goals:
- validate design invariants before implementation expands;
- derive edge-case tests;
- expose impossible/unsafe transition combinations.

No TLA+ service is required in production.

---

## 15. M0/M1 impact

### M0
Reserve:
- FormalSpecificationArtifact;
- FormalVerificationEvidence.

Recommended:
- one TLA+ model/check spike.

### M1
No formal-method runtime dependency.

### A3/A4 / selected critical code
Use CBMC/Kani/Frama-C/VeriFast according to language/property.

---

## 16. New invariants

1. Formal verification is always scoped to explicit properties and assumptions.
2. Bounded verification exposes its bounds and does not masquerade as unlimited proof.
3. Formal Evidence binds exact source/specification/tool/version.
4. Counterexamples are retained as engineering artifacts.
5. Formal Evidence complements rather than replaces integration/HIL/physical Evidence.
6. Platform concurrency/reconciliation invariants are preferred early model-check targets.
7. Formal Evidence never automatically becomes Release Authority.

---

## 17. Conclusion

Round 15 adds a selective assurance technique without making normal development heavyweight.

The strongest immediate use is validating engineering-platform's own distributed-state invariants before M1 implementation grows.
