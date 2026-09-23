# Open Source Reference Review — Round 18: Manufacturing Traceability and Fleet Rollout

Date: 2026-09-24
Status: **Archived research / implementation input**

Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md

## 1. Scope

Round 18 reviewed:
- Eclipse BaSyx / Asset Administration Shell
- OpenTAP
- open62541 / OPC UA
- ERPNext Manufacturing concepts
- Eclipse hawkBit
- Mender
- RAUC / SWUpdate
- Uptane/TUF practices

Focus:
- manufacturing work order / station / serial traceability;
- factory-test and calibration records;
- industrial asset identity;
- staged OTA/fleet rollout;
- per-device deployment state;
- partial failure and multi-component recovery.

---

## 2. Engineering platform should not become an MES

Manufacturing systems commonly own:
- work order;
- line/station scheduling;
- operator;
- material/BOM consumption;
- serial/batch creation;
- inventory;
- quality inspection;
- production quantities.

engineering-platform should not replace those capabilities.

It should own the engineering identity/proof that manufacturing consumes:
- Release/Artifact;
- TargetRevision;
- DeviceInstance;
- provisioning/calibration/test Evidence;
- signed production recipe/procedure revision;
- engineering release authority.

MES/ERP records are integrated references, not engineering authority.

---

## 3. ManufacturingExecutionRef

Add lightweight external reference:

~~~text
ManufacturingExecutionRef
  provider/system
  work_order_id
  operation/job_card_id
  station/line_id
  lot/batch
  serial/device_id
  external status
  observed_at
~~~

This allows traceability without reproducing the entire MES data model.

For systems such as ERPNext, serial/batch/work-order/job-card identity may be attached here.

---

## 4. StationProfile

Add versioned engineering station definition:

~~~text
StationProfile
~~~

Includes:
- station identity/class;
- trust class;
- hardware fixture refs;
- programmer/debugger/instrument identities;
- installed Procedure/tool versions;
- calibration status;
- allowed Target families;
- capability grants;
- environment/location metadata;
- software image/profile digest.

A station may be:
- development HIL;
- engineering validation;
- production flash;
- factory functional test;
- calibration;
- provisioning.

Station identity is not equivalent to Worker identity.

---

## 5. Station calibration and capability

For measurement/production stations, capability is only valid while required equipment calibration is valid.

Add:

~~~text
EquipmentCalibrationRecord
~~~

Fields:
- equipment/fixture identity;
- calibration procedure;
- calibration standard/reference;
- result;
- valid_from;
- valid_until / review_at;
- issuer;
- raw certificate/evidence artifact.

Scheduler/Procedure policy can require current calibration.

A self-reported instrument capability is not sufficient for production-grade Evidence.

---

## 6. ProductionRecipeRevision

A factory/provisioning/test operation consumes immutable:

~~~text
ProductionRecipeRevision
~~~

References:
- Release/Artifact bundle;
- TargetRevision;
- flash/provision steps;
- Procedure Revision;
- configuration/calibration;
- required StationProfile;
- post-operation checks;
- rollback/rework policy;
- expected outputs/records.

It is engineering-controlled even when MES triggers execution.

Changing the recipe creates a new revision/digest.

---

## 7. ManufacturingResultReceipt

Factory execution creates:

~~~text
ManufacturingResultReceipt
~~~

Binds:
- DeviceInstance/serial;
- ManufacturingExecutionRef;
- ProductionRecipeRevision;
- StationProfile;
- operator/service identity where applicable;
- exact flashed/provisioned Artifact digests;
- configuration/calibration refs;
- test/calibration Evidence;
- timestamps;
- final disposition.

Disposition examples:
- PASS;
- FAIL;
- REWORK_REQUIRED;
- SCRAP;
- QUARANTINE.

Receipt is immutable.

---

## 8. Calibration is not just metadata

For product behavior that depends on per-unit calibration:

~~~text
DeviceCalibrationArtifact
~~~

or a per-device CalibrationArtifact instance binds:
- DeviceInstance;
- calibration Procedure;
- station/equipment calibration status;
- measured parameters;
- fitted coefficients/thresholds;
- raw measurement Evidence;
- validity/version.

Production/calibration parameters become part of the device's deployed/configuration identity where behavior depends on them.

---

## 9. Asset Administration Shell practice

Eclipse BaSyx demonstrates a useful industrial pattern:
- stable asset identity;
- submodels for different facets;
- registries/discovery;
- machine-readable digital representation.

### Absorb

Do not adopt AAS as the platform business model.

Instead:
- allow Device/Station/Fixture/Release projections into AAS when factory/industrial integration benefits;
- keep engineering-platform canonical identity and authority internal;
- use AAS/OPC UA as interoperability surfaces.

---

## 10. OPC UA practice

open62541 validates the value of:
- standardized industrial data model/access;
- portable embedded implementation;
- certification/conformance testing;
- plugin architecture.

Potential future use:
- station/equipment telemetry;
- industrial measurement access;
- MES/test-cell integration.

Do not require OPC UA for simple internal HIL benches.

---

## 11. OpenTAP practice

OpenTAP provides:
- reusable automated test sequencing;
- plugin-based instruments/actions;
- result viewing/storage;
- scalable station automation.

### Decision

For Windows-heavy factory/test stations, OpenTAP is a serious ProcedureExecution candidate.

But platform authority remains:
- Procedure Revision;
- Artifact identity;
- station trust/calibration;
- Evidence/Verification.

OpenTAP session/result files are TestReport/Raw Evidence inputs.

---

## 12. Fleet deployment is per-device state

Mender explicitly models:
- server-side Deployment;
- per-device Device Deployment;
- per-device status/sub-status;
- logs;
- static vs dynamic target groups;
- chronological deployment ordering.

### Absorb

PromotionPlan needs both:

~~~text
FleetPromotion
  global/stage state

DevicePromotionAttempt
  per DeviceInstance state
~~~

A fleet rollout cannot be represented only by one global PASS/FAIL.

---

## 13. CohortSnapshot

For staged rollout, add immutable:

~~~text
CohortSnapshot
~~~

Binds:
- selection rule;
- evaluated device set or selection snapshot semantics;
- population count;
- Target/Release;
- creation time;
- digest.

This is important because dynamic groups can change while rollout is running.

Policy must define whether:
- cohort membership is frozen;
- final phase can expand;
- new devices are excluded/included.

No hidden provider-specific semantics.

---

## 14. DevicePromotionAttempt

Each device rollout attempt records:

~~~text
device_id
promotion_plan/stage
release/config subject
state
started_at
completed_at
download/install/reboot/commit/rollback substate
provider receipt/log refs
result
rollback result
actual observed version/config
~~~

Examples of terminal state:
- SUCCESS;
- FAILURE;
- ALREADY_INSTALLED;
- NO_COMPATIBLE_ARTIFACT;
- ABORTED;
- ROLLED_BACK;
- DECOMMISSIONED;
- UNKNOWN.

Global stage verdict is derived from device attempts plus policy.

---

## 15. Abort is not instantaneous rollback

Mender's documented behavior is a valuable failure semantic:
- devices not yet aware of deployment may never start;
- devices already completed remain completed;
- devices mid-update may have to finish local transition/rollback behavior.

### Absorb

Promotion abort must distinguish:
- dispatch stop;
- active device cancellation support;
- rollback request;
- rollback confirmation.

Never model "abort" as if all devices returned atomically to old state.

---

## 16. Multi-component device update

Modern products may contain:
- Linux rootfs;
- application;
- main MCU;
- motor MCU;
- dock MCU;
- model/data/config.

Add:

~~~text
SystemUpdateManifest
~~~

which defines:
- component update order/DAG;
- per-component Release/Artifact;
- compatibility constraints;
- prepare/install/commit boundaries;
- rollback set;
- partial-failure policy.

This extends Release Bundle semantics for one physical product.

---

## 17. Coordinated commit

For multi-component update, distinguish:

~~~text
PREPARE
INSTALL
VALIDATE
COMMIT
~~~

where possible.

If component B fails after A changed:
- policy determines whether A rolls back;
- the platform records partial actual state;
- no "all failed" shorthand hides mixed device state.

Mender Orchestrator's system-level rollback concept is a useful reference, but the platform keeps a provider-neutral model.

---

## 18. Recoverability is a release property

An update path can create an unrecoverable intermediate state even when each individual component update is valid.

Therefore Release/Promotion Verification may require:

~~~text
UpdatePathEvidence
~~~

covering:
- source version/profile;
- intermediate transitions;
- reboot/authentication changes;
- rollback;
- power/network interruption;
- component order;
- recovery path.

Test "fresh install succeeds" is not enough.

---

## 19. Fleet rollout stage gates

Stage promotion may require:
- minimum success count/rate;
- maximum rollback/failure rate;
- health SLO;
- observation duration;
- no critical Incident;
- compatibility/attestation conditions;
- manual approval for final cohort.

Missing/late devices are explicitly classified.

Unknown devices cannot silently count as success.

---

## 20. Manufacturing vs fleet boundaries

Manufacturing:
- establishes physical identity;
- provisioning;
- initial software/config/calibration;
- production test.

Fleet:
- updates already-owned active devices;
- observes rollout health;
- coordinates rollback/recovery.

They share:
- DeviceInstance;
- Release/Artifact;
- configuration;
- Evidence;
- trust.

They have different authority/workflow policies.

---

## 21. M1 impact

None on runtime topology.

M0 reserves:
- StationProfile;
- EquipmentCalibrationRecord;
- ProductionRecipeRevision;
- ManufacturingResultReceipt;
- CohortSnapshot;
- DevicePromotionAttempt;
- SystemUpdateManifest.

Synthetic fixtures only.

---

## 22. M2/M3 impact

### M2 manufacturing pilot
Use one representative station:
- motor MCU production/test/calibration or final-device validation;
- exact Device/Release/Station/Procedure trace.

### M3/fleet
Integrate the actual updater:
- RAUC/Mender/hawkBit/project backend;
- staged cohort;
- per-device status;
- partial failure/rollback;
- actual-state reconciliation.

---

## 23. New invariants

1. MES work order is an external manufacturing reference, not engineering Release authority.
2. Production station trust and equipment calibration are explicit and time-bounded.
3. Factory execution produces immutable per-device receipts.
4. Per-unit calibration is part of deployed identity when behavior depends on it.
5. Fleet rollout tracks per-device attempts, not only global deployment status.
6. Dynamic cohort semantics are frozen/declared rather than inherited invisibly from provider behavior.
7. Abort is not assumed atomic.
8. Multi-component updates expose partial state and coordinated rollback semantics.
9. Release qualification includes update-path/recoverability where relevant.
10. Global rollout success is derived from explicit device-level evidence and policy.

---

## 24. Conclusion

Round 18 closes two practical embedded gaps:

> **Manufacturing traceability should connect engineering Release to each physical serial through a trusted recipe/station/result receipt.**

and:

> **Fleet rollout should be modeled as staged per-device state reconciliation, not a single deployment status.**
