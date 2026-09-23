# M0 Reference Adoption Plan v17

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V16.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md

## 1. Principle

Round 17 adds resolved-configuration semantics without adding configuration infrastructure to M1.

---

## 2. Configuration schema fixture

Create one ConfigurationSchemaArtifact for a representative platform object.

Prove:
- schema is versioned/content-addressed;
- invalid structure is rejected;
- schema revision changes are explicit.

---

## 3. Constraint fixture

Create one bounded cross-field rule.

Example:
- secure_boot=true requires signing_key_ref;
or
- protocol_min <= protocol_max.

Prove:
- same resolved input + same rule version is deterministic;
- rule failure is ConfigurationValidationEvidence;
- rule does not grant authorization.

---

## 4. Resolved configuration fixture

Create:
- base config;
- one overlay/default;
- one final ResolvedConfigurationArtifact.

Prove:
- changing overlay/default changes resolved digest;
- source file identity alone is insufficient;
- Run/Build subject can bind exact resolved digest.

---

## 5. Configuration validation fixture

Generate:
- VALID case;
- INVALID case.

Bind:
- schema;
- constraint set;
- resolver/evaluator version;
- resolved config.

Prove validation result is distinct from functional Verification.

---

## 6. Compatibility fixture

Create old/new config schema revisions.

Use v16 InterfaceCompatibility semantics to demonstrate:
- compatible change;
- breaking change requiring migration.

No CUE/KCL/CEL service is required.

---

## 7. Existing M0 path remains

All v16 work remains:
- formal-method spike;
- interface compatibility;
- device trust;
- ML lineage;
- incident/knowledge loop;
- ToolProfile/runtime qualification;
- Run/Build/Integration/Evidence/Release chains;
- OTel/CDEvents;
- failure injection.

---

## 8. M0 exit additions

M0 additionally requires:
- resolved config schema/digest semantics frozen;
- defaults/overlays participate in identity;
- validation layers are separated;
- one compatible/breaking config evolution fixture passes.

M0 does not require CUE, KCL or CEL deployment.

---

## 9. M1 target

M1 remains the same small engineering loop.

Round 17 prevents hidden configuration drift without increasing production topology.
