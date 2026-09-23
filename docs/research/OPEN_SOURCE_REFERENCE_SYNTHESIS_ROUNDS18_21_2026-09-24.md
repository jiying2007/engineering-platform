# Open Source Reference Synthesis — Rounds 18–21

Date: 2026-09-24
Status: **Archived synthesis / implementation input**

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V21.md
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V21.md

## 1. Scope

Rounds 18–21 closed four lifecycle gaps:
- manufacturing traceability and fleet rollout;
- Knowledge freshness/revalidation;
- AI code-review quality/independence;
- empirical AI adoption/governance.

## 2. Durable semantics absorbed

The following are now considered durable implementation-independent concepts:

- ManufacturingExecutionRef
- StationProfile
- EquipmentCalibrationRecord
- ProductionRecipeRevision
- ManufacturingResultReceipt
- CohortSnapshot
- DevicePromotionAttempt
- SystemUpdateManifest
- UpdatePathEvidence
- KnowledgeItem
- KnowledgeFreshnessPolicy
- KnowledgeValidationReceipt
- ReviewExecution
- ReviewFinding
- ReviewResolution
- ReviewVerdict
- ReviewContextProfile
- ReviewerQualificationProfile
- ReviewerQualificationEvidence
- AIUsagePolicy
- RuntimeInstructionProfile
- MeasurementStudyDefinition
- optional AIAssistedChangeProvenance

## 3. Durable rules

1. Engineering-platform does not become MES/ERP; manufacturing systems remain external scheduling/material/production systems.
2. Production stations, fixtures and measurement instruments have explicit trust/calibration lifecycle.
3. Manufacturing produces immutable per-device receipts linking Release to physical serial.
4. Fleet rollout is per-device and stage-based; global status is derived.
5. Abort/rollback is asynchronous and requires reconciliation.
6. Knowledge is published only after curation and carries provenance, scope, freshness and revalidation.
7. Stale Knowledge cannot silently authorize formal actions.
8. AI reviewer output is structured Finding data, not automatic approval.
9. Reviewer qualification and independence are explicit policy inputs.
10. AI coding effectiveness is measured end-to-end and locally; usage volume is not productivity authority.
11. Runtime instructions are immutable qualification inputs but never capability enforcement.
12. AI governance changes through evidence-backed, reviewed policy revisions.

## 4. M1 dependency posture

Rounds 18–21 do not add mandatory M1 runtime services.

M1 remains centered on:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy
- Git/CI
- Codex
- native Ubuntu Worker
- short-lived workload identity/mTLS

## 5. Deferred integrations

Potential later integrations include:
- MES/ERP connector;
- OpenTAP/factory station tooling;
- BaSyx/AAS or OPC UA industrial projection;
- Mender/hawkBit/RAUC fleet/update backend;
- DataHub/OpenMetadata-like knowledge/catalog projection;
- PR-Agent/Qodo or other AI reviewer;
- DevLake/analytics projection.

No integration inherits engineering authority.

## 6. Next research frontier

The remaining high-value lifecycle gaps are:
- PLM/ECN/ECO and hardware revision/change control;
- EDA/schematic/PCB/BOM design artifact traceability;
- PSIRT/CRA/CVE remediation and support-window obligations;
- RMA/repair/failure-analysis feedback;
- multi-product / multi-SKU configuration family management.

The objective is to connect engineering-platform to product-lifecycle systems without turning it into PLM, MES or ERP.
