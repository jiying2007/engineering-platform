# M0 Reference Adoption Plan v29

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V25.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V29.md

## 1. Principle

Rounds 26–29 add only schema/fixture coverage for:
- supplier lifecycle and CAPA;
- machine-readable compliance;
- Digital Twin read models;
- FOSS/license compliance.

No new M1 production service is required.

---

# Track A — Supplier lifecycle / alternate parts

## 2. Supplier-change fixture

Create:
- one ComponentLifecycleRecord;
- one SupplierChangeNotice;
- one affected ProductStructureSnapshot line.

Prove:
- notice alone does not mutate TargetRevision;
- Impact Analysis is created;
- historical builds stay bound to old baseline.

---

## 3. Alternate-part fixture

Create:
- original part;
- candidate substitute;
- Variant/Target-limited qualification;
- Verification Evidence;
- AlternatePartQualificationDecision.

Prove:
- purchasing substitution without qualification is rejected for formal manufacturing;
- scope/effectivity limits are enforced.

---

# Track B — Quality / CAPA

## 4. QualityCase fixture

Create one Non-Conformance-style QualityCase.

Include:
- containment;
- correction;
- corrective action;
- preventive action.

Prove:
- closure is blocked until required CAPAEffectivenessEvidence exists;
- temporary containment is not treated as root-cause removal.

---

## 5. Material compliance fixture

Create one MaterialComplianceArtifact for a component revision.

Prove:
- scope/version/validity are explicit;
- different revision/lot cannot silently reuse it;
- supplier document path is not identity.

---

# Track C — Compliance

## 6. Compliance profile fixture

Create:
- one ComplianceFrameworkArtifact;
- one ComplianceProfile;
- 3 selected controls;
- ControlImplementationStatements.

Use synthetic controls if necessary.

Prove:
- profile/tailoring is exact-version bound;
- implementation statement is not assessment PASS.

---

## 7. Assessment fixture

Create:
- ComplianceAssessmentPlan;
- Evidence reused from normal engineering Verification;
- ComplianceAssessmentResult;
- one ComplianceFinding.

Prove:
- Evidence is referenced rather than duplicated;
- Evidence becoming STALE changes assessment applicability;
- compliance status does not directly approve Release.

---

# Track D — Device observed state

## 8. Twin fixture

Create:
- desired Release/config;
- ObservedDeviceState;
- DeviceTwinProjection.

Cases:
- IN_SYNC;
- DRIFTED;
- STALE;
- PARTIAL.

Prove:
- old last-known state cannot satisfy a freshness-sensitive policy;
- device self-report has explicit trust source;
- command dispatch does not imply confirmed convergence.

---

# Track E — FOSS/license compliance

## 9. License scan fixture

Create:
- ThirdPartyComponentRecord;
- LicenseFinding;
- one SPDX/CycloneDX-style BOM ref.

Prove:
- scanner output is a Finding only;
- exact package/version/source is retained.

---

## 10. Distribution-policy fixture

Create two DistributionProfiles:
- INTERNAL;
- EMBEDDED_DEVICE_SHIPMENT.

Use one component whose obligations differ by context.

Prove:
- LicenseComplianceAssessment can differ for the same component set based on distribution profile;
- exception requires explicit LicenseExceptionDecision.

---

## 11. Shipment artifacts fixture

Create:
- NoticeArtifact;
- SourceOfferPackageArtifact.

Bind exact Release digest.

Prove:
- changing Release/component set makes old notice/source-offer package inapplicable unless regenerated/explicitly proven reusable.

---

# Track F — Existing M0 work

## 12. Retain all v25 requirements

All previous work remains:
- core canonicalization;
- Run/Build/Integration chains;
- hardware/PLM/PSIRT/RMA/Variant fixtures;
- AI governance/review;
- Device trust;
- ML lineage;
- Interface compatibility;
- ResolvedConfiguration;
- TraceLink;
- Incident/Knowledge;
- Release Admission;
- OTel/CDEvents;
- formal model-check spike;
- failure injection.

---

## 13. M0 exit additions

M0 additionally requires:
- supplier notice/impact fixture;
- alternate-part qualification fixture;
- CAPA effectiveness fixture;
- one material-compliance artifact;
- one machine-readable compliance profile/assessment;
- one Digital Twin drift/freshness fixture;
- one distribution-sensitive license assessment;
- notice/source-offer exact-release binding.

M0 does not require:
- QMS;
- supplier intelligence provider;
- OSCAL service;
- IoT twin platform;
- FOSSology/ORT/ScanCode.

---

## 14. M1 target

M1 remains the same narrow formal engineering loop.

The added fixtures guarantee later hardware supply-chain, compliance, field-state and licensing integrations fit the same authority model.
