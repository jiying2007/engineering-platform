# Open Source Reference Review — Round 20: AI Code Review Quality and Reviewer Independence

Date: 2026-09-24
Status: **Archived research / implementation input**

Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md

## 1. Scope

Round 20 reviewed:
- The PR-Agent / Qodo open-source reviewer
- reviewdog / Danger patterns
- Google Code Review guidance
- Code Review Bench
- Alibaba AACR-Bench
- SWE-PRBench
- CodeReviewQA / related 2025–2026 research

Focus:
- AI reviewer output as structured engineering data;
- reviewer independence;
- reviewer qualification;
- false-positive/noise handling;
- benchmark design;
- review context quality;
- high-risk human review boundaries.

---

## 2. AI review is a Review Execution, not approval

A code-review agent may:
- analyze diff/context;
- produce findings;
- classify severity/category;
- suggest fixes;
- ask questions.

It does not automatically:
- approve Integration;
- satisfy independent Review;
- close Findings;
- prove Verification.

Formal model:

~~~text
ReviewSubject
 -> ReviewExecution
 -> ReviewFinding(s)
 -> ReviewResolution(s)
 -> ReviewVerdict
 -> Decision / Integration Eligibility
~~~

---

## 3. ReviewSubject must be frozen

Review binds:
- exact Integration Subject digest;
- candidate source trees;
- Requirement/Task context;
- Interface/Target refs;
- selected Verification Evidence;
- open Risks;
- review scope/profile.

If candidate source changes:
- review applicability is invalidated or explicitly carry-forward evaluated;
- comments may remain historical but cannot silently approve the new subject.

---

## 4. ReviewExecution

Add immutable:

~~~text
ReviewExecution
~~~

Fields:
- review_execution_id;
- ReviewSubject digest;
- reviewer execution identity;
- reviewer kind;
- Runtime/model/profile;
- Runtime Adapter;
- ToolProfile;
- context profile/digests;
- instructions/profile;
- started/completed;
- output Artifact;
- qualification evidence ref;
- independence level;
- status.

Reviewer kinds:
- HUMAN;
- AI_RUNTIME;
- STATIC_TOOL;
- HYBRID.

---

## 5. ReviewFinding

Every actionable review issue becomes:

~~~text
ReviewFinding
~~~

Fields:
- finding ID;
- ReviewExecution;
- exact source location/symbol;
- category;
- severity;
- statement;
- rationale;
- evidence/context refs;
- suggested action;
- confidence if tool-generated;
- fingerprint/dedup key;
- status.

Categories may include:
- DEFECT;
- SECURITY;
- CONCURRENCY;
- DATA;
- API_COMPATIBILITY;
- PERFORMANCE;
- TEST_GAP;
- MAINTAINABILITY;
- DOCUMENTATION;
- STYLE;
- SPECULATIVE.

The platform should not rely on free-form PR comments alone.

---

## 6. ReviewResolution

A finding is resolved separately:

~~~text
ReviewResolution
~~~

Disposition:
- FIXED;
- ACCEPTED_RISK;
- FALSE_POSITIVE;
- DUPLICATE;
- NOT_APPLICABLE;
- SUPERSEDED;
- DEFERRED;
- DISPUTED.

Links:
- fix commit/change;
- Evidence;
- Risk/Decision;
- resolver/reviewer;
- rationale.

AI reviewer cannot mark its own finding FALSE_POSITIVE or FIXED without actual evidence/workflow.

---

## 7. ReviewVerdict

ReviewVerdict is derived from:
- required reviewer classes;
- unresolved findings;
- severity policy;
- independence;
- qualification;
- required human decisions.

Possible result:
- PASS;
- FAIL;
- CONDITIONAL;
- INCOMPLETE.

"AI returned no comments" is not automatically PASS.

---

## 8. Reviewer independence

Reuse existing independence ladder but make execution identity explicit.

Example:

~~~text
R0  implementation self-check
R1  new session, same provider/model family
R2  independent Run/context with no implementation conversation
R3  different runtime/provider or separately qualified reviewer system
R4  human domain specialist / designated authority
~~~

Policy chooses minimum required level.

Critical rule:

> Same implementation execution must not satisfy independent review merely by running another prompt over its own output.

---

## 9. AI reviewer qualification

Add reviewer-specific:

~~~text
ReviewerQualificationProfile
~~~

which may extend RuntimeQualificationProfile.

Includes:
- languages/domains;
- issue categories;
- context profile;
- benchmark corpus;
- required precision/recall/noise;
- severity-weighted requirements;
- privacy/data class;
- model/provider/version;
- ToolProfile.

Output:

~~~text
ReviewerQualificationEvidence
~~~

Eligibility is scoped.

Example:
- qualified for Go/Python maintainability review;
- not qualified to satisfy security-critical C review.

---

## 10. Benchmark design

Code Review Bench and AACR-Bench demonstrate useful complementary approaches.

### Offline frozen benchmark

Advantages:
- reproducible;
- curated golden findings;
- detailed category/severity analysis.

Risks:
- training/memorization leakage;
- benchmark overfitting.

### Online/fresh benchmark

Advantages:
- recent unseen PRs;
- measures real developer response;
- less memorization.

Risks:
- weaker/indirect ground truth;
- selection bias;
- judge uncertainty.

### Decision

ReviewerQualification should combine:
- frozen regression corpus;
- fresh/rolling corpus;
- project-specific replay set.

No single public benchmark is sufficient authority.

---

## 11. Review metrics

Track at least:
- precision;
- recall;
- severity-weighted recall;
- critical miss rate;
- noise/false-positive rate;
- line/location precision;
- duplicate finding rate;
- actionable acceptance/fix rate;
- latency;
- cost.

Do not optimize a reviewer only for comment volume.

---

## 12. Ground truth is imperfect

Human historical review comments are not complete truth.

Useful ground-truth sources include:
- expert-curated findings;
- post-review fixes;
- later Incident/regression;
- security Finding;
- generated counterexample;
- customer/field defect.

When a reviewer misses an issue later proven real:
- add to failure replay/qualification corpus;
- update evaluator;
- retain provenance.

This mirrors KWS hard-negative/failure replay.

---

## 13. LLM judge is also a tool

If an LLM judges review quality, record:
- judge model/version;
- prompt/profile;
- input digest;
- judgment;
- confidence/uncertainty.

Judge output is Evaluation Evidence, not unquestionable truth.

Where important:
- multi-judge agreement;
- expert adjudication;
- sampled audit

can be required.

---

## 14. Context profile is part of qualification

Recent benchmarks show context choice materially affects reviewer behavior.

Therefore freeze:

~~~text
ReviewContextProfile
~~~

Examples:
- DIFF_ONLY;
- DIFF_PLUS_FILES;
- REPOSITORY_MAP;
- TARGETED_SYMBOL_CONTEXT;
- REQUIREMENT_PLUS_DIFF;
- FULL_REPO_RETRIEVAL.

Do not assume "more context is always better".

Qualify the exact context strategy.

---

## 15. Review selection by risk

Possible policy:

### Low-risk
- static checks;
- qualified AI review;
- human optional according to team policy.

### Medium-risk
- AI + independent human review;
- or multiple qualified reviewer classes.

### High-risk/A3/A4
- AI review as supplemental;
- designated independent human/domain review required;
- formal/security/HIL evidence as applicable.

AI reviewer quality metrics may reduce toil, but do not erase required human authority.

---

## 16. Review automation hygiene

Borrow reviewdog/Danger practice:

Automate deterministic chores separately:
- lint;
- style;
- metadata;
- changelog;
- ownership;
- missing tests;
- generated-file checks.

Do not spend LLM review budget on deterministic checks.

AI reviewer should focus on contextual/design/behavioral issues that static rules do not cover well.

---

## 17. Review comment publication

Git provider comments are projections.

Canonical objects:
- ReviewExecution;
- ReviewFinding;
- ReviewResolution;
- ReviewVerdict.

PR comments/annotations may be regenerated or synchronized from canonical records.

Deleting/editing a GitHub comment must not rewrite audit history.

---

## 18. AI-generated change and AI review independence

Store implementation provenance:
- HUMAN;
- AI_ASSISTED;
- AI_PRIMARY.

Review policy may require:
- different execution identity;
- different Runtime/provider for stronger independence;
- human reviewer for sensitive categories.

Do not infer lower quality merely because code was AI-generated.
Use risk, Evidence and reviewer qualification.

---

## 19. Continuous reviewer calibration

Reviewer qualification can drift due:
- model update;
- provider routing;
- instruction change;
- retrieval/context change;
- ToolProfile change.

Trigger requalification on material change.

Track reviewer performance over time against:
- failure replay;
- fresh PRs;
- field regressions.

---

## 20. M0/M1 impact

### M0
Freeze:
- ReviewExecution;
- ReviewFinding;
- ReviewResolution;
- ReviewVerdict;
- ReviewContextProfile;
- ReviewerQualificationProfile/Evidence.

### M1
One qualified Codex/AI review can be used as supplemental Review.
Existing human review remains authoritative where policy requires it.

No PR-Agent/Qodo service is required.

### M2/M3
Evaluate real review tools using project-specific benchmark/failure replay.

---

## 21. New invariants

1. AI review output is structured Finding data, not automatic approval.
2. Review always binds an exact frozen subject.
3. Findings and their resolutions are separate immutable records.
4. "No comments" is not automatically a PASS verdict.
5. Independent review requires independent execution identity according to policy.
6. Reviewer qualification is scoped by language/domain/category/context.
7. Benchmark precision/recall/noise are versioned Evaluation Evidence.
8. LLM judges are themselves qualified/traceable tools.
9. Review context strategy is part of reviewer identity/qualification.
10. High-risk review keeps required human/domain independence even when AI reviewers are strong.
11. Deterministic review chores stay deterministic; LLM review focuses on contextual issues.
12. Field escapes/regressions feed the reviewer failure-replay corpus.

---

## 22. Conclusion

Round 20 turns AI code review into a governable, measurable engineering capability:

> **An AI reviewer may discover valuable defects, but its authority comes from a qualified ReviewExecution and policy-defined independence—not from the fact that it produced a PR comment.**
