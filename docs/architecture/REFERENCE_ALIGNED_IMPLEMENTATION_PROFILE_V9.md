# Reference-Aligned Implementation Profile v9

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V8.md

Research basis:
- Open Source Reference Review Rounds 1–8
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v9 adds a lightweight semantic traceability layer without introducing a second requirements platform or graph database.

The key addition is:

~~~text
TraceLink
~~~

used for explicit requirement/design/code/test/evidence relationships that are valuable to query, validate and analyze.

---

## 2. Structural relationships remain normal domain fields

Do not model every relationship as TraceLink.

Examples that remain structural:
- TaskRevision belongs to Work;
- Run executes TaskRevision;
- Artifact produced by Run;
- Evidence evaluates Subject.

TraceLink is used only when the relationship itself carries traceability meaning.

---

## 3. TraceLink

Immutable record:

~~~text
trace_link_id
source_ref
target_ref
relation_type
provenance_kind
created_by
created_at
applicability
confidence?
generator/tool?
rationale?
evidence_ref?
~~~

Source/target are typed references.

Initial relation vocabulary:
- DERIVES_FROM
- REFINES
- IMPLEMENTS
- SATISFIES
- VERIFIES
- TESTS
- CONSTRAINS
- AFFECTS
- DEPENDS_ON
- SUPERSEDES

Vocabulary is versioned.

---

## 4. Trace provenance

Kinds:

~~~text
EXPLICIT
DERIVED
INFERRED
~~~

### EXPLICIT
Human/formal workflow created or approved.

### DERIVED
Reproducibly generated from domain/code/tool data.

### INFERRED
AI/heuristic proposal.

Rules:
- inferred link never satisfies an authority-bearing coverage gate by default;
- promotion to accepted trace requires policy/decision where needed;
- derived links store generator/version/input digest.

---

## 5. Code trace identity

Preferred:
- SCIP symbol ID + exact CodeIntelligenceArtifact/source-tree digest.

Fallback:
- repo identity;
- commit/tree digest;
- file path;
- stable range/snapshot reference.

Mutable branch references do not qualify as authority-bearing code trace targets.

---

## 6. Traceability Matrix

Trace Matrix is generated, not stored as a mutable source of truth.

Primary projection:

~~~text
Requirement / AC
 -> Task/Design
 -> Implementation
 -> TestDefinition/Procedure
 -> Evidence
 -> Verification
 -> Release
~~~

Other projections:
- Target/Interface -> affected Requirements/Tasks/Tests;
- Release -> originating Requirements/Decisions;
- Test -> covered ACs;
- source component -> rationale/requirements.

---

## 7. Trace Coverage

Generate findings such as:
- AC without implementation trace;
- AC without verification trace;
- implementation without Requirement/rationale;
- TestDefinition without requirement/quality objective;
- Evidence not consumed by current Verification;
- invalid/dangling/superseded trace.

Coverage metrics are projections.

Policy determines which gaps:
- block Requirement READY;
- block Release;
- require Review;
- remain warnings.

---

## 8. Trace integrity

Validate:
- typed-reference existence;
- source/target type compatibility for relation;
- source-tree alignment for code references;
- supersession applicability;
- duplicates/conflicts;
- cyclic relationship classes where prohibited.

Trace integrity failure is not silently ignored.

---

## 9. Impact analysis

Trace graph traversal proposes affected objects.

Path:

~~~text
changed Requirement / Target / Interface / source
 -> structural relations + TraceLinks
 -> candidate affected set
 -> ImpactAnalysisDecision
 -> replan / reverify / no-impact
~~~

The graph itself does not decide invalidation.

Policy + typed Decision remain authoritative.

---

## 10. Requirements source interoperability

Control Plane Requirement Revision remains canonical.

Future:

~~~text
RequirementSourceAdapter
~~~

may import from:
- WorkBuddy;
- requirements-as-code;
- StrictDoc/Doorstop-style repository;
- ReqIF;
- enterprise requirements platform.

External source becomes:
- snapshot Artifact;
- Context;
- proposed Requirement Revision input.

Import never silently overwrites a formal Requirement Revision.

---

## 11. Requirement interchange

Reserve:

~~~text
RequirementInterchangeArtifact
~~~

Fields:
- format;
- format_version;
- source_system;
- snapshot_digest;
- mapping_version;
- imported_at.

ReqIF is the primary embedded/automotive interchange candidate.

Not required in M1.

---

## 12. M1 minimal trace path

M1 implements only enough to prove:

~~~text
Requirement AC
 -> Task Revision
 -> CI/TestDefinition
 -> Evidence
 -> Verification
~~~

Implementation trace may begin at:
- repository/component;
- file;

and later become symbol-level via SCIP.

No trace graph service is required.

---

## 13. M2/M3 trace expansion

Add:
- Target/Interface traces;
- Procedure/HIL traces;
- Artifact/Release trace views;
- source symbol traces;
- impact analysis across multi-component product configuration;
- requirement interchange adapters.

---

## 14. No graph database by default

Use PostgreSQL relation tables and indexed materialized/read projections first.

Introduce graph storage only after measured query/scale needs.

---

## 15. Final rules added by v9

1. **Semantic traceability is explicit and queryable.**
2. **Structural ownership is not duplicated as arbitrary trace links.**
3. **Trace provenance distinguishes explicit, derived and inferred links.**
4. **AI-inferred traces are proposals, not authority.**
5. **Trace matrices and coverage are projections.**
6. **Trace impact analysis proposes affected scope; policy/Decision authorizes requalification.**
7. **External requirements remain source/context until formalized.**

Architecture v1.2 remains canonical.
Profile v9 is the current implementation companion.
