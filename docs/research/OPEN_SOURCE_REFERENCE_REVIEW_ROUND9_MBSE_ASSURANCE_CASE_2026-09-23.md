# Open Source Reference Review — Round 9: MBSE and Assurance Cases

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V9.md

## 1. Scope

Round 9 reviewed:
- SysML v2 release/reference ecosystem
- OpenSysML
- Eclipse Capella
- Resolute
- SACM
- GSN-oriented open-source tools

Focus:
- model-based systems engineering artifacts;
- model-to-requirement/test traceability;
- assurance arguments for high-risk engineering decisions;
- whether Assurance Case should become a platform concept.

---

## 2. SysML v2 / MBSE

References:
- https://github.com/Systems-Modeling/SysML-v2-Release
- https://github.com/Open-MBEE/OpenSysML
- https://github.com/eclipse-capella/capella

SysML v2 provides:
- formal system-model language;
- requirements;
- interfaces/structure/behavior;
- analyses;
- satisfaction relationships;
- standardized API/services.

OpenSysML demonstrates executable/validated SysML v2 models.

### Optimization

Reserve immutable:

~~~text
ModelArtifact
~~~

Metadata:
- model kind/language;
- model version/spec version;
- source system/tool;
- snapshot/content digest;
- model element namespace;
- generator/export version;
- optional execution/validation receipt.

Possible kinds:
- SYSML_V2;
- CAPELLA;
- UML/SYSML_V1;
- AADL;
- CUSTOM.

---

## 3. Model elements participate through TraceLink

Do not create parallel MBSE authority.

Use typed:

~~~text
ModelElementRef
~~~

inside TraceLink.

Examples:
- Requirement AC SATISFIES model requirement;
- architecture component IMPLEMENTS interface;
- TestDefinition VERIFIES model constraint;
- Target component DERIVES_FROM model element.

Model snapshot digest is required for authority-bearing trace.

Mutable model URL is not enough.

---

## 4. Model execution/analysis as Evidence

Executable model or analysis output may produce:
- calculation result;
- constraint result;
- simulation result;
- consistency/validation result.

It becomes authoritative Evidence only when:
- exact ModelArtifact is bound;
- exact Procedure/tool/version is bound;
- issuer is trusted for that Evidence class;
- Subject Manifest includes the relevant model/inputs.

Model tooling itself does not decide Release authority.

---

## 5. Assurance Case

References:
- Resolute: https://github.com/loonwerks/Resolute
- SACM materials: https://github.com/SystemsAssuranceGroup/SACM
- GSN-oriented tools/research implementations

Resolute explicitly models:
- claims;
- subclaims;
- verification actions;
- assumptions;
- predicates/logic;
- assurance case hierarchy.

SACM/GSN provide standardized ways to represent structured assurance arguments.

### Decision

Do **not** require Assurance Case for ordinary A0/A1/A2 engineering.

For A3/A4 or externally regulated programs, reserve:

~~~text
AssuranceCaseArtifact
~~~

as an immutable structured argument over existing engineering facts.

---

## 6. Assurance Case structure

Minimal platform-neutral concepts:

~~~text
AssuranceCase
  subject_digest
  top_claims[]

Claim
  statement
  context/assumptions
  subclaims[]
  argument/rationale
  evidence_citations[]
  decision_citations[]
  challenges/open_issues[]
~~~

The case cites immutable:
- Evidence;
- Verification;
- Review;
- Risk Decisions;
- Model analysis;
- Release Manifest.

It does not copy/replace those records.

---

## 7. Assurance Case is not Verification

Verification answers:
> did required declared checks pass for this exact Subject?

Assurance Case answers:
> why do these checks, assumptions, analyses and reviews collectively justify a high-assurance claim?

Human/reviewer authority still decides adequacy.

No tool-generated GSN/SACM graph can self-authorize production.

---

## 8. Export/interchange

If high-assurance use appears, support:
- GSN-style visualization/export;
- SACM-style interchange where practical.

Keep internal schema small and provider-neutral until a real regulated/safety program requires deeper conformance.

Do not make Eclipse/GSN tooling a core dependency.

---

## 9. Assumptions must be visible

A useful Assurance Case practice is explicit assumptions/context.

For high-risk decisions:
- safety/environment assumption;
- hardware constraint;
- operational condition;
- test coverage boundary;
- model limitation

must be visible and linked.

If an assumption becomes false:
- Assurance Case applicability changes;
- Impact Analysis may trigger re-verification/review.

---

## 10. High-assurance profile

Extend Assurance Profile semantics conceptually:

~~~text
A0-A2
  Verification Plan + Evidence + Review as required

A3
  may require structured Assurance Summary/Case

A4
  may require explicit Claim/Argument/Evidence case
  + stronger independence/quorum/provenance
~~~

Exact company policy remains configurable.

This is a policy option, not a fixed regulatory claim.

---

## 11. M0/M1 impact

None on the critical path.

Only reserve Artifact kinds:
- MODEL;
- ASSURANCE_CASE.

TraceLink already supports model references later.

M1 does not implement:
- SysML repository;
- MBSE server;
- GSN editor;
- SACM engine.

---

## 12. M2/M3+ impact

When product/system complexity warrants:
- ingest immutable model snapshots;
- link model elements through TraceLink;
- use trusted model analyses as Evidence;
- introduce AssuranceCaseArtifact for high-risk releases.

---

## 13. New invariants

1. **MBSE model is an Artifact/source of engineering facts, not a second Control Plane authority.**
2. **Model traces bind immutable model snapshots/elements.**
3. **Model analysis becomes Evidence only through trusted Procedure/Issuer semantics.**
4. **Assurance Case cites Evidence; it does not manufacture Evidence.**
5. **Assurance Case is optional and risk-profile driven.**
6. **AI/tool-generated assurance arguments require human/independent review.**
7. **False/changed assumptions trigger applicability/impact analysis.**

---

## 14. Conclusion

Round 9 does not add an M1 requirement.

It provides a clean high-assurance extension path:

~~~text
Requirement / Target / Model
 -> TraceLink
 -> Verification / Evidence
 -> optional AssuranceCaseArtifact
 -> Human / Independent Review
 -> Release Authority
~~~

This preserves the light-weight normal engineering path while allowing future safety/regulated projects to build structured evidence arguments without redesigning the core platform.
