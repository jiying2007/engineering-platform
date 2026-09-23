# Reference-Aligned Implementation Profile v10

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V9.md

Research basis:
- Open Source Reference Review Rounds 1–10
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v10 adds two optional/high-value extension paths without changing M1 dependency breadth:

1. MBSE/Assurance Case integration for high-risk or model-heavy programs.
2. Explicit verification-environment/fidelity semantics for simulation, emulation, HIL and physical testing.

Architecture v1.2 remains unchanged.

---

## 2. M1 stack remains intentionally small

Required:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy
- Git/CI
- Codex
- native Ubuntu execution

Not required:
- SysML/Capella
- GSN/SACM tools
- Renode/QEMU/Gazebo/Webots
- assurance-case engine

---

## 3. ModelArtifact

Reserve an immutable Artifact specialization:

~~~text
ModelArtifact
~~~

Metadata:
- model kind/language;
- specification/version;
- source tool/system;
- snapshot/content digest;
- model element namespace;
- export/generator version;
- optional validation/execution receipt.

Kinds may include:
- SYSML_V2
- CAPELLA
- UML_SYSML_V1
- AADL
- SIMULATION_MODEL
- CUSTOM

---

## 4. ModelElementRef

TraceLink may reference exact model elements through:

~~~text
ModelElementRef
  model_artifact_digest
  element_id/path
  element_type
~~~

Rules:
- authority-bearing model trace binds immutable model snapshot;
- mutable model URLs are only locators;
- model elements remain external engineering artifacts, not a second domain authority.

---

## 5. Model analysis as Evidence

Model execution/analysis may produce:
- constraint result;
- simulation result;
- calculation;
- architecture consistency check;
- requirement satisfaction claim.

It becomes trusted Evidence only when:
- exact ModelArtifact is bound;
- exact tool/procedure/version is bound;
- issuer is trusted for that Evidence class;
- inputs/environment are part of the Subject.

---

## 6. Optional AssuranceCaseArtifact

For A0-A2:
- Verification Plan;
- Evidence;
- Review;
- Risk Decisions

remain sufficient unless policy says otherwise.

For A3/A4 or regulated/high-assurance programs, optionally require:

~~~text
AssuranceCaseArtifact
~~~

Minimal concepts:
- subject_digest;
- claims;
- subclaims;
- rationale/argument;
- assumptions/context;
- Evidence citations;
- Verification/Review/Decision citations;
- challenges/open issues.

The case references existing immutable facts.

It never manufactures Evidence.

---

## 7. Assurance Case semantics

Verification asks:
> Did the required checks pass for the exact Subject?

Assurance Case asks:
> Why do the evidence, analyses, assumptions and independent reviews collectively justify the high-level claim?

Human/independent review remains required by Assurance Profile.

Possible future interchange:
- GSN visualization;
- SACM export/import.

No M1 implementation.

---

## 8. VerificationEnvironmentClass

Every authoritative TestAttempt/Evidence records an environment class.

Initial vocabulary:

~~~text
STATIC
HOST_SIMULATED
EMULATED
VIRTUAL_PLATFORM
SIL
HIL
PHYSICAL_DUT
FIELD
~~~

Names may be refined, but the semantic distinction is mandatory.

---

## 9. VerificationEnvironmentDefinition

Immutable/content-addressed definition may include:
- simulator/emulator;
- tool/version;
- board/machine model;
- peripheral model;
- robot/world model;
- physics engine/settings;
- sensor/noise model;
- network topology;
- stimulus;
- seed;
- limitations;
- configuration digest.

TestVariant references the exact environment definition when relevant.

---

## 10. Evidence environment binding

TestVariant/Evidence includes:
- VerificationEnvironmentClass;
- environment definition digest;
- Target mapping;
- Procedure;
- environment-specific limitations.

Simulation and HIL attempts of the same TestDefinition are different variants.

Their results are not merged implicitly.

---

## 11. EnvironmentEquivalenceDecision

Default:
> no evidence equivalence across different verification environments.

Optional policy-backed:

~~~text
EnvironmentEquivalenceDecision
~~~

Fields:
- source environment;
- target environment/Target Revision;
- Evidence/test class;
- exact equivalence scope;
- validation Evidence;
- limitations;
- policy/authority;
- validity period/version.

This allows bounded reuse without allowing "simulation passed" to silently mean "hardware passed".

---

## 12. Fidelity ladder

Resolved Verification Execution Plan may stage:

~~~text
STATIC
 -> HOST_SIMULATED
 -> EMULATED / VIRTUAL_PLATFORM
 -> SIL
 -> HIL
 -> PHYSICAL_DUT
~~~

Use lower-cost environments to:
- fail fast;
- expand matrix coverage;
- reduce device queue;
- reproduce failures deterministically.

Use required higher-fidelity environments for properties that depend on:
- electrical behavior;
- timing fidelity;
- thermal/current;
- real sensors;
- mechanical behavior;
- final product integration.

---

## 13. Assurance Profile controls minimum fidelity

Examples:

~~~text
logic/unit property
 -> HOST_SIMULATED acceptable

Linux userspace/rootfs integration
 -> EMULATED may satisfy some gates

MCU protocol state machine
 -> VIRTUAL_PLATFORM may satisfy development gate
 -> HIL/PHYSICAL required by release policy

motor torque/current/thermal
 -> HIL/PHYSICAL_DUT

robot navigation regression
 -> SIL broad coverage
 -> physical/HIL acceptance for release criteria
~~~

These are policy examples, not hard-coded rules.

---

## 14. Virtual-platform candidates

Potential M2+ execution mechanisms:
- Renode for supported embedded SoC/MCU virtual platforms;
- QEMU for Linux/CPU/machine emulation;
- Gazebo/Webots for robotics simulation;
- project-specific SIL.

These are Procedure execution mechanisms behind existing semantics.

No new authority.

---

## 15. Continuous virtual qualification

Future Continuous Qualification can use cheap virtual environments for:
- nightly matrix;
- fuzzing;
- randomized scenarios;
- protocol stress;
- long regression suites.

Only selected regressions/acceptance checks consume scarce HIL/physical resources.

---

## 16. M2 optimization goal

During real embedded pilot:

1. classify existing tests by VerificationEnvironmentClass;
2. identify hardware-independent checks;
3. move broad cheap checks earlier where feasible;
4. keep final hardware-sensitive checks explicit;
5. measure:
   - HIL utilization;
   - queue time;
   - early failure catch rate;
   - simulation-to-HIL correlation.

The platform optimizes cost/time only after preserving fidelity semantics.

---

## 17. Model/Assurance milestone mapping

### M0/M1
Only reserve Artifact kinds and environment fields.

### M2
Implement verification-environment classification.
Optionally integrate virtual platforms where useful.

### M3+
In model-heavy/system programs:
- ModelArtifact;
- ModelElementRef traces;
- model analysis Evidence.

### A3/A4 / regulated
Optional AssuranceCaseArtifact.

---

## 18. Final rules added by v10

1. **Models are immutable engineering artifacts, not alternate Control Plane authority.**
2. **Model analyses become Evidence only through exact trusted Procedure/Issuer binding.**
3. **Assurance Case is optional and driven by risk/assurance profile.**
4. **Assurance arguments cite Evidence; they never replace Evidence.**
5. **Every authoritative test identifies verification environment/fidelity.**
6. **Simulation/emulation never implicitly equals real hardware.**
7. **Cross-environment reuse requires an explicit equivalence Decision.**
8. **Verification plans may ladder from cheap virtual environments to scarce physical ones.**
9. **Hardware-sensitive release properties remain bound to appropriate physical/HIL proof.**

Architecture v1.2 remains canonical.
Profile v10 is the current implementation companion.
