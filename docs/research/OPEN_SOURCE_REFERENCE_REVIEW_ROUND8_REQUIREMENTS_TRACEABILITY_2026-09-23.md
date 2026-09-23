# Open Source Reference Review — Round 8: Requirements Traceability and Digital Thread

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V8.md

## 1. Scope

Round 8 reviewed requirements/traceability practices from:
- Eclipse Capra
- StrictDoc
- Doorstop
- OpenFastTrace
- Eclipse RMF / ReqIF ecosystem concepts

Focus:
- requirement-to-artifact trace links;
- trace matrices;
- link integrity;
- impact analysis;
- docs-as-code requirements;
- requirements interchange.

---

## 2. Eclipse Capra — trace links across arbitrary engineering artifacts

Repository:
- https://github.com/eclipse-capra/capra

Capra allows trace links between arbitrary artifacts through adapters:
- requirements;
- source code;
- UML/SysML/AUTOSAR/AADL model elements;
- issue tracker items;
- Office/Docs;
- CI test executions.

It supports:
- trace-link consistency;
- change notifications;
- relationship graph visualization;
- traceability matrices;
- change-impact analysis.

### Optimization

engineering-platform should explicitly model semantic trace relationships instead of relying only on foreign keys scattered across objects.

Add immutable:

~~~text
TraceLink
~~~

Fields:
- trace_link_id;
- source typed reference;
- target typed reference;
- relation_type;
- provenance/source;
- created_by;
- created_at;
- applicability/status;
- confidence/trust where derived;
- optional rationale/evidence reference.

---

## 3. Structural relation vs semantic trace

Do not duplicate normal domain ownership relationships.

### Structural domain relation

Examples:
- TaskRevision belongs to Work;
- Evidence evaluates Subject;
- Run executes TaskRevision;
- Artifact produced by Run.

These remain normal domain fields/foreign keys.

### Semantic TraceLink

Examples:
- Requirement AC is implemented by source component;
- Requirement is satisfied by design element;
- TestDefinition verifies AC;
- Procedure verifies interface contract;
- Finding affects Requirement/Risk;
- design rule constrains Target;
- source symbol implements interface.

TraceLink exists where the relationship itself is valuable to query/audit and is not simply object ownership.

---

## 4. Trace relation vocabulary

Start with a small controlled vocabulary:

~~~text
DERIVES_FROM
REFINES
IMPLEMENTS
SATISFIES
VERIFIES
TESTS
CONSTRAINS
AFFECTS
DEPENDS_ON
SUPERSEDES
~~~

Do not create dozens of relation types before real Work demands them.

Relation semantics and direction must be documented and versioned.

---

## 5. Trace target references can span artifact types

Typed references may target:
- Requirement / AC;
- Work / Task Revision;
- Target / Interface Contract;
- Design document/section;
- source file;
- source symbol;
- CodeIntelligence symbol;
- TestDefinition;
- Procedure Revision;
- Artifact;
- Evidence;
- Verification;
- Risk/Decision;
- Release.

For source symbols, SCIP identifiers are a useful portable reference where available.

Fallback references may use:
- repository + commit/tree + file + line/range;
- content snapshot digest.

Mutable branch/URL is never enough for authority-bearing trace.

---

## 6. Explicit vs derived trace links

Trace links need provenance.

Kinds:

~~~text
EXPLICIT
  created/accepted by human or formal workflow

DERIVED
  generated from domain structure, code analysis or tooling

INFERRED
  proposed by AI/heuristic
~~~

Rules:
- inferred trace never becomes authority automatically;
- a policy may require review/promotion before an inferred link can satisfy trace coverage;
- derived links are reproducible from declared source/tool/version where possible.

This is especially important once AI begins suggesting requirement-code-test relationships.

---

## 7. Traceability Matrix is a projection

Do not store a separate mutable traceability matrix as authority.

Generate projections such as:

~~~text
Requirement AC
 -> Task
 -> Implementation
 -> TestDefinition
 -> Evidence
 -> Verification
~~~

and:

~~~text
Target / Interface
 -> dependent Requirements
 -> Tasks
 -> Artifacts
 -> Verification coverage
~~~

The matrix is a view over current domain relationships + TraceLinks.

---

## 8. StrictDoc / Doorstop — requirements-as-code practices

Repositories:
- https://github.com/strictdoc-project/strictdoc
- https://github.com/doorstop-dev/doorstop

Useful practices:
- textual requirements in version control;
- stable IDs;
- requirement/test hierarchy;
- link validation;
- generated reports;
- CI-friendly trace checks.

### Decision

WorkBuddy/Control Plane Requirement Revision remains canonical authority.

But support future import/export/adapters for requirements-as-code when useful.

Possible:

~~~text
RequirementSourceAdapter
  WorkBuddy
  StrictDoc-like repository
  ReqIF import
  external enterprise requirement system
~~~

Do not require StrictDoc/Doorstop in M1.

---

## 9. OpenFastTrace — trace coverage and obsolete artifacts

Repository:
- https://github.com/itsallcode/openfasttrace

OpenFastTrace emphasizes:
- whether every specified requirement is implemented;
- whether implementation artifacts have corresponding requirements;
- trace completeness;
- identifying obsolete/unjustified product parts.

### Optimization

Add Trace Coverage projections:

~~~text
AC without implementation link
AC without verification link
implementation without requirement/rationale
TestDefinition without requirement/quality objective
Evidence without current Verification use
~~~

These are engineering-health/readiness findings.

Policy may turn specific missing trace categories into:
- Requirement READY blocker;
- Release gate;
- review warning.

---

## 10. Change impact uses trace graph plus explicit rules

Capra-style impact analysis supports:

~~~text
changed Requirement/Target/Interface
 -> traverse relevant trace relations
 -> identify possibly impacted:
    Task
    source component
    test/procedure
    Artifact
    Verification
    Release
~~~

Trace traversal proposes impact.

Final applicability/requalification decision remains a typed ImpactAnalysisDecision under policy.

A graph edge alone does not automatically invalidate everything.

---

## 11. Trace integrity checks

M0/M1 should validate:
- dangling typed references;
- reference to superseded current-only object where disallowed;
- direction/type incompatibility;
- duplicate conflicting links;
- source-tree mismatch for code symbol/file links;
- invalid Requirement/AC IDs.

Trace integrity is separate from trace coverage.

---

## 12. ReqIF interoperability

ReqIF is useful when exchanging requirements with established automotive/embedded requirements tools.

Reserve:

~~~text
RequirementInterchangeArtifact
~~~

with:
- format;
- format_version;
- source system;
- snapshot digest;
- import/export mapping version.

ReqIF import/export is future work, not M1.

The imported document is source/context until formally converted into a Requirement Revision.

---

## 13. No trace graph database in M1

Use PostgreSQL typed edges and indexed projections first.

Add a graph database only if:
- cross-project trace traversal becomes a measurable bottleneck;
- relationship volume/queries justify it.

This matches the broader strategy already used for code intelligence and GUAC.

---

## 14. M0/M1 impact

### M0
Freeze:
- TraceLink schema;
- relation vocabulary v1;
- provenance/trust semantics;
- trace integrity rules;
- Trace Coverage projection definitions.

### M1
Implement a minimal trace path:

~~~text
Requirement AC
 -> Task Revision
 -> TestDefinition / CI check
 -> Evidence
 -> Verification
~~~

Implementation/source code trace may initially be at repository/component granularity.

### M2+
Add:
- Target/Interface traces;
- source file/symbol traces;
- Procedure/HIL traces;
- ReqIF/external adapters.

---

## 15. New invariants

1. **Structural ownership and semantic traceability are distinct.**
2. **Trace matrices are projections, not mutable authority.**
3. **AI-inferred trace links never become authority automatically.**
4. **Authority-bearing source-code traces bind immutable source identity.**
5. **Missing trace coverage is explicit, never silently assumed.**
6. **Trace traversal proposes change impact; policy/Decision determines actual requalification.**
7. **External requirement files are source/context until converted into a formal Requirement Revision.**

---

## 16. Conclusion

Round 8 strengthens the digital engineering thread without introducing another requirements platform.

The architecture gains one lightweight durable relation type:

> **TraceLink**

This makes Requirement -> Design/Task -> Code/Test -> Evidence -> Verification traceability queryable, reviewable and suitable for impact analysis while preserving the existing Control Plane authority model.
