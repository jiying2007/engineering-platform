# Open Source Reference Review — Round 17: Configuration, Constraints and Resolved Configuration

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V16.md

## 1. Scope

Round 17 reviewed:
- JSON Schema
- CUE
- KCL
- CEL
- HCL
- Jsonnet
- existing OPA/Rego policy decision

Focus:
- structured configuration contracts;
- cross-field constraints;
- authoring vs resolved configuration;
- avoiding YAML/config drift;
- deterministic configuration identity;
- separating data validation from authorization/assurance policy.

---

## 2. Configuration has three distinct concerns

Do not solve all configuration needs with one language.

### Structure

Questions:
- which fields exist?
- type?
- required/optional?
- enum/range/shape?

Preferred:
- JSON Schema or native typed schema;
- Protobuf schema where protocol-native.

### Local semantic constraints

Questions:
- if A then B must exist;
- value X must be lower than Y;
- component combination must satisfy a local invariant.

Preferred:
- schema-native validation where possible;
- CEL-style bounded, side-effect-free expressions for suitable cases.

### Authorization/assurance policy

Questions:
- may actor perform action?
- does release require R4 review?
- does production require signed boot?
- can this waiver apply?

Preferred:
- OPA/Rego under existing policy model.

Do not collapse these three layers.

---

## 3. ConfigurationSchemaArtifact

Add immutable:

~~~text
ConfigurationSchemaArtifact
~~~

Metadata:
- schema kind;
- schema/version;
- content digest;
- owner/domain;
- applicable object/profile;
- compatibility policy ref;
- constraint-set refs;
- tooling/version.

Possible kinds:
- JSON_SCHEMA;
- PROTOBUF;
- OPENAPI_SCHEMA;
- CUSTOM.

Schema is itself versioned and content-addressed.

---

## 4. ConstraintSetArtifact

For local semantic constraints not expressible cleanly in structural schema:

~~~text
ConstraintSetArtifact
~~~

Metadata:
- language/profile;
- expression set;
- applicable schema/object;
- content digest;
- evaluator/version;
- deterministic/safety constraints.

Preferred language when suitable:
- CEL or schema-native expression mechanisms.

Constraint evaluation must be:
- side-effect free;
- resource bounded;
- deterministic for same inputs/profile.

---

## 5. ResolvedConfigurationArtifact

The configuration that actually affects execution/release must be materialized:

~~~text
ResolvedConfigurationArtifact
~~~

Includes:
- source config refs;
- overlays/inputs;
- schema digest;
- constraint-set digest;
- resolved canonical values;
- content digest;
- resolver/tool/version;
- environment/target scope.

Rule:

> authoring source is not execution identity; resolved canonical configuration is.

This is especially important when configuration uses:
- defaults;
- inheritance;
- overlays;
- environment variables;
- generated values;
- feature flags.

---

## 6. CUE

Repository:
- https://github.com/cue-lang/cue

CUE combines:
- schemas;
- validation;
- constraints;
- data unification;
- generation from/to JSON/YAML/OpenAPI/Protobuf.

### Decision

Strong optional authoring/resolution candidate for:
- complex Target configuration;
- release/config composition;
- generated deployment/manifests;
- configuration validation.

Do not make CUE syntax the canonical platform storage format.

If used:

~~~text
CUE sources
 -> deterministic resolution
 -> ResolvedConfigurationArtifact
 -> canonical digest
~~~

---

## 7. KCL

Repository:
- https://github.com/kcl-lang/kcl

KCL provides:
- schema-centric configuration;
- type checking;
- constraints/rules;
- configuration composition;
- JSON/YAML generation;
- platform-engineering focus.

### Decision

Useful alternative/reference to CUE for large configuration estates.

Do not support both CUE and KCL as first-class M1 authoring languages.

Choose only if a real project/team need emerges.

---

## 8. CEL

CEL is:
- non-Turing complete;
- side-effect-free;
- designed for fast/safe expression evaluation;
- suitable for JSON/Protobuf values.

### Preferred use

Use CEL-style expressions for local schema/object validation where:
- rule must travel with schema;
- cross-field logic is needed;
- a small bounded expression is better than a full policy engine.

Examples:
- min version <= max version;
- if secure_boot=true then signing_key_ref must exist;
- mutually exclusive configuration fields;
- bounded Target compatibility assertions.

Do not use CEL to replace OPA policy packages.

---

## 9. JSON Schema

JSON Schema remains preferred for generic JSON object structural validation.

Use it for:
- API/domain payload schemas;
- manifests;
- import/export objects;
- plugin/config contracts.

Maintain:
- schema version;
- stable canonical URI/ID policy;
- conformance fixtures;
- compatibility/migration semantics.

JSON Schema validates shape; it does not replace domain state guards.

---

## 10. HCL / Jsonnet

HCL and Jsonnet demonstrate human-friendly/composable configuration.

### HCL
Good for application-specific config authoring.

### Jsonnet
Powerful generation/composition but can import files and has execution/security considerations.

### Decision

Reference only unless a concrete integration requires them.

Do not add extra authoring languages preemptively.

If arbitrary configuration programs are evaluated:
- treat them as untrusted code/data;
- sandbox imports/execution;
- resolve to immutable canonical output before Formal use.

---

## 11. Source configuration vs resolved subject

Example:

~~~text
base target config
 + SKU overlay
 + environment overlay
 + feature flags
 + calibration
 -> resolved configuration
~~~

Verification binds the resolved configuration digest.

A source file being unchanged does not prove behavior unchanged if:
- default changed;
- imported module changed;
- external flag changed;
- environment input changed.

---

## 12. Configuration validation evidence

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

Result:
- VALID;
- INVALID;
- INCONCLUSIVE;
- TOOL_ERROR.

This proves a configuration satisfies declared constraints.

It does not prove functional correctness.

---

## 13. Configuration compatibility

Use InterfaceCompatibilityPolicy for schema/config evolution.

Examples:
- new firmware reads old config;
- rollback firmware reads current persisted settings;
- OTA migration converts config schema;
- CalibrationArtifact schema changes.

Breaking config schema change may require:
- migration procedure;
- compatibility evidence;
- rollback data strategy.

---

## 14. Target configuration

TargetRevision should reference exact resolved configuration artifacts for behavior-affecting platform/product configuration where needed.

Examples:
- partition layout;
- boot config;
- component versions;
- protocol versions;
- feature set;
- power profile;
- flash geometry.

Do not hide these only inside free-form YAML in repo.

---

## 15. Build configuration

BuildDefinitionManifest references exact:
- resolved build configuration;
- toolchain profile;
- feature flags/options;
- dependency lock.

Build Receipt records actual applied values and deviations.

This reduces:
> same commit, different hidden build options.

---

## 16. Release/runtime configuration

Release Manifest/RuntimeConfigurationSnapshot references exact ResolvedConfigurationArtifact where behavior depends on it.

Feature-flag systems may be external, but the verified/promoted subject records the resolved configuration snapshot/digest.

---

## 17. Configuration generation is a transform

When configuration tooling generates a final artifact:

~~~text
source config
 -> resolver/generator
 -> ResolvedConfigurationArtifact
~~~

record a Transform/Resolution Receipt:
- input digests;
- resolver/version;
- parameters;
- output digest.

This matches existing Artifact Derivation semantics.

---

## 18. M0/M1 impact

### M0
Freeze:
- ConfigurationSchemaArtifact;
- ConstraintSetArtifact;
- ResolvedConfigurationArtifact;
- ConfigurationValidationEvidence.

### M1
Use:
- JSON Schema/native Go types for structure;
- OPA for authorization/assurance;
- minimal local domain guards.

CEL/CUE are optional spikes only if they reduce complexity in a real schema.

### M2+
Use resolved config for:
- Target;
- Device;
- Release;
- calibration;
- OTA;
- runtime flags.

---

## 19. New invariants

1. **Authoring configuration and resolved execution configuration are distinct.**
2. **Formal execution/verification binds the resolved canonical configuration digest.**
3. **Structural validation, local semantic constraints and authorization policy remain separate layers.**
4. **OPA remains policy authority; local CEL/schema validation does not grant authorization.**
5. **Configuration generators are deterministic transforms with receipts.**
6. **Defaults/imports/overlays participate in resolved configuration identity.**
7. **Configuration validation proves declared constraints, not functional correctness.**
8. **Configuration schema evolution follows explicit compatibility/migration rules.**
9. **Untrusted configuration programs/imports are sandboxed before resolution.**
10. **Do not add multiple configuration DSLs without a demonstrated project need.**

---

## 20. Conclusion

Round 17 closes a common source of hidden engineering drift:

> **The thing that is tested and released is the fully resolved configuration, not the human-authored YAML/template that happened to generate it.**

The recommended default remains deliberately simple:
- typed/JSON Schema for structure;
- CEL-style expressions only for bounded local constraints when needed;
- OPA for authorization/assurance;
- optional CUE/KCL authoring only when complexity justifies them.
