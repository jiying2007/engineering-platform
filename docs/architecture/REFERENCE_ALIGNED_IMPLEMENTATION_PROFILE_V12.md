# Reference-Aligned Implementation Profile v12

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V11.md

Research basis:
- Open Source Reference Review Rounds 1–12
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v12 adds the closed-loop semantics from production/field failures back into engineering:

~~~text
Finding
 -> Incident
 -> Investigation
 -> Reproduction
 -> Regression Range / Bisection
 -> Root Cause Decision
 -> Fix
 -> Fix Verification
 -> Regression Test
 -> Postmortem
 -> Corrective Work
 -> KnowledgeCandidate
~~~

No external Incident platform becomes M1 authority.

---

## 2. Incident

Add stable:

~~~text
Incident
~~~

Fields/concepts:
- incident_id;
- type/classification;
- severity;
- affected Target/Release/Artifact/environment;
- detection/source signal refs;
- owner/coordinator;
- impact;
- timeline refs;
- related changes/releases;
- linked Risk/Work;
- lifecycle state;
- postmortem/closure refs.

An Incident groups signals; source alerts/findings remain immutable.

---

## 3. Incident correlation

Add:

~~~text
IncidentCorrelation
~~~

Contains:
- signals/incidents being correlated;
- method/tool/version;
- rationale;
- confidence;
- provenance;
- accepted/rejected status.

AI/heuristic correlation is a proposal.

It does not silently merge or rewrite Incident history.

---

## 4. InvestigationHypothesis

Use:

~~~text
InvestigationHypothesis
~~~

rather than storing AI RCA as fact.

Fields:
- statement;
- support evidence;
- contradicting evidence;
- source/tool;
- confidence;
- proposed verification/reproduction;
- status.

Status:
- PROPOSED;
- SUPPORTED;
- REJECTED;
- INCONCLUSIVE.

Accepted root cause is a separate Decision.

---

## 5. ReproductionCase

Add immutable/versioned:

~~~text
ReproductionCase
~~~

Includes:
- Incident/Finding;
- exact Subject/Release/RuntimeConfiguration;
- VerificationEnvironment;
- Procedure/steps;
- input/corpus/testcase;
- expected vs observed behavior;
- retry/statistical rules;
- repeatability result;
- Evidence refs.

Reproduction is the bridge from operational observation back to engineering verification.

---

## 6. Regression and bisection

Reserve:
- RegressionRange;
- BisectionPlan;
- BisectionAttempt;
- BisectionResult.

Regression dimensions may include:
- source commit;
- Release;
- firmware component;
- dependency;
- toolchain;
- Runtime/model;
- configuration.

Rules:
- every candidate is exact-subject bound;
- skipped/unbuildable/inconclusive candidates remain visible;
- first bad candidate is not automatically root cause.

---

## 7. RootCauseDecision

An accepted root cause is a typed Decision that cites:
- Incident;
- hypotheses;
- Reproduction;
- bisection/result;
- diagnostic Evidence;
- related change/Artifact.

AI or automation may propose it.

Required authority/review comes from policy/Assurance Profile.

---

## 8. FixVerification

Incident resolution/closure should reference:

~~~text
FixVerification
~~~

Minimum:
- Incident;
- fix Integration Subject/Release;
- ReproductionCase;
- Evidence that known-bad reproduces where appropriate;
- Evidence fix passes;
- broader regression Evidence;
- result.

"PR merged" is not equivalent to "Incident fixed".

---

## 9. Regression test promotion

A useful incident reproduction may be promoted into:
- TestDefinition;
- Procedure Revision;
- fuzz corpus/testcase;
- Continuous Qualification input.

Promotion is explicit/versioned.

Ad-hoc incident scripts/log queries do not silently become long-term tests.

---

## 10. PostmortemArtifact

Postmortem is an immutable document/artifact citing formal records.

Contains:
- summary;
- impact;
- timeline;
- detection;
- contributing factors;
- RootCauseDecision;
- mitigation/fix;
- FixVerification;
- corrective actions;
- open risks;
- lessons.

It does not rewrite source facts.

---

## 11. Corrective action extraction

Postmortem actions become typed platform work:
- Work/Task;
- Risk;
- Test/Procedure update;
- Policy update;
- ToolProfile/runtime qualification update;
- Monitoring/OperationalObjective update;
- KnowledgeCandidate.

Avoid untracked checklist text.

---

## 12. KnowledgeCandidate

Incident/Postmortem content does not automatically become Knowledge.

Add:

~~~text
KnowledgeCandidate
~~~

Types:
- KNOWN_ISSUE;
- RECOVERY_RUNBOOK;
- DESIGN_RULE;
- COMPATIBILITY_RULE;
- TEST_METHOD;
- DIAGNOSTIC_PATTERN.

Fields:
- source Incident/Postmortem;
- proposed scope/Target;
- evidence/decision refs;
- content/snapshot digest;
- curator;
- review/freshness date;
- status.

Only curated/approved candidate becomes durable Engineering Knowledge.

---

## 13. Knowledge lifecycle

Suggested:

~~~text
CANDIDATE
 -> REVIEWED
 -> PUBLISHED
 -> SUPERSEDED
    | RETIRED
~~~

Published Knowledge retains:
- provenance;
- scope;
- validity assumptions;
- version;
- curator;
- next review/freshness.

Runtime-retrieved Knowledge is still Context, not Authority.

---

## 14. Runbook automation

Recovery Runbooks may execute via existing Procedure/Action Gateway semantics.

Each Runbook:
- version;
- capabilities;
- preconditions;
- input contract;
- steps/actions;
- output/receipt contract;
- failure/rollback behavior.

High-risk repair/recovery remains policy/Human Authority gated.

Rundeck/StackStorm are references, not required dependencies.

---

## 15. Incident effects on Release/Evidence

Incident may trigger proposals to:
- quarantine Release/Artifact;
- rollback PromotionPlan;
- invalidate/re-evaluate Evidence applicability;
- suspend Issuer/toolchain;
- require Reverification.

Actual state changes occur via typed policy Decisions.

Alert/incident tooling cannot directly mutate release authority.

---

## 16. Continuous Qualification integration

Future recurring qualification can generate Incidents/Findings when:
- fuzzing finds crash;
- soak finds reliability regression;
- benchmark finds statistically significant degradation;
- virtual/HIL nightly finds product regression.

These follow the same Incident/Reproduction/Fix loop.

---

## 17. Performance regression

Performance RegressionCase must bind:
- benchmark definition;
- environment;
- Artifact/Subject;
- repetitions/samples;
- metric unit/statistical method;
- variance/noise.

Single noisy observations do not establish regression.

---

## 18. Milestone mapping

### M0
Freeze/reserve:
- Incident;
- InvestigationHypothesis;
- ReproductionCase;
- RootCauseDecision;
- FixVerification;
- PostmortemArtifact;
- KnowledgeCandidate.

Bisection can remain minimal/reserved.

### M1
No Incident service required.

One synthetic fixture should prove:
- field Finding/Incident can link back to Release/Artifact;
- Knowledge cannot be published directly from raw Incident content.

### M2/M3
Implement as real field/device data accumulates:
- Incident ingest/grouping;
- automatic FFDC;
- reproduction;
- bisection;
- regression test promotion;
- knowledge curation.

---

## 19. Final rules added by v12

1. Alerts/findings are immutable signals; Incident is a separate grouping/lifecycle.
2. AI RCA/correlation is hypothesis, not root-cause authority.
3. Reproduction is explicit engineering Evidence.
4. Bisection narrows origin but does not decide root cause automatically.
5. Incident fix requires FixVerification.
6. Regression tests are promoted explicitly.
7. Postmortem cites formal immutable facts.
8. Corrective actions become real platform objects.
9. Knowledge publication requires curated KnowledgeCandidate promotion.
10. Incident systems cannot directly mutate Release/Evidence authority.

Architecture v1.2 remains canonical.
Profile v12 is the current implementation companion.
