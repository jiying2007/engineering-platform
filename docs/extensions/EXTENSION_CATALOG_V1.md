# Extension Catalog v1

Date: 2026-09-24
Status: **REFERENCE CATALOG — NOT CORE M0/M1**

## 1. Purpose

This catalog preserves the results of the 43 research rounds without making every researched concept part of the initial implementation contract.

Rule:

> **An extension activates only when a real embedded product milestone or repeated engineering need proves it necessary.**

Research value is retained.
Core scope is protected.

---

## 2. Tier A — Near-Core Embedded Extensions

These are likely M2/M3 extensions because they directly support embedded software development.

### Device / HIL
Relevant concepts:
- DeviceInstance;
- Device lease/fencing;
- labgrid-style resource control;
- Procedure/TestVariant/TestAttempt;
- MeasurementDataArtifact;
- metrology/calibration;
- HIL evidence.

Trigger:
- first real device/HIL pilot.

### Interface compatibility
Relevant:
- InterfaceContractRevision;
- CompatibilityPolicy/Evidence;
- ConsumerContract;
- VersionSkewPolicy;
- CompatibilityEnvelope.

Trigger:
- second independently evolving component/protocol.

### Simulation / virtual target
Relevant:
- VerificationEnvironmentClass;
- Renode/QEMU/SIL;
- environment equivalence.

Trigger:
- physical-HIL queue or iteration cost becomes material.

### Diagnostics
Relevant:
- DiagnosticServiceProfile;
- DiagnosticSnapshotArtifact;
- Action Gateway mutation receipts;
- UDS/SOVD candidates.

Trigger:
- repeat field/RMA/debug diagnostic needs.

### System safety
Relevant:
- SafetyLoss/Hazard/UCA/LossScenario/SafetyConstraint;
- safe-state transition evidence.

Trigger:
- safety-sensitive motor/charger/battery/control requirement.

### ML/audio/KWS
Relevant:
- DatasetArtifact;
- ExperimentDefinition;
- MLModelArtifact;
- CalibrationArtifact;
- Evaluation Evidence.

Trigger:
- model/data pipeline enters engineering-platform.

---

## 3. Tier B — Product Delivery Extensions

Activate only after Core + Device/HIL are proven.

### Release / OTA / Fleet
- PromotionPlan;
- SystemUpdateManifest;
- per-device promotion attempts;
- staged rollout;
- update-path evidence.

### Product security / PSIRT
- ProductSecurityCase;
- VEX/advisory;
- SecuritySupportPolicy;
- RegulatoryNotificationCase.

### Privacy / engineering data
- DataAssetProfile;
- DataProcessingActivity;
- retention/disposition;
- privacy transforms.

### FOSS / supply-chain software
- SBOM;
- license findings/policy;
- notice/source offer;
- vulnerability findings.

### Long-term support
- ProductSupportPolicy;
- cross-version qualification;
- upgrade-order constraints.

---

## 4. Tier C — Product Lifecycle Integration

These remain external-system integrations rather than Core ownership.

### PLM / PDM
- hardware design/BOM snapshots;
- EngineeringChangePackage;
- effectivity;
- PLM refs.

### MES / Manufacturing
- StationProfile;
- ProductionRecipeRevision;
- ManufacturingResultReceipt.

### ERP / QMS / Supplier
- SupplierChangeNotice;
- alternate-part qualification;
- QualityCase / CAPA;
- material compliance.

### RMA / Service
- ReturnReceipt;
- DeviceStateSnapshot;
- RepairActionReceipt.

### Certification / Compliance
- conformity cases;
- certification reports;
- OSCAL-style compliance package.

### Digital Twin
- ObservedDeviceState;
- DeviceTwinProjection.

### LIMS / external labs
- SampleInstance;
- custody;
- lab-result import.

### DPP / GS1 / EPCIS
- Digital Product Passport;
- external identifiers;
- cross-enterprise trace events.

These extensions must not pull MES/PLM/ERP semantics into Core Work/Run execution.

---

## 5. Tier D — High-Assurance / Scale Extensions

Activate only when actual risk or scale demands them.

- SPIFFE/SPIRE;
- OpenBao/Vault;
- Coder/remote workspace;
- remote execution;
- Harbor/OCI distribution;
- TUF/Uptane;
- AssuranceCaseArtifact;
- SysML/Capella;
- TLA+/CBMC/Kani/Frama-C product proofs;
- transparency-log anchoring;
- large result-history service;
- speculative multi-repo gate queue;
- large Device farm / LAVA.

---

## 6. Research-only / no current adoption

Research can remain archived without a planned implementation.

Examples:
- FRACAS/FMEA platform search where no mature reusable platform met adoption threshold;
- alternative configuration DSLs when native schemas are sufficient;
- generic analytics/knowledge platforms without proven need.

A negative result is valid architecture evidence.

---

## 7. Extension admission gate

Before promoting an Extension into active implementation, require:

1. a real consumer/task/product need;
2. current Core cannot satisfy it cleanly;
3. exact missing invariant or workflow is documented;
4. reuse/build alternatives are compared;
5. new authority boundary is explicit;
6. M0/Core does not gain unrelated dependencies;
7. one pilot and exit criterion are defined.

---

## 8. Extension implementation rule

An extension must integrate through existing Core concepts whenever possible:

~~~text
WorkItem
TaskContract
Run
Action Gateway
Artifact
Evidence
Verification
Review
Target
TraceLink
~~~

Do not introduce a new Plane just because a domain has specialized terminology.

---

## 9. Research index

Detailed exploration remains in:

- `docs/research/OPEN_SOURCE_REFERENCE_INDEX.md`
- Round-specific review documents.

Those documents are design evidence and option analysis.

They are not the M0 implementation checklist.

---

## 10. Final rule

> **Core exists to make embedded software engineering faster and more trustworthy. Extensions exist to connect that Core to additional product-lifecycle obligations only when those obligations become real.**
