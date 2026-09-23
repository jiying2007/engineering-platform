# M0 Reference Adoption Plan v16

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V15.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V16.md

## 1. Principle

Round 16 adds interface-compatibility contracts, not new infrastructure services.

M0 proves compatibility semantics with local tools/fixtures.

---

## 2. Compatibility schema freeze

Freeze:
- CompatibilityDimension;
- InterfaceCompatibilityPolicy;
- InterfaceCompatibilityEvidence;
- ConsumerContractArtifact;
- ConsumerContractEvidence.

Extend InterfaceContractRevision with:
- kind;
- schema artifact digest;
- producer/consumer refs;
- policy ref.

---

## 3. Platform API compatibility fixture

Use one platform schema/API.

If Protobuf:
- run Buf breaking-style check.

If OpenAPI:
- run an OpenAPI-diff-compatible check.

Prove:
- source/schema compiles but deliberate breaking change is still detected;
- report binds exact old/new digests.

---

## 4. Consumer contract fixture

Create one synthetic consumer/provider interaction.

Prove:
- schema compatibility can pass while a consumer-specific expectation fails;
- ConsumerContractEvidence is distinct from InterfaceCompatibilityEvidence.

No Pact Broker/service is required.

---

## 5. Semantic validation fixture

Add one cross-field or conditional rule.

Prove:
- validation rule changes alter InterfaceContractRevision digest when behavior-facing;
- invalid runtime message is rejected consistently.

---

## 6. Schema property-test fixture

For an HTTP/OpenAPI fixture, optionally use Schemathesis-style generated cases.

Prove:
- schema-valid edge cases exercise implementation;
- failure becomes ReproductionCase/TestDefinition;
- tool output enters Evidence through normal trusted path.

---

## 7. Embedded compatibility reservation

Create one synthetic matrix:
- main MCU producer/consumer;
- motor MCU protocol revision;
- compatible and breaking versions.

Prove:
- breaking revision expands ImpactAnalysis candidate set;
- release bundle cannot claim compatibility from version strings alone.

---

## 8. Existing M0 path remains

All v15 requirements remain:
- formal model-check spike;
- device trust;
- ML lineage;
- incident/knowledge loop;
- ToolProfile/runtime qualification;
- core Run/Build/Integration/Evidence/Release chains;
- OTel/CDEvents;
- failure injection.

---

## 9. M0 exit additions

M0 additionally requires:
- compatibility dimensions/policy frozen;
- one machine-detected breaking change fixture;
- one consumer-contract mismatch fixture;
- persisted/config/OTA contract kinds representable;
- compatibility matrix derivable from immutable records.

No registry/broker service is required.

---

## 10. M1 target

M1 remains the same formal engineering loop, now with machine-verifiable schema/interface evolution for its own protocols.
