# M0 Reference Adoption Plan v12

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V11.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V12.md

## 1. Principle

Round 12 adds post-release/field feedback semantics without adding an Incident service to M1.

M0 only freezes enough structure to ensure later Incident/Regression/Knowledge loops use the same authority model.

---

## 2. Incident fixture

Create one synthetic Incident linked to:
- Release Manifest;
- Artifact;
- Finding;
- RuntimeConfigurationSnapshot.

Prove:
- source Finding remains immutable;
- Incident grouping does not rewrite the Finding;
- Incident can reference related Release/Artifact without mutating them.

---

## 3. Investigation hypothesis fixture

Create:
- one AI/heuristic-proposed InvestigationHypothesis;
- one contradicting Evidence item.

Prove:
- hypothesis status is separate from RootCauseDecision;
- AI output cannot mark root cause accepted.

---

## 4. Reproduction fixture

Create a ReproductionCase for the synthetic Incident.

Bind:
- exact known-bad subject;
- environment;
- procedure;
- expected/observed behavior;
- Evidence.

Prove:
- reproduction is independently verifiable;
- changing environment/config creates a distinct reproduction subject.

---

## 5. Fix verification fixture

Create:
- synthetic fix Integration Subject;
- same ReproductionCase;
- passing Evidence;
- broader regression Evidence.

Prove:
- merge alone cannot close Incident;
- FixVerification is required for resolved/closed status in the fixture policy.

---

## 6. Knowledge candidate fixture

Create a KnowledgeCandidate from the closed synthetic Incident.

Prove:
- raw Incident/Postmortem cannot directly become PUBLISHED Knowledge;
- curator/review/provenance are required;
- published knowledge retains source Incident/Postmortem references.

---

## 7. Bisection schema reservation

Freeze minimal:
- RegressionRange;
- BisectionPlan;
- BisectionAttempt;
- BisectionResult.

No automated bisection implementation is required in M0/M1.

---

## 8. Existing M0 path remains

Still required:
- core schema/canonicalization;
- TraceLink;
- ToolProfile/RuntimeQualificationProfile;
- Run Input / ExecutionSpec / Receipt;
- Build Definition / Receipt;
- Integration Subject / Eligibility;
- Session Grant;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Verification;
- Release Admission/Reconciliation;
- OTel/CDEvents;
- TestReport/BOM/Finding;
- failure injection.

---

## 9. M0 exit additions

M0 additionally verifies:
- Incident grouping does not rewrite source signals;
- AI RCA remains a hypothesis;
- FixVerification is distinct from fix merge;
- KnowledgeCandidate requires curation before publication;
- synthetic Incident can trace Release -> Finding -> Reproduction -> Fix Verification -> KnowledgeCandidate.

No external Incident/Knowledge platform is required.

---

## 10. M1 target

M1 remains unchanged.

Round 12 only guarantees that the future reverse feedback loop can attach cleanly to Release/Artifact/Evidence without redesigning the forward delivery model.
