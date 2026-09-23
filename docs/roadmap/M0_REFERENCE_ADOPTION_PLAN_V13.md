# M0 Reference Adoption Plan v13

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V12.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V13.md

## 1. Principle

Round 13 adds ML/data lineage schemas, not a new MLOps platform.

M0 only freezes generic types so audio/KWS/model workflows can reuse the same Run/Artifact/Evidence/Verification implementation later.

---

## 2. DatasetArtifact fixture

Create one small synthetic dataset manifest with:
- exact membership;
- partition role;
- source/provenance;
- content digest.

Prove:
- mutable folder path is not identity;
- membership change creates a new digest.

---

## 3. ExperimentDefinition fixture

Create one synthetic ExperimentDefinition bound to:
- source commit;
- DatasetArtifact;
- hyperparameters;
- seed;
- environment;
- expected output role.

Prove:
- Run Input Manifest references exact ExperimentDefinition digest;
- actual Run Receipt can record deviations.

---

## 4. MLModelArtifact fixture

Create one dummy model Artifact with:
- bytes digest;
- model format;
- ExperimentDefinition/Run lineage;
- input/output contract.

Create one byte-changing transform and Transform Receipt.

Prove:
- transformed model is a new Artifact;
- old evaluation is not automatically applicable to transform-sensitive properties.

---

## 5. CalibrationArtifact fixture

Create one threshold/config Artifact.

Prove:
- changing threshold creates new CalibrationArtifact digest;
- deployment subject digest changes even if model bytes are unchanged.

---

## 6. Dataset separation fixture

Create:
- TRAIN partition;
- QUALIFICATION partition;
- overlap check Evidence.

Prove:
- exact overlap is detectable;
- "different directory" is not accepted as independence proof.

No real ML dataset is required.

---

## 7. Existing M0 critical path remains

Still required:
- canonical schemas/digests;
- TraceLink;
- ToolProfile/RuntimeQualificationProfile;
- Incident/Reproduction/KnowledgeCandidate fixtures;
- Run Input / ExecutionSpec / Receipt;
- Build Definition / Receipt;
- Integration Subject / Eligibility;
- Session Grant;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Verification;
- Release Admission/Reconciliation;
- OTel/CDEvents;
- TestReport/BOM/Finding;
- failure injection.

---

## 8. M0 exit additions

M0 additionally verifies:
- DatasetArtifact snapshot semantics;
- ExperimentDefinition can feed a normal Run;
- MLModelArtifact transform lineage;
- CalibrationArtifact participates in Subject identity;
- DatasetSeparationEvidence is representable.

No MLflow/ClearML/W&B/Kubeflow service is required.

---

## 9. M1 target

M1 remains unchanged.

Round 13 only ensures later ML/audio/KWS workflows fit the same formal engineering contracts without parallel governance.
