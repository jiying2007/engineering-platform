# Open Source Reference Synthesis — Rounds 11–17

Date: 2026-09-24
Status: **Archived synthesis / implementation input**

Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V17.md

## 1. Purpose

Rounds 11–17 moved the research beyond execution infrastructure into:
- operational metrics and progressive delivery;
- AI Runtime / MCP governance;
- incident/regression/knowledge closure;
- ML dataset/model/calibration lineage;
- device provisioning and attestation;
- formal methods;
- interface compatibility;
- resolved configuration.

The purpose of this synthesis is to distinguish durable platform semantics from milestone-specific integrations.

---

## 2. Durable domain additions

The following concepts are considered durable and implementation-independent:

- MetricDefinition
- RuntimeConfigurationSnapshot
- PromotionPlan / PromotionStage
- ToolProfile
- RuntimeQualificationProfile
- Incident
- InvestigationHypothesis
- ReproductionCase
- RegressionRange / Bisection records
- RootCauseDecision
- FixVerification
- PostmortemArtifact
- KnowledgeCandidate
- DatasetArtifact
- ExperimentDefinition
- MLModelArtifact
- CalibrationArtifact
- DatasetSeparationEvidence
- DeviceProvisioningReceipt
- DeviceTrustProfile
- BootTrustEvidence
- DeviceAttestationEvidence
- FormalSpecificationArtifact
- FormalVerificationEvidence
- InterfaceCompatibilityPolicy
- InterfaceCompatibilityEvidence
- ConsumerContractArtifact / Evidence
- ConfigurationSchemaArtifact
- ConstraintSetArtifact
- ResolvedConfigurationArtifact
- ConfigurationValidationEvidence

These concepts are not tied to any one open-source implementation.

---

## 3. Durable architecture rules

### 3.1 Metrics are derived

Metrics and dashboards consume authoritative data.
They do not mutate Work, Verification or Release.

### 3.2 Runtime/tool governance is exact-profile based

Formal Runs bind exact ToolProfile and RuntimeQualification scope.
Tool discovery/catalog listing does not grant authority.

### 3.3 Operational feedback returns through typed engineering objects

Finding/Alert does not directly become Root Cause or Knowledge.

The closed loop is:

~~~text
Finding
 -> Incident
 -> Hypothesis
 -> Reproduction
 -> Root Cause Decision
 -> Fix
 -> Fix Verification
 -> Regression Test
 -> Postmortem
 -> Corrective Work
 -> Curated Knowledge
~~~

### 3.4 ML is not a parallel governance plane

Training is a Run.
Data/model/calibration are Artifacts.
Evaluation is Evidence.
Qualification is Verification.
Shipping is Release.

### 3.5 Device trust is layered

Keep distinct:
- Target identity;
- DeviceInstance;
- provisioning;
- boot trust;
- remote attestation;
- operational health;
- fleet ownership.

### 3.6 Formal methods are scoped Evidence

Formal proof/model-check outputs include assumptions/bounds and participate in Verification.
They never self-authorize Release.

### 3.7 Interface compatibility is multidimensional

Schema compilation is insufficient.
Compatibility may differ across source, wire, behavioral, persisted-data, config and security dimensions.

### 3.8 Execution binds resolved configuration

Human-authored templates/config files are inputs.
The tested/released subject binds the deterministic resolved canonical configuration digest.

---

## 4. Current concrete M1 stack remains intentionally small

M1 still requires only:
- PostgreSQL;
- Temporal;
- OPA;
- S3/MinIO-compatible ArtifactStore;
- OpenTelemetry;
- Toxiproxy for tests;
- Git/CI;
- Codex;
- native Ubuntu Worker;
- available enterprise PKI or lightweight short-lived mTLS.

No new service from Rounds 11–17 is mandatory.

---

## 5. Deferred integration candidates

### M2 likely
- labgrid;
- pytest/pytest-embedded/OpenHTF;
- production signing backend;
- selected simulation/emulation;
- selected BOM/security scanner.

### ML/audio workstreams
- MLflow/ClearML/W&B/DVC interoperability if it reduces workflow cost;
- no transfer of authority.

### Production/device
- MCUboot/wolfBoot;
- RAUC/Mender/hawkBit;
- Keylime/TPM/PSA;
- FDO;
- target-specific secure boot/attestation.

### Scale
- ToolHive/Docker MCP Gateway;
- SPIRE;
- OpenBao;
- Harbor;
- GUAC;
- LAVA;
- remote execution.

### A3/A4
- AssuranceCaseArtifact;
- SysML/Capella;
- TLA+/CBMC/Kani/Frama-C/VeriFast according to property/language.

---

## 6. Research quality rule

A new external project should modify the canonical implementation profile only if it:

1. reveals a missing invariant;
2. supplies a durable interoperability standard;
3. meaningfully reduces bespoke infrastructure;
4. exposes a failure/recovery mode not represented;
5. provides a proven object/state model applicable to embedded engineering.

A feature-rich tool is not, by itself, a reason to expand platform topology.

---

## 7. Decision

Rounds 11–17 are considered absorbed into the platform semantics.

Future research should now focus on uncovered lifecycle boundaries:
- manufacturing/MES/calibration traceability;
- fleet rollout partial-failure semantics;
- Knowledge freshness/expiry/revalidation;
- AI code-review evaluation and independence;
- enterprise AI coding governance evidence.

The project can proceed with M0 implementation in parallel.
