# Open Source Reference Review — Round 16: Interface Contracts and Compatibility Verification

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V15.md

## 1. Scope

Round 16 reviewed:
- Buf
- Protobuf / gRPC practices
- Protovalidate
- Pact
- OpenAPI compatibility tools such as oasdiff
- AsyncAPI
- Schemathesis

Focus:
- interface/schema revision identity;
- source/wire/runtime compatibility;
- consumer-provider contracts;
- breaking-change detection;
- generated/schema-based tests;
- embedded protocol/API evolution.

---

## 2. Interface Contract Revision is an immutable artifact/contract

The architecture already has Interface Contract / Interface Contract Revision.

Strengthen it with explicit machine-readable identity:

~~~text
InterfaceContractRevision
  interface_id
  revision
  kind
  schema/spec artifact digest
  producer/provider refs
  consumer refs
  compatibility policy
  content digest
~~~

Kinds may include:
- PROTOBUF;
- OPENAPI;
- ASYNCAPI;
- BINARY_PROTOCOL;
- IPC_RPC;
- OTA_METADATA;
- PERSISTED_SCHEMA;
- CONFIG_SCHEMA;
- CUSTOM.

---

## 3. Compatibility is multidimensional

Buf demonstrates that Protobuf compatibility is not one binary question.

Examples:
- source compatibility;
- generated-code/API compatibility;
- wire compatibility;
- JSON compatibility.

### Add:

~~~text
CompatibilityDimension
~~~

Possible values:
- SOURCE;
- WIRE;
- WIRE_JSON;
- API;
- BEHAVIORAL;
- PERSISTED_DATA;
- CONFIGURATION;
- SECURITY;
- TIMING_RESOURCE;
- CUSTOM.

A change may be compatible in one dimension and breaking in another.

---

## 4. Compatibility Policy

Add versioned:

~~~text
InterfaceCompatibilityPolicy
~~~

Defines:
- applicable interface kind;
- producer/consumer direction;
- required dimensions;
- backward/forward/full compatibility;
- supported version window;
- deprecation period;
- allowed exceptions;
- migration requirements.

Example:

~~~text
motor_protocol_v4
  provider: motor MCU
  consumer: main MCU
  required:
    WIRE backward compatible
    API compatibility not applicable
  supported consumers:
    v3..v4
~~~

---

## 5. Compatibility Evidence

Add:

~~~text
InterfaceCompatibilityEvidence
~~~

Binds:
- old contract digest;
- new contract digest;
- compatibility policy;
- checker/tool/version;
- dimension;
- result;
- detected breaking changes;
- raw report.

Results can include:
- COMPATIBLE;
- BREAKING;
- CONDITIONALLY_COMPATIBLE;
- INCONCLUSIVE;
- TOOL_ERROR.

A schema compile PASS is not compatibility Evidence.

---

## 6. Buf practice for Protobuf

Repository:
- https://github.com/bufbuild/buf

Buf provides:
- build/lint;
- module/dependency model;
- deterministic compilation;
- breaking-change checks;
- FILE/PACKAGE/WIRE_JSON/WIRE rule classes;
- versioned generation config.

### Decision

For any future Protobuf/gRPC platform protocol:
- prefer Buf-style lint/breaking checks;
- pin code-generation configuration/plugins;
- treat generated SDK/API changes as derived Artifacts;
- store breaking-check report as InterfaceCompatibilityEvidence.

M1 can adopt Buf if platform protocols use Protobuf; otherwise the schema remains tool-neutral.

---

## 7. Semantic validation — Protovalidate pattern

Schema shape alone is often insufficient.

Protovalidate demonstrates:
- field constraints;
- message constraints;
- cross-field CEL rules;
- consistent validation across languages.

### Absorb

Interface Contract Revision may include:

~~~text
validation_rules_ref
~~~

for semantic validation constraints.

Validation rule changes are part of the interface contract digest when behavior-facing.

This is useful for:
- Control API;
- Worker Protocol;
- Device Agent messages;
- OTA metadata;
- configuration objects.

---

## 8. Consumer-driven contracts — Pact

Repositories:
- https://github.com/pact-foundation/pact-specification
- https://github.com/pact-foundation/pact-go

Pact verifies interactions from the consumer's actual expectations against the provider.

### Optimization

Add optional:

~~~text
ConsumerContractArtifact
~~~

Binds:
- consumer identity/revision;
- provider/interface revision;
- expected interactions/messages;
- contract format/version;
- content digest.

Verification creates:

~~~text
ConsumerContractEvidence
~~~

This is especially useful where:
- schema compatibility is too broad;
- only a subset of fields/operations is actually consumed;
- multiple consumers upgrade at different speeds.

---

## 9. Consumer compatibility does not replace schema compatibility

Keep separate:

~~~text
Schema Compatibility
  "is the interface revision compatible under formal schema rules?"

Consumer Contract Verification
  "does this specific consumer still work against this provider?"
~~~

Both may be required by policy.

For embedded multi-component releases, this is valuable when:
- main MCU and motor MCU versions evolve independently;
- Linux daemon and MCU protocol evolve independently;
- app/cloud interfaces support multiple device generations.

---

## 10. Async/message contracts

AsyncAPI provides a structured contract for event/message-oriented systems.

Potential uses:
- platform integration events;
- device message buses;
- async IPC;
- future telemetry/control messaging.

Do not force AsyncAPI on compact binary MCU protocols.

Use it where message semantics naturally fit.

---

## 11. Schema-based fuzz/property testing — Schemathesis

Repository:
- https://github.com/schemathesis/schemathesis

Schemathesis generates:
- valid/invalid inputs;
- edge cases;
- stateful API workflows;
- schema conformance checks;
- crash reproductions/baselines.

### Optimization

For OpenAPI/GraphQL-style interfaces:
- use schema-driven negative/property testing as Verification Procedure;
- preserve failures as ReproductionCase/TestDefinition;
- record schema coverage.

This complements breaking-change checks.

A backward-compatible schema can still contain buggy implementation behavior.

---

## 12. Interface compatibility matrix

Generate projection:

~~~text
Interface Revision
  x
Producer Artifact/Version
  x
Consumer Artifact/Version
  x
Target Revision
  ->
Compatibility Evidence / Verdict
~~~

This is especially useful for a product with:
- Linux;
- main MCU;
- motor MCU;
- charger/dock MCU;
- app/cloud.

Do not store a manually maintained matrix as separate authority.

---

## 13. Breaking interface changes expand Impact Analysis

If InterfaceContractRevision is BREAKING under current policy:

Impact Analysis automatically includes candidate dependents from:
- TraceLinks;
- declared consumer list;
- Target/Release component graph;
- code/schema dependencies.

It then proposes:
- Task replanning;
- broader Verification;
- coordinated multi-component Integration Candidate;
- migration/rollback requirements.

Actual actions still require typed Decisions/policy.

---

## 14. Version negotiation

For runtime protocols that support multiple versions, Interface Contract should define:
- version field/negotiation mechanism;
- minimum/maximum supported versions;
- feature/capability negotiation;
- unknown-field behavior;
- fallback behavior;
- deprecation/removal date.

Runtime negotiation result may be captured as Evidence during integration/HIL tests.

---

## 15. Persistent data and OTA metadata

Compatibility is not only network/RPC.

Apply Interface Contract semantics to:
- persistent storage schema;
- config format;
- OTA manifest metadata;
- partition layout;
- bootloader handoff;
- calibration file format.

A breaking persisted-data change may require migration Evidence even if RPC schema is unchanged.

---

## 16. M0/M1 impact

### M0
Freeze:
- InterfaceCompatibilityPolicy;
- InterfaceCompatibilityEvidence;
- CompatibilityDimension vocabulary;
- optional ConsumerContractArtifact/Evidence.

### M1
Apply to platform's own API/protocol schemas where practical.

Example:
- Control API/OpenAPI diff;
- Worker Protocol Protobuf breaking check if Protobuf is chosen.

No Pact/Schemathesis service is required.

### M2
Apply to:
- Device Agent;
- MCU/Linux interface contracts;
- OTA/config/persisted schemas.

---

## 17. New invariants

1. **Interface compatibility is multidimensional, not a single boolean.**
2. **Schema compilation is not compatibility proof.**
3. **Compatibility Evidence binds exact old/new contract revisions and policy.**
4. **Consumer contract verification and schema compatibility are different checks.**
5. **Semantic validation rules participate in interface identity when behavior-facing.**
6. **Breaking changes expand impact scope explicitly.**
7. **Compatibility matrices are projections from immutable contracts/evidence.**
8. **Persistent/config/OTA schemas receive the same compatibility discipline as RPC APIs.**
9. **Schema-driven fuzzing complements but does not replace compatibility checks.**
10. **Runtime version negotiation outcomes are observable Evidence, not assumed compatibility.**

---

## 18. Conclusion

Round 16 turns Interface Contract from a document reference into a machine-verifiable engineering object.

For embedded multi-component systems the most important rule is:

> **A component version is releasable only when its exact interface revisions are compatible with the exact consumers in the target Release Bundle, or a coordinated migration is explicitly verified.**
