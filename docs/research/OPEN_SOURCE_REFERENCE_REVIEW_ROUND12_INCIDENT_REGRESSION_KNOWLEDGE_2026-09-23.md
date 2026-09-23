# Open Source Reference Review — Round 12: Incident, Regression, Bisection and Knowledge Closure

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V11.md

## 1. Scope

Round 12 reviewed:
- Netflix Dispatch (historical/archived)
- Keep
- Robusta / HolmesGPT patterns
- Sentry
- ClusterFuzz / OSS-Fuzz
- mozregression
- rustc-perf / Go perf
- Rundeck / StackStorm-style runbook automation

Focus:
- incident aggregation;
- change/alert correlation;
- AI-assisted investigation;
- reproducible regression diagnosis;
- automated bisection;
- postmortem;
- knowledge extraction.

---

## 2. Alert/Finding is not Incident

Monitoring/security/test systems may emit many:
- alerts;
- Findings;
- crashes;
- test regressions;
- device anomalies.

Keep/Sentry/ClusterFuzz demonstrate the value of grouping/deduplication.

Add stable:

~~~text
Incident
~~~

An Incident may aggregate many source signals.

Fields/concepts:
- incident_id;
- classification/severity;
- affected Target/Release/Artifact/environment;
- detection references;
- timeline;
- owner/coordinator;
- status;
- impact;
- linked Work/Risk/Release;
- closure/postmortem refs.

Source signals remain immutable and are not deleted when grouped.

---

## 3. Incident correlation is a hypothesis

Alert correlation or AI grouping can propose:
- same root cause;
- same rollout/change;
- same device population;
- duplicate crash signature.

Represent as:

~~~text
IncidentCorrelation
  source signals/incidents
  rationale
  method/tool/version
  confidence
  provenance
  accepted/rejected status
~~~

AI/correlation engine output cannot silently merge authoritative history.

Human/policy-approved grouping controls Incident membership where material.

---

## 4. Investigation hypothesis

Do not store AI RCA prose as Root Cause truth.

Use:

~~~text
InvestigationHypothesis
~~~

Fields:
- statement;
- supporting observations;
- contradicting observations;
- source/provenance;
- confidence;
- proposed tests;
- status:
  PROPOSED
  SUPPORTED
  REJECTED
  INCONCLUSIVE.

A hypothesis becomes accepted root-cause conclusion only through explicit investigation/closure Decision.

---

## 5. Change correlation

Robusta/Sentry-style change correlation is useful:

~~~text
Incident
 -> recent Release/Promotion
 -> configuration change
 -> dependency change
 -> infrastructure/tool change
~~~

But temporal proximity alone is not causation.

Use TraceLinks/Impact records to propose likely related changes.
Verification/reproduction establishes stronger evidence.

---

## 6. Reproduction is first-class

Add:

~~~text
ReproductionCase
~~~

Includes:
- Incident/Finding reference;
- exact subject/release/config;
- environment/fidelity;
- steps/procedure;
- input/corpus/testcase;
- expected/observed behavior;
- repeatability statistics;
- raw Evidence refs.

A deterministic or statistically characterized reproduction materially strengthens diagnosis.

---

## 7. RegressionRange

mozregression/ClusterFuzz demonstrate narrowing a known-good / known-bad range.

Add:

~~~text
RegressionRange
  dimension
  known_good_ref
  known_bad_ref
  candidate_space
  reproduction_case
  result/status
~~~

Possible dimensions:
- Git commit;
- Release;
- firmware component version;
- configuration;
- model/runtime version;
- dependency/toolchain.

The range is evidence about where a regression was introduced, not yet root cause.

---

## 8. Bisection Plan and Attempts

Automated bisection should be explicit:

~~~text
BisectionPlan
  regression_range
  candidate ordering/graph
  reproduction procedure
  environment
  verdict rule
  retry/statistical policy
  budget
~~~

Each:

~~~text
BisectionAttempt
  candidate
  exact Artifact/Subject
  result
  confidence/statistics
  evidence
~~~

Final output:

~~~text
BisectionResult
  first_bad / suspect_set
  confidence
  excluded candidates
  limitations
~~~

Never hide skipped/unbuildable/inconclusive candidates.

---

## 9. Bisection is not Root Cause

A first bad commit may contain:
- multiple changes;
- generated artifacts;
- dependency updates;
- latent defect exposure.

Therefore:

~~~text
first_bad_commit
!=
root_cause_decision
~~~

It creates a strong investigation input and affected scope.

---

## 10. Benchmark/performance regression

rustc-perf and Go perf show:
- controlled benchmark corpus;
- stable environment;
- repeated measurements;
- statistical comparison;
- known benchmark variance.

For performance regressions, Reproduction/Bisection must include:
- benchmark definition/version;
- environment definition;
- repetitions/samples;
- statistic/confidence method;
- noise/variance metadata.

Do not decide performance regression from a single noisy measurement.

---

## 11. Fix Verification

Incident closure requires proof that the claimed fix addresses the reproduced failure.

Add typed relationship:

~~~text
FixVerification
  incident
  fix Integration Subject / Release
  reproduction case
  verification evidence
  regression coverage added
  result
~~~

Preferred:
1. failure reproduces on known-bad;
2. proposed fix passes same reproduction;
3. required broader regression suite passes;
4. release/target evidence is refreshed.

---

## 12. Regression test promotion

When an Incident exposes a durable defect class, convert the reproduction into:
- TestDefinition;
- Procedure Revision;
- fuzz corpus/testcase;
- known-negative case;
- continuous qualification input.

This conversion is explicit and versioned.

An ad-hoc debug script does not automatically become a permanent test.

---

## 13. Postmortem

Dispatch-style incident orchestration and post-incident review support a structured:

~~~text
PostmortemArtifact
~~~

Contents:
- incident summary;
- user/product impact;
- timeline;
- detection;
- contributing factors;
- accepted root cause Decision;
- what went well/poorly;
- corrective actions;
- regression prevention;
- open risks;
- follow-up Work;
- Evidence citations.

Postmortem cites immutable facts rather than rewriting them.

---

## 14. Knowledge candidate, not automatic Knowledge

Do not automatically publish Incident chat/log/RCA into long-term engineering Knowledge.

Create:

~~~text
KnowledgeCandidate
~~~

Derived from closed Incident/Postmortem.

Candidate types:
- Known Issue;
- Recovery Runbook;
- Design Rule;
- Compatibility Rule;
- Test Method;
- Diagnostic Pattern.

Fields:
- source Incident/Postmortem;
- proposed content;
- scope/Target;
- evidence/decision refs;
- curator;
- freshness/review date;
- status.

Only curated/approved candidates become durable Knowledge.

---

## 15. Runbook automation

Rundeck/StackStorm-style automation is useful for:
- collecting diagnostics;
- safe recovery;
- repeated incident operations.

Map to existing Procedure/Action Gateway semantics.

A Runbook:
- has version;
- required capabilities;
- preconditions;
- inputs;
- expected outputs;
- rollback/recovery.

High-risk recovery still requires policy/Human Authority.

---

## 16. Incident lifecycle

Suggested:

~~~text
DETECTED
 -> TRIAGED
 -> INVESTIGATING
 -> MITIGATING
 -> MONITORING
 -> RESOLVED
 -> POSTMORTEM_PENDING
 -> CLOSED
~~~

Side states:
- DUPLICATE;
- FALSE_POSITIVE;
- SUPERSEDED.

Resolution does not imply knowledge closure or corrective actions complete.

---

## 17. Corrective actions

Postmortem actions become explicit:
- Work;
- Risk;
- Test/Procedure update;
- monitoring/SLO update;
- Tool/Policy update;
- KnowledgeCandidate.

They are not checklist text trapped inside a document.

---

## 18. Incident-to-release feedback

A confirmed Incident may:
- quarantine a Release/Artifact;
- suspend an Evidence Issuer/toolchain;
- trigger rollback PromotionPlan;
- mark Evidence applicability stale;
- create Reverification requirements.

These effects occur through typed Decisions/policy, not from an alerting system directly.

---

## 19. M1 impact

M1 does not require an Incident platform.

Reserve:
- Incident;
- InvestigationHypothesis;
- ReproductionCase;
- PostmortemArtifact;
- KnowledgeCandidate.

Bisection schemas may remain post-M1 if desired.

---

## 20. M2/M3 impact

Add as real field data appears:
- incident ingestion/correlation;
- device failure diagnostics;
- automated Reproduction;
- regression bisection;
- test promotion;
- knowledge curation.

Potential external systems remain integrations/read sources, not authority.

---

## 21. New invariants

1. **Alert/Finding is not Incident.**
2. **Correlation/AI RCA is a hypothesis, not root-cause authority.**
3. **Reproduction is explicit, exact-subject evidence.**
4. **Bisection result narrows regression origin but does not automatically establish root cause.**
5. **Incident fix closure cites Fix Verification.**
6. **Every permanent regression test is explicitly promoted/versioned.**
7. **Postmortem cites immutable facts rather than rewriting history.**
8. **Incident content becomes Knowledge only through curated KnowledgeCandidate promotion.**
9. **Corrective actions become real Work/Risk/Test/Policy objects.**
10. **Incident systems cannot directly revoke/promote Release without typed policy Decisions.**

---

## 22. Conclusion

Round 12 defines the feedback loop missing from a purely forward delivery pipeline:

~~~text
Release / Field
 -> Finding / Incident
 -> Investigation
 -> Reproduction
 -> Bisection / Root Cause Decision
 -> Fix
 -> Fix Verification
 -> Regression Test
 -> Postmortem
 -> Corrective Work
 -> Curated Knowledge
~~~

This creates a closed engineering-learning loop without turning noisy operational data or AI summaries into authority.
