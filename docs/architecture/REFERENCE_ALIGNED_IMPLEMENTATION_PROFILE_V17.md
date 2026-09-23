# Reference-Aligned Implementation Profile v17

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V16.md

Research basis:
- Open Source Reference Review Rounds 1–17
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v17 makes resolved configuration a first-class immutable subject input while preserving clear separation between:
- structure/schema;
- local semantic constraints;
- authorization/assurance policy.

No new mandatory M1 runtime service is introduced.

---

## 2. ConfigurationSchemaArtifact

Add immutable:

~~~text
ConfigurationSchemaArtifact
~~~

Metadata:
- schema kind/version;
- content digest;
- applicable object/domain;
- owner;
- compatibility policy;
- constraint refs;
- tooling/version.

Default generic representation:
- JSON Schema/native typed schema;
- Protobuf schema where protocol-native.

---

## 3. ConstraintSetArtifact

Add optional immutable:

~~~text
ConstraintSetArtifact
~~~

For bounded local semantic rules not cleanly represented by structural schema.

Possible evaluator:
- CEL/schema-native constraint mechanism.

Rules:
- deterministic;
- side-effect free;
- bounded;
- exact evaluator/version captured.

Do not use local constraints as authorization authority.

---

## 4. ResolvedConfigurationArtifact

Formal execution/release consumes:

~~~text
ResolvedConfigurationArtifact
~~~

Contains:
- source configuration refs;
- overlays/defaults/inputs;
- schema digest;
- constraint-set digest;
- canonical resolved values;
- resolver/version;
- target/environment scope;
- content digest.

Rule:

> source authoring representation is not execution identity; resolved canonical configuration is.

---

## 5. ConfigurationValidationEvidence

Add:

~~~text
ConfigurationValidationEvidence
~~~

Binds:
- ResolvedConfigurationArtifact;
- ConfigurationSchemaArtifact;
- ConstraintSetArtifact;
- evaluator/version;
- result;
- violations.

Results:
- VALID
- INVALID
- INCONCLUSIVE
- TOOL_ERROR

This proves declared configuration constraints only, not product functionality.

---

## 6. Validation layers

Keep separate:

### Structural/schema validation
- JSON Schema/native schema/Protobuf.

### Local object/config constraints
- CEL-style bounded expressions or schema-native rules.

### Authorization/Assurance policy
- OPA/Rego and platform domain policy.

### Domain state guards
- Control Plane transition logic/invariants.

No layer silently substitutes for another.

---

## 7. CUE/KCL strategy

CUE/KCL are optional authoring/resolution tools for complex configuration estates.

If used:

~~~text
authoring sources
 -> deterministic resolver
 -> ResolvedConfigurationArtifact
 -> canonical digest
~~~

Platform storage/authority remains resolved typed data, not one DSL's source syntax.

Do not support multiple DSLs without demonstrated need.

---

## 8. Configuration as Artifact transform

Configuration resolution creates a receipt:

~~~text
source config digests
 + overlays/inputs
 + resolver/version
 -> ResolvedConfigurationArtifact
~~~

Record:
- input digests;
- resolver/tool version;
- parameters;
- output digest.

This reuses Artifact Derivation semantics.

---

## 9. Target configuration

TargetRevision references exact resolved behavior-affecting configuration where appropriate:
- partition layout;
- boot config;
- component versions;
- protocol revisions;
- feature set;
- flash geometry;
- power/board profile.

Do not leave these only as uncontrolled repository YAML.

---

## 10. Build configuration

BuildDefinitionManifest references:
- exact ResolvedConfigurationArtifact;
- toolchain;
- dependency lock;
- options/features.

Build Receipt records actual applied configuration and deviations.

---

## 11. Runtime/Release configuration

RuntimeConfigurationSnapshot and Release Manifest reference exact resolved configuration when behavior depends on:
- feature flags;
- environment values;
- calibration;
- policy-selected options.

Same binaries with different behavior-affecting configuration are different deployed Subjects.

---

## 12. Calibration integration

CalibrationArtifact may itself contain or reference a ConfigurationSchemaArtifact.

Threshold/config changes:
- produce new Artifact digest;
- may require compatibility/migration;
- trigger Impact Analysis/Reverification.

This supports audio/KWS/FOC/device tuning cleanly.

---

## 13. Configuration compatibility

Use InterfaceCompatibilityPolicy/Evidence from v16 for:
- config schema evolution;
- persisted data;
- OTA metadata;
- rollback config compatibility.

A breaking config revision may require:
- migration;
- rollback strategy;
- compatibility Evidence.

---

## 14. M0/M1 strategy

### M0
Freeze:
- ConfigurationSchemaArtifact;
- ConstraintSetArtifact;
- ResolvedConfigurationArtifact;
- ConfigurationValidationEvidence.

### M1
Use simple:
- native typed structs / JSON Schema;
- OPA;
- explicit domain guards.

CEL/CUE are optional spikes only when a real contract benefits.

### M2+
Apply resolved configuration identity to:
- Target;
- Device/HIL;
- OTA;
- calibration;
- runtime feature configuration.

---

## 15. Final rules added by v17

1. Authoring config and resolved execution config are different objects.
2. Formal Subjects bind resolved canonical configuration digest.
3. Schema, local constraints, authorization policy and domain guards stay separate.
4. Local validation never grants capability/authority.
5. Config generation/resolution is a recorded deterministic transform.
6. Defaults/imports/overlays are part of resolved identity.
7. Validation Evidence does not imply functional correctness.
8. Config schema evolution follows compatibility/migration policy.
9. Untrusted config evaluation is sandboxed before formal use.
10. Configuration DSL proliferation is avoided.

Architecture v1.2 remains canonical.
Profile v17 is the current implementation companion.
