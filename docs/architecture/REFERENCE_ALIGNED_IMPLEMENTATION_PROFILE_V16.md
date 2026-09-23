# Reference-Aligned Implementation Profile v16

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V15.md

Research basis:
- Open Source Reference Review Rounds 1–16
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v16 makes interface compatibility a first-class, machine-verifiable engineering concern.

This applies to:
- Control/Worker APIs;
- Linux/MCU protocols;
- Device Agent;
- OTA metadata;
- persistent schemas;
- configuration formats;
- asynchronous messages.

M1 dependency breadth remains unchanged.

---

## 2. InterfaceContractRevision

Strengthen the existing immutable revision with:

~~~text
interface_id
revision
kind
schema_artifact_digest
producer_refs
consumer_refs
compatibility_policy_ref
content_digest
~~~

Kinds:
- PROTOBUF
- OPENAPI
- ASYNCAPI
- BINARY_PROTOCOL
- IPC_RPC
- OTA_METADATA
- PERSISTED_SCHEMA
- CONFIG_SCHEMA
- CUSTOM

---

## 3. Compatibility dimensions

Compatibility is not one boolean.

Initial dimensions:

~~~text
SOURCE
WIRE
WIRE_JSON
API
BEHAVIORAL
PERSISTED_DATA
CONFIGURATION
SECURITY
TIMING_RESOURCE
CUSTOM
~~~

A revision may be compatible in one dimension and breaking in another.

---

## 4. InterfaceCompatibilityPolicy

Versioned policy defines:
- interface kind;
- producer/consumer direction;
- required dimensions;
- backward/forward/full compatibility;
- supported version window;
- deprecation/removal policy;
- exception/migration rules.

Target/Assurance Profile selects required policy.

---

## 5. InterfaceCompatibilityEvidence

Evidence binds:
- old contract digest;
- new contract digest;
- compatibility policy;
- dimension;
- checker/tool/version;
- result;
- detected breaking changes;
- raw report.

Result vocabulary:
- COMPATIBLE
- BREAKING
- CONDITIONALLY_COMPATIBLE
- INCONCLUSIVE
- TOOL_ERROR

Compilation success alone is not Compatibility Evidence.

---

## 6. Protobuf profile

Where Protobuf/gRPC is used, prefer Buf-style:
- lint;
- deterministic/module-aware schema build;
- pinned generation config;
- breaking checks;
- explicit wire/source/JSON compatibility.

Generated code is a derived Artifact.

Buf itself is optional and protocol-specific.

---

## 7. Semantic validation

Interface contract may reference versioned semantic validation rules.

Examples:
- cross-field constraints;
- allowed ranges;
- conditional required fields;
- state-dependent validation.

Protovalidate/CEL-style approaches are strong candidates for Protobuf-based contracts.

Behavior-facing validation rule changes participate in interface revision identity.

---

## 8. ConsumerContractArtifact

Optional immutable:

~~~text
ConsumerContractArtifact
~~~

Contains:
- consumer identity/revision;
- provider/interface revision;
- expected requests/messages;
- expected response/message shapes;
- matching rules;
- format/version;
- digest.

Verification produces:

~~~text
ConsumerContractEvidence
~~~

Pact is a reference implementation pattern.

---

## 9. Schema and consumer checks remain separate

~~~text
Schema compatibility
!=
Consumer contract compatibility
!=
Implementation functional correctness
~~~

Policy can require multiple layers.

Example:
- Protobuf wire check passes;
- specific old main-MCU consumer still fails due behavioral assumption;
- HIL/consumer contract catches it.

---

## 10. Schema-driven negative/property testing

For OpenAPI/GraphQL-style interfaces, Schemathesis-style testing can:
- generate valid/invalid edge cases;
- check implementation/schema conformance;
- exercise stateful sequences;
- preserve failure reproductions.

It produces Verification Evidence through normal Procedure/Issuer semantics.

It does not replace compatibility checks.

---

## 11. Compatibility matrix is a projection

Generate:

~~~text
Interface Revision
 x Producer Artifact/Version
 x Consumer Artifact/Version
 x Target Revision
 -> Compatibility Evidence / Verdict
~~~

Do not maintain a separate mutable matrix as authority.

---

## 12. Embedded multi-component release

For PCR-like products:

~~~text
Linux
main MCU
motor MCU
dock MCU
bootloader
persistent/config formats
~~~

each Release Bundle records exact InterfaceContractRevision references.

Release Verification ensures:
- required compatibility dimensions pass;
- coordinated breaking changes include matching consumers;
- migration/rollback paths are verified when required.

---

## 13. Breaking-change impact

A BREAKING result expands candidate impact using:
- declared consumer refs;
- TraceLinks;
- Target component graph;
- Release Bundle relations;
- source/schema dependency graph.

Impact Analysis then proposes:
- coordinated Task/Integration changes;
- broader Verification;
- migration;
- Release Bundle coordination.

No automatic release-state mutation occurs without typed Decision/policy.

---

## 14. Version negotiation

Runtime interfaces that negotiate versions should define:
- supported min/max;
- protocol/capability handshake;
- unknown-field behavior;
- fallback;
- deprecation/removal date.

Negotiated runtime compatibility can be captured as Evidence during CI/HIL/device tests.

---

## 15. Persisted/config/update contracts

Interface discipline also applies to:
- persistent storage schema;
- OTA metadata;
- partition layout;
- bootloader handoff;
- calibration/config file schema.

A persisted-data breaking change may require migration Evidence even when RPC contracts are unchanged.

---

## 16. M0/M1 mapping

### M0
Freeze:
- CompatibilityDimension;
- InterfaceCompatibilityPolicy;
- InterfaceCompatibilityEvidence;
- optional ConsumerContractArtifact/Evidence.

### M1
Apply where platform schemas exist:
- Control API;
- Worker/Session protocol;
- integration event schemas.

### M2
Apply to actual Linux/MCU/Device/OTA contracts.

No Pact/Schemathesis server is required.

---

## 17. Final rules added by v16

1. Interface compatibility is explicitly multidimensional.
2. Compilation is not compatibility proof.
3. Compatibility Evidence binds exact old/new revisions and policy.
4. Schema compatibility and consumer contract verification are separate.
5. Semantic validation participates in contract identity.
6. Breaking interfaces expand impact scope.
7. Compatibility matrices are derived projections.
8. Persisted/config/OTA schemas receive the same compatibility governance as RPC.
9. Schema-driven property testing complements compatibility checks.
10. Exact Release Bundle consumers must be compatible or jointly migrated/verified.

Architecture v1.2 remains canonical.
Profile v16 is the current implementation companion.
