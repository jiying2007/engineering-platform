# M0 Reference Adoption Plan v9

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V8.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V9.md

## 1. Principle

M0 adds traceability semantics, not a graph platform.

The new work is:
- TraceLink schema;
- relation vocabulary;
- trace provenance;
- trace integrity;
- trace coverage projections.

---

## 2. TraceLink schema

Freeze:
- typed source/target refs;
- relation type;
- provenance kind;
- created_by/time;
- applicability;
- optional generator/version/input digest;
- optional confidence/rationale.

Define relation vocabulary v1:
- DERIVES_FROM
- REFINES
- IMPLEMENTS
- SATISFIES
- VERIFIES
- TESTS
- CONSTRAINS
- AFFECTS
- DEPENDS_ON
- SUPERSEDES.

---

## 3. Trace integrity fixtures

Create fixtures for:
- valid AC -> Task -> Test -> Evidence chain;
- dangling reference;
- source-tree mismatch;
- invalid relation type pairing;
- superseded target;
- AI-inferred link.

Prove:
- integrity failures are explicit;
- inferred links do not satisfy authority-bearing gates by default.

---

## 4. Trace coverage projections

Implement simple read projections:

~~~text
AC -> Task
AC -> TestDefinition
AC -> Evidence/Verification
~~~

Report:
- missing implementation trace;
- missing verification trace;
- dangling/invalid links.

Policy decides which are blockers.

No graph database required.

---

## 5. Minimal M1 trace

M1 must prove:

~~~text
Requirement AC
 -> Task Revision
 -> CI/TestDefinition
 -> Evidence
 -> Verification
~~~

Source implementation link may initially use repository/component/file identity.

SCIP symbol-level trace is optional.

---

## 6. Impact analysis fixture

Use one Requirement or Interface change.

Traverse:
- structural relations;
- TraceLinks.

Produce candidate impacted:
- Task;
- test;
- verification.

Then create explicit ImpactAnalysisDecision.

Prove:
- graph traversal alone does not mutate applicability;
- typed Decision performs actual requalification/no-impact decision.

---

## 7. Requirement source/interchange reservation

Freeze a minimal:

~~~text
RequirementInterchangeArtifact
~~~

No import implementation is required.

Fields:
- format/version;
- source system;
- snapshot digest;
- mapping version.

ReqIF/StrictDoc/Doorstop adapters are post-M1 work.

---

## 8. Existing M0 path remains

Still required from v8:
- canonicalization;
- Run Input / ExecutionSpec / Run Receipt;
- Build Definition / Receipt;
- Integration Subject / Eligibility;
- Session Grant/Gateway;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Attestation/Verification;
- Release Admission;
- OTel/CDEvents;
- TestReport/BOM/Finding;
- failure injection.

---

## 9. M0 exit additions

M0 additionally requires:
- TraceLink schema/version frozen;
- integrity fixtures pass;
- trace coverage projection can identify missing AC verification;
- inferred link cannot satisfy formal trace coverage by default;
- impact analysis creates an explicit Decision rather than mutating state from graph traversal.

No graph database is required.

---

## 10. M1 target

The M1 loop remains operationally small while now producing a queryable formal thread from Requirement AC through Verification.
