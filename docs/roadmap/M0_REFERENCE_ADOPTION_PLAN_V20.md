# M0 Reference Adoption Plan v20

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V17.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V20.md

## 1. Principle

Rounds 18–20 add schema/fixture coverage for:
- manufacturing/fleet;
- knowledge freshness;
- AI review governance.

They do not add new M1 services.

---

# Track A — Manufacturing / Station

## 2. Station fixture

Create one synthetic:

~~~text
StationProfile
~~~

for a representative embedded station.

Include:
- station identity;
- trust class;
- programmer/instrument refs;
- software profile;
- allowed Target;
- capability grants;
- required calibration.

Prove:
- station identity differs from Worker identity;
- self-reported station privilege cannot grant production capability.

---

## 3. Equipment calibration fixture

Create one:

~~~text
EquipmentCalibrationRecord
~~~

with:
- equipment identity;
- Procedure;
- issuer;
- valid_from;
- valid_until;
- Evidence.

Test:
- valid calibration permits production-grade measurement;
- expired calibration blocks it;
- historical Evidence remains immutable.

---

## 4. Production recipe / result fixture

Create:

~~~text
ProductionRecipeRevision
ManufacturingExecutionRef
ManufacturingResultReceipt
~~~

Bind:
- one DeviceInstance;
- exact Release/Artifact;
- exact StationProfile;
- Procedure;
- calibration;
- external work-order reference.

Prove:
- MES reference cannot replace Release authority;
- recipe change changes digest;
- per-device receipt is immutable.

---

# Track B — Fleet / OTA

## 5. Cohort fixture

Create:
- one static/frozen cohort;
- one dynamic-rule cohort example.

Freeze:

~~~text
CohortSnapshot
~~~

Prove the platform can distinguish:
- frozen membership;
- provider-dynamic membership semantics;
- newly joined devices.

No real fleet backend required.

---

## 6. DevicePromotionAttempt fixture

Simulate 5 devices:
- SUCCESS;
- FAILURE;
- ROLLED_BACK;
- ALREADY_INSTALLED;
- UNKNOWN.

Prove:
- global stage verdict derives from explicit policy;
- UNKNOWN cannot count as success;
- one global deployment status cannot erase device state.

---

## 7. Abort fixture

Simulate:
- one device not yet dispatched;
- one installing;
- one completed.

Issue abort.

Prove:
- new dispatch stops;
- completed device remains completed;
- installing device may need rollback;
- stage remains mixed until reconciliation.

---

## 8. Multi-component update fixture

Create:

~~~text
SystemUpdateManifest
~~~

for:
- Linux;
- main MCU;
- motor MCU.

Simulate:
- Linux prepared;
- main MCU installed;
- motor MCU fails.

Prove:
- partial actual state is retained;
- rollback policy identifies which components revert;
- Release cannot become CONFIRMED before system state reconciles.

---

## 9. Update-path fixture

Create one:

~~~text
UpdatePathEvidence
~~~

covering:
- old release;
- new release;
- reboot;
- rollback;
- interruption.

Prove:
- fresh-install Evidence does not satisfy update-path requirement automatically.

---

# Track C — Knowledge Freshness

## 10. Knowledge lifecycle fixture

Create:
- one Known Issue;
- one Recovery Runbook.

States:
- PUBLISHED;
- STALE;
- REVALIDATING;
- PUBLISHED/INVALID.

Prove:
- history is preserved;
- stale item remains discoverable but marked;
- stale Runbook cannot authorize privileged autonomous execution.

---

## 11. Freshness-policy fixture

Create:

~~~text
KnowledgeFreshnessPolicy
~~~

Example triggers:
- 180-day review;
- referenced InterfaceContractRevision change;
- Procedure failure.

Prove:
- time trigger and dependency trigger both cause revalidation requirement.

---

## 12. Knowledge revalidation fixture

Run a synthetic Procedure that creates:

~~~text
KnowledgeValidationReceipt
~~~

Prove:
- exact KnowledgeItem version is bound;
- current dependency digests are recorded;
- receipt updates current validity projection without rewriting old knowledge.

---

# Track D — AI Code Review

## 13. Review object fixture

Create:

~~~text
ReviewExecution
ReviewFinding
ReviewResolution
ReviewVerdict
~~~

Use a synthetic PR with:
- one real defect;
- one false-positive AI finding;
- one unresolved medium finding.

Prove:
- ReviewExecution output is not approval;
- finding and resolution are separate;
- no-comments result does not imply PASS.

---

## 14. Reviewer independence fixture

Simulate:
- R0 same implementation session self-check;
- R1 same provider/model new session;
- R2 independent Run/context;
- R4 human reviewer.

Prove:
- Assurance policy can require R2 or R4;
- re-prompting the implementation Run cannot satisfy independent review.

---

## 15. Reviewer qualification fixture

Freeze:

~~~text
ReviewerQualificationProfile
~~~

Measure on a small internal replay corpus:
- precision;
- recall;
- critical miss;
- noise;
- location precision.

Create ReviewerQualificationEvidence.

No production benchmark service required.

---

## 16. Review context fixture

Compare:
- DIFF_ONLY;
- TARGETED_SYMBOL_CONTEXT.

Prove:
- context profile is part of ReviewExecution identity;
- model/profile cannot silently switch context strategy while reusing qualification.

---

# Track E — Existing M0 Critical Path

## 17. Retain all v17 critical work

Still required:
- canonical serialization/digests;
- core domain schemas;
- TraceLink;
- RunInput / ExecutionSpec / RunReceipt;
- BuildDefinition / BuildReceipt;
- Integration Subject / Eligibility;
- Interface compatibility;
- ResolvedConfiguration;
- ToolProfile / Runtime qualification;
- Session Grant/Gateway;
- PostgreSQL / Temporal / OPA;
- Artifact / Evidence / Attestation / Verification;
- Release Admission / reconciliation;
- ML lineage fixtures;
- Device trust/provisioning fixtures;
- Incident/reproduction/knowledge candidate;
- OTel/CDEvents;
- failure injection;
- targeted formal model-check spike.

---

## 18. M0 exit additions

M0 additionally requires:
- station/calibration/production receipt schema fixtures pass;
- per-device fleet attempt states are representable;
- multi-component partial-state rollback fixture passes;
- Knowledge stale/revalidation lifecycle works;
- stale Knowledge cannot silently authorize privileged action;
- Review Finding/Resolution/Verdict are separated;
- reviewer independence policy fixture passes;
- one small ReviewerQualificationEvidence exists.

M0 does not require:
- MES;
- ERPNext;
- BaSyx;
- OPC UA;
- OpenTAP;
- Mender/hawkBit;
- DataHub/OpenMetadata;
- PR-Agent/Qodo.

---

## 19. M1 target

M1 remains the same narrow trusted engineering loop.

The additional fixtures guarantee that manufacturing, fleet, knowledge and AI review can attach later without redesigning the authority model.
