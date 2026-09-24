# Open Source Reference Synthesis — Rounds 22–30

Date: 2026-09-24
Status: **Archived synthesis / implementation input**

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V29.md
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V29.md

## 1. Scope

Rounds 22–30 extended the platform from software-centric engineering into product-lifecycle engineering:
- PLM/ECO/hardware design and BOM baselines;
- PSIRT/VEX/CRA response;
- RMA/repair/failure analysis;
- product family/SKU/variant resolution;
- supplier lifecycle/alternate parts/CAPA/material compliance;
- machine-readable compliance;
- Digital Twin / observed state;
- FOSS/license compliance;
- reliability-tooling negative-result assessment.

## 2. Durable semantics absorbed

Durable, implementation-independent concepts now include:

- ExternalPLMRef
- HardwareDesignArtifact
- DesignExportReceipt
- ProductStructureSnapshot
- EngineeringChangePackage
- EffectivityRule
- ProductSecurityCase
- VulnerabilityApplicabilityAssessment
- VEXArtifact
- SecurityAdvisoryArtifact
- SecuritySupportPolicy
- SecurityRemediationPlan
- RegulatoryNotificationCase
- ExternalServiceCaseRef
- ReturnReceipt
- DeviceStateSnapshot
- FailureAnalysisRecord
- RepairActionReceipt
- FeatureModelArtifact
- ProductVariantDefinition
- VariantResolutionReceipt
- VariantConstraintEvidence
- VariantCoveragePlan
- VariantEquivalenceDecision
- SupportedVariantSet
- ComponentLifecycleRecord
- SupplierChangeNotice
- AlternatePartQualificationDecision
- QualityCase
- CAPAPlan
- CAPAEffectivenessEvidence
- MaterialComplianceArtifact
- MaterialLotStatus
- ComplianceFrameworkArtifact
- ComplianceProfile
- ControlImplementationStatement
- ComplianceAssessmentPlan
- ComplianceAssessmentResult
- CompliancePackageArtifact
- ObservedDeviceState
- DeviceTwinProjection
- ThirdPartyComponentRecord
- LicenseFinding
- LicensePolicy
- DistributionProfile
- LicenseComplianceAssessment
- NoticeArtifact
- SourceOfferPackageArtifact
- LicenseExceptionDecision

## 3. Durable architecture rules

1. PLM/PDM/MES/ERP/QMS/CRM remain external lifecycle systems.
2. engineering-platform owns exact engineering identities, evidence, decisions and effectivity.
3. EDA source, fabrication outputs and BOM are distinct immutable artifacts linked by export receipts.
4. Engineering Change approval and production effectivity are separate.
5. As-designed, as-built, field-observed, returned and repaired states remain distinct.
6. Product-security applicability is exact-product/release/configuration scoped; scanner matches are only findings.
7. Regulatory reporting is jurisdiction-specific and separately receipted.
8. SKU/commercial variant, resolved engineering variant, TargetRevision and DeviceInstance are distinct identities.
9. Supplier PCN/EOL/change notices trigger impact analysis; purchasing substitution is not engineering equivalence.
10. CAPA closes only after effectiveness evidence.
11. Compliance is a structured projection over engineering facts, not a second source of truth.
12. Digital Twin is a freshness/trust-bound read model.
13. FOSS/license obligations are distribution-context specific and exact-release bound.
14. Reliability/FMEA/FRACAS research did not justify a new platform subsystem.

## 4. Current implementation posture

M1 hard dependencies remain intentionally small:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy for tests
- Git/CI
- Codex
- native Ubuntu Worker
- short-lived workload identity/mTLS

No PLM, MES, QMS, Digital Twin, PSIRT, license-scanning or reliability platform is required by M1.

## 5. Integration posture

Potential later integrations:
- KiCad/KiBot and PLM/ERP BOM systems;
- CSAF/VEX/CVE/CRA reporting connectors;
- RMA/service systems;
- feature/variant resolution;
- supplier lifecycle feeds;
- OSCAL export/import;
- Ditto/ThingsBoard-like observed-state projection;
- FOSSology/ORT/ScanCode;
- product-specific reliability tools.

All remain behind typed refs, Artifacts, Evidence, Receipts and Decisions.

## 6. Negative-result discipline

Round 30 established an explicit research rule:

> If targeted research does not reveal a missing invariant, useful standard, mature reusable mechanism or failure mode, the canonical architecture should not expand.

This rule remains in force for future research.

## 7. Next research frontier

The remaining high-value lifecycle gaps are:
- product certification/regulatory evidence lifecycle;
- battery/charging/thermal safety qualification;
- privacy and device-data lifecycle;
- supplier PCN impact-analysis automation practices;
- field telemetry/anomaly/predictive-maintenance feedback;
- long-term backward-compatibility and support matrices across product generations.

Research can proceed in parallel with M0 implementation.
