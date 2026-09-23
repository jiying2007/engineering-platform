# M0 Reference Adoption Plan v10

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V9.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V10.md

## 1. Principle

Round 9/10 findings do not expand the M0/M1 critical path.

M0 only reserves enough schema to avoid future breakage:
- model/assurance artifact kinds;
- verification environment/fidelity fields.

MBSE, assurance-case tooling and virtual-platform integrations are post-M1 work.

---

## 2. M0 schema reservations

Add to existing schemas:

### Artifact kinds
- MODEL
- ASSURANCE_CASE

### TestVariant / Evidence
- verification_environment_class
- verification_environment_definition_ref

### Optional future records
- EnvironmentEquivalenceDecision

No Model Repository or Assurance Case service is required.

---

## 3. Verification environment enum fixture

Freeze an initial enum/profile:

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

Test:
- schema compatibility/evolution;
- environment required for authoritative test Evidence where applicable;
- two different environment definitions produce different TestVariants.

---

## 4. Environment definition fixture

Create one immutable example:

~~~text
VerificationEnvironmentDefinition
  tool/version
  target mapping
  configuration
  seed/conditions
  known limitations
  content digest
~~~

M1 may use a simple CI/host definition.

No emulator is required.

---

## 5. M2 fidelity pilot

For the embedded pilot:
1. inventory existing tests;
2. classify environment/fidelity;
3. identify virtualizable tests;
4. try one relevant emulator/simulator where practical;
5. preserve HIL/physical release checks;
6. measure HIL-time reduction and correlation.

Candidates:
- Renode
- QEMU
- Gazebo/Webots
- project-specific SIL

Selection is target-specific.

---

## 6. Environment equivalence fixture

Before any cross-environment Evidence reuse is implemented, create a policy fixture proving:

~~~text
simulation PASS
!=
physical PASS
~~~

unless there is an explicit EnvironmentEquivalenceDecision.

No actual equivalence decision needs to exist in M1.

---

## 7. ModelArtifact reservation

Freeze a general Artifact metadata profile for models:
- model kind;
- version/spec;
- source tool;
- content digest;
- element namespace.

No SysML/Capella integration required.

---

## 8. Assurance Case reservation

Reserve a generic Artifact kind and references only.

M1 does not implement:
- claims graph;
- GSN;
- SACM.

When A3/A4 requires it, introduce the structured schema based on real assurance needs.

---

## 9. Existing M0 critical path remains

Still required:
- canonicalization;
- core domain schemas;
- TraceLink;
- Run Input / ExecutionSpec / Run Receipt;
- Build Definition / Build Receipt;
- Integration Subject / Eligibility;
- Session Grant/Gateway;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Attestation/Verification;
- TestReport/BOM/Finding;
- Release Admission/Reconciliation;
- OTel/CDEvents;
- failure injection.

---

## 10. M0 exit additions

M0 additionally verifies:
- environment class is represented in test/evidence fixtures;
- simulator/model locator cannot substitute for digest identity;
- model/assurance Artifact kinds can be represented without special infrastructure.

No new runtime service is required.

---

## 11. M1 target

M1 remains the same small formal engineering loop.

Round 9/10 merely ensure the schema can grow into MBSE/high-assurance and simulation/HIL workflows without redesign.
