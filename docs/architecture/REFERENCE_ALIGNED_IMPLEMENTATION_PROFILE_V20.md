# Reference-Aligned Implementation Profile v20

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md

Research basis:
- Open Source Reference Review Rounds 1–20
- Open Source Reference Synthesis Optimization
- Open Source Reference Synthesis Rounds 11–17
- Architecture v1.2

## 1. Goal

Profile v20 absorbs three lifecycle areas that were previously under-specified:

1. manufacturing/factory traceability and fleet rollout semantics;
2. Knowledge freshness/revalidation;
3. AI code-review quality and reviewer independence.

No new mandatory M1 runtime service is introduced.

---

# Part A — Manufacturing and Fleet

## 2. MES remains external

engineering-platform does not become an MES/ERP.

MES may own:
- work orders;
- line/station scheduling;
- inventory/material;
- job cards;
- production quantities;
- serial/batch administration.

engineering-platform owns:
- Release/Artifact identity;
- TargetRevision;
- DeviceInstance;
- Provisioning/Calibration/Test Evidence;
- ProductionRecipeRevision;
- engineering Release authority.

Use:

~~~text
ManufacturingExecutionRef
~~~

for external MES linkage.

---

## 3. StationProfile

Add versioned:

~~~text
StationProfile
~~~

Defines:
- station identity/class;
- trust class;
- fixture/instrument/programmer refs;
- software/tool profile;
- calibration requirements;
- allowed Targets;
- capability grants;
- environment/site.

Station classes may include:
- HIL;
- validation;
- factory test;
- calibration;
- provisioning;
- production flash.

Station identity is separate from Worker identity.

---

## 4. EquipmentCalibrationRecord

Measurement capability is time-bound.

Add:

~~~text
EquipmentCalibrationRecord
~~~

Binds:
- equipment/fixture;
- calibration Procedure;
- standard/reference;
- result;
- issuer;
- valid_from;
- valid_until/review_at;
- raw certificate/Evidence.

Production-grade measurement Evidence can require a valid calibration record.

---

## 5. ProductionRecipeRevision

Immutable factory engineering recipe:

~~~text
ProductionRecipeRevision
~~~

References:
- Release/Artifact bundle;
- TargetRevision;
- flash/provision actions;
- ProcedureRevision;
- calibration/config;
- required StationProfile;
- post-operation checks;
- rework/rollback rules;
- output/receipt contract.

MES may schedule the job, but the engineering recipe is controlled/versioned by engineering-platform.

---

## 6. ManufacturingResultReceipt

Per-device immutable receipt:

~~~text
ManufacturingResultReceipt
~~~

Binds:
- DeviceInstance;
- external work order/job/station refs;
- ProductionRecipeRevision;
- StationProfile;
- exact flashed/provisioned Artifacts;
- configuration/calibration;
- Evidence;
- timestamps;
- disposition.

Disposition:
- PASS;
- FAIL;
- REWORK_REQUIRED;
- SCRAP;
- QUARANTINE.

---

## 7. Per-unit calibration

Per-unit behavior-affecting calibration is an Artifact bound to DeviceInstance.

It records:
- Procedure;
- station/equipment calibration;
- measured values;
- fitted parameters;
- raw Evidence;
- configuration/result digest.

Changing per-unit calibration changes that device's effective deployed subject where behavior depends on it.

---

## 8. Industrial interoperability

Potential integrations:
- Eclipse BaSyx / Asset Administration Shell;
- OPC UA / open62541;
- OpenTAP.

Strategy:
- AAS/OPC UA are interoperability projections/interfaces;
- OpenTAP may be a ProcedureExecution mechanism on factory stations;
- none becomes engineering authority.

---

## 9. Fleet rollout is per-device state

Add:

~~~text
CohortSnapshot
DevicePromotionAttempt
~~~

CohortSnapshot freezes or explicitly describes cohort membership semantics.

DevicePromotionAttempt records:
- DeviceInstance;
- PromotionPlan/Stage;
- exact Release/config subject;
- download/install/reboot/commit/rollback states;
- provider receipt/logs;
- actual observed version/config;
- result.

Global rollout status is a projection over device attempts + policy.

---

## 10. Promotion abort semantics

Abort is not assumed atomic.

Distinguish:
- stop new dispatch;
- cancel active update where supported;
- request rollback;
- rollback observed/confirmed.

Devices may be in mixed states after an abort.

UNKNOWN remains explicit until reconciled.

---

## 11. SystemUpdateManifest

For multi-component products:

~~~text
SystemUpdateManifest
~~~

Defines:
- component DAG/order;
- per-component Artifacts;
- compatibility;
- PREPARE/INSTALL/VALIDATE/COMMIT boundaries;
- rollback set;
- partial-failure policy.

Applicable components may include:
- Linux;
- application;
- main MCU;
- motor MCU;
- dock MCU;
- ML model;
- calibration/config.

---

## 12. UpdatePathEvidence

Release qualification may require:

~~~text
UpdatePathEvidence
~~~

covering:
- source version/config;
- transitions;
- reboot/authentication;
- component ordering;
- power/network interruption;
- rollback;
- recovery.

"Fresh install succeeds" does not prove update-path recoverability.

---

# Part B — Knowledge Lifecycle

## 13. KnowledgeItem

Published Engineering Knowledge becomes:

~~~text
KnowledgeItem
~~~

Types:
- KNOWN_ISSUE;
- RECOVERY_RUNBOOK;
- DESIGN_RULE;
- COMPATIBILITY_RULE;
- TEST_METHOD;
- DIAGNOSTIC_PATTERN.

Fields:
- version;
- content Artifact;
- scope;
- assumptions;
- provenance;
- Evidence/Decision refs;
- owner/steward;
- status;
- review_at;
- expires_at;
- validation policy;
- supersession refs.

---

## 14. Knowledge lifecycle

~~~text
CANDIDATE
 -> REVIEWED
 -> PUBLISHED
 -> STALE
 -> REVALIDATING
 -> PUBLISHED
    | INVALID
    | SUPERSEDED
    | RETIRED
~~~

Do not delete history to represent lifecycle changes.

---

## 15. KnowledgeFreshnessPolicy

Versioned policy defines:
- review interval;
- expiry;
- dependency triggers;
- required revalidation;
- stale-use behavior;
- owner/escalation.

Freshness triggers include:
- time;
- Target/Interface change;
- dependency/toolchain change;
- failed Procedure;
- Incident;
- revoked Artifact/Release;
- changed assumptions.

---

## 16. KnowledgeValidationReceipt

Revalidation creates immutable:

~~~text
KnowledgeValidationReceipt
~~~

Binds:
- KnowledgeItem version;
- current dependencies;
- Procedure;
- Target/environment;
- validator/issuer;
- Evidence;
- result;
- time.

Result:
- VALID;
- STALE;
- INVALID;
- INCONCLUSIVE;
- TOOL_ERROR.

---

## 17. Knowledge retrieval policy

Context Resolver considers:
- exact scope match;
- current status;
- freshness;
- trust level;
- provenance;
- Target/Interface compatibility.

Recency alone is not ranking authority.

STALE Knowledge may be advisory context but cannot silently authorize privileged action.

---

## 18. Knowledge publication boundary

Keep:

~~~text
Incident/Postmortem
 -> KnowledgeCandidate
 -> curation/review
 -> KnowledgeItem
~~~

AI may draft.

AI cannot directly publish authoritative Engineering Knowledge.

Historical Formal Runs retain exact KnowledgeItem versions/digests used.

---

# Part C — AI Code Review

## 19. ReviewExecution

Add immutable:

~~~text
ReviewExecution
~~~

Binds:
- frozen ReviewSubject;
- reviewer execution identity;
- reviewer kind;
- Runtime/model/profile;
- ToolProfile;
- context profile;
- instructions;
- qualification evidence;
- independence level;
- output Artifact.

Reviewer kinds:
- HUMAN;
- AI_RUNTIME;
- STATIC_TOOL;
- HYBRID.

---

## 20. ReviewFinding

Canonical actionable review issue:

~~~text
ReviewFinding
~~~

Includes:
- exact source location/symbol;
- category;
- severity;
- statement/rationale;
- evidence/context;
- confidence;
- fingerprint/dedup;
- source ReviewExecution.

GitHub/GitLab comments are projections of this object.

---

## 21. ReviewResolution

Finding resolution is separate:

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
- fix change;
- Evidence;
- Risk/Decision;
- resolver;
- rationale.

Reviewer output cannot self-resolve without evidence/workflow.

---

## 22. ReviewVerdict

Derived from:
- required reviewer classes;
- unresolved findings;
- severity;
- qualification;
- independence;
- policy/human decisions.

Possible:
- PASS;
- FAIL;
- CONDITIONAL;
- INCOMPLETE.

"No AI comments" is not automatically PASS.

---

## 23. Review independence

Reuse and clarify:

~~~text
R0 implementation self-check
R1 new session, same provider/model family
R2 independent Run/context
R3 different qualified runtime/provider/reviewer system
R4 human domain specialist/designated authority
~~~

Same implementation execution cannot satisfy independent review by simply re-prompting itself.

Assurance Profile sets minimum level.

---

## 24. ReviewerQualificationProfile

Add:

~~~text
ReviewerQualificationProfile
ReviewerQualificationEvidence
~~~

Scope:
- language;
- domain;
- issue category;
- context strategy;
- benchmark corpus;
- confidentiality class;
- model/provider/version.

Metrics:
- precision;
- recall;
- severity-weighted recall;
- critical miss rate;
- false-positive/noise rate;
- line precision;
- duplicate rate;
- actionable acceptance/fix rate;
- latency/cost.

---

## 25. Benchmark strategy

Use complementary qualification data:
- frozen curated benchmark;
- fresh/rolling benchmark;
- project-specific failure replay.

Public references include:
- Code Review Bench;
- AACR-Bench;
- SWE-PRBench;
- CodeReviewQA.

No single public benchmark grants universal reviewer authority.

---

## 26. ReviewContextProfile

Freeze context strategy:

~~~text
DIFF_ONLY
DIFF_PLUS_FILES
REPOSITORY_MAP
TARGETED_SYMBOL_CONTEXT
REQUIREMENT_PLUS_DIFF
FULL_REPO_RETRIEVAL
~~~

"More context" is not assumed to be better.

The exact strategy is part of qualification.

---

## 27. Human review boundary

Possible policy:
- low risk: static + qualified AI may suffice;
- medium risk: AI + human;
- A3/A4/high-risk: AI supplemental, independent human/domain review required.

AI quality data may reduce toil.
It does not silently remove required Human Authority.

---

## 28. Deterministic review stays deterministic

Use static/deterministic systems for:
- lint;
- style;
- metadata;
- ownership;
- changelog;
- schema checks;
- generated file checks;
- known rule violations.

Use LLM reviewers for contextual/design/behavioral reasoning.

---

# Part D — Milestone Strategy

## 29. M0/M1

M0 freezes/reserves:
- StationProfile;
- EquipmentCalibrationRecord;
- ProductionRecipeRevision;
- ManufacturingResultReceipt;
- CohortSnapshot;
- DevicePromotionAttempt;
- SystemUpdateManifest;
- KnowledgeItem;
- KnowledgeFreshnessPolicy;
- KnowledgeValidationReceipt;
- ReviewExecution;
- ReviewFinding;
- ReviewResolution;
- ReviewVerdict;
- ReviewContextProfile;
- ReviewerQualificationProfile/Evidence.

M1 does not deploy:
- MES;
- OpenTAP;
- BaSyx;
- OPC UA server;
- Mender/hawkBit fleet;
- metadata/knowledge platform;
- PR-Agent/Qodo.

Synthetic fixtures only.

---

## 30. M2/M3

### M2
- real HIL/factory-like station pilot;
- station/equipment calibration;
- Device manufacturing receipt;
- real knowledge revalidation from Incident/Procedure;
- AI reviewer qualification on project replay corpus.

### M3
- fleet staged rollout;
- multi-component SystemUpdateManifest;
- per-device rollback/reconciliation;
- optional MCP gateway;
- optional production knowledge/search UI.

---

## 31. Final rules added by v20

1. Engineering Release identity reaches each manufactured serial through immutable recipe/station/result receipts.
2. Production measurement authority depends on station/equipment calibration validity.
3. Fleet rollout is per-device and stage-derived, not one global status.
4. Abort/rollback is asynchronous and reconciled, not atomic.
5. Multi-component update exposes partial state and recovery semantics.
6. Durable Knowledge has provenance, scope, owner, freshness and validation lifecycle.
7. Stale Knowledge cannot silently authorize formal/privileged actions.
8. AI Review creates Findings; it does not automatically approve.
9. Review Findings and Resolutions are separate immutable records.
10. Reviewer independence and qualification are explicit policy inputs.
11. Reviewer benchmark/context profiles are versioned and continuously recalibrated.
12. High-risk human/domain review remains independent of AI reviewer strength.

Architecture v1.2 remains canonical.
Profile v20 is the current implementation companion.
