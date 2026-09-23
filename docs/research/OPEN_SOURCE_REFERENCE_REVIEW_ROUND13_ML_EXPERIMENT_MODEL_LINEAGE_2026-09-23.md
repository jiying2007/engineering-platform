# Open Source Reference Review — Round 13: Experiment, Dataset, Model and Calibration Lineage

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V12.md

## 1. Scope

Round 13 reviewed:
- MLflow
- DVC/CML ecosystem concepts
- Google ML Metadata
- Kubeflow Pipelines
- ClearML
- Metaflow
- Weights & Biases
- Model Card Toolkit

Focus:
- experiment lineage;
- dataset identity/versioning;
- training/evaluation reproducibility;
- model registry/promotion;
- calibration/threshold artifacts;
- mapping ML workflows into the existing engineering authority model.

---

## 2. Do not create a parallel MLOps authority

ML platforms commonly define:
- experiment;
- run;
- dataset;
- model;
- registry;
- deployment.

engineering-platform already owns:
- Task/Run/Attempt;
- Artifact;
- Evidence;
- Verification;
- Release;
- TraceLink.

### Decision

Map ML concepts onto the existing domain.

~~~text
Training / optimization experiment
 -> Formal Task + Run

Dataset
 -> DatasetArtifact

Trained model
 -> MLModelArtifact

Calibration / threshold / quantization parameters
 -> CalibrationArtifact / Config Artifact

Metrics / evaluation
 -> Evaluation Evidence

Model promotion
 -> Release/Artifact promotion
~~~

MLflow/ClearML/W&B may be tools/views but are not engineering authority.

---

## 3. DatasetArtifact

Add Artifact specialization:

~~~text
DatasetArtifact
~~~

Minimum metadata:
- content/snapshot digest;
- source/provenance;
- generation/import procedure;
- license/usage constraints;
- confidentiality/privacy class;
- schema/feature definition;
- sample/object manifest;
- split/partition metadata;
- preprocessing/augmentation lineage;
- statistics/quality report refs.

Mutable directory/object-store path is a locator, not dataset identity.

---

## 4. Dataset partition identity

For ML/audio/KWS evaluation, distinguish immutable partitions:

~~~text
TRAIN
VALIDATION
CALIBRATION
TEST
QUALIFICATION
FAILURE_REPLAY
HARD_NEGATIVE
CUSTOM
~~~

A partition is identified by exact membership/snapshot digest.

Rules:
- changing membership creates a new partition digest;
- test/qualification set mutation invalidates comparability where relevant;
- training and authoritative qualification partitions must be distinguishable.

---

## 5. Data leakage / overlap Evidence

For high-value model qualification, generate:

~~~text
DatasetSeparationEvidence
~~~

Examples:
- exact sample overlap check;
- source/speaker/device/session overlap rules;
- generator-family overlap;
- synthetic-family independence;
- temporal/site split;
- contamination scan.

A "different folder" does not prove independent evaluation data.

This is especially important for audio/KWS qualification.

---

## 6. ExperimentDefinition

Training/parameter-search intent may use immutable:

~~~text
ExperimentDefinition
~~~

Includes:
- source code/commit;
- DatasetArtifact refs;
- training/evaluation config;
- preprocessing/augmentation config;
- model architecture definition;
- hyperparameters/search space;
- random seeds where applicable;
- framework/toolchain/environment;
- resource profile;
- expected output roles;
- evaluation plan.

It is referenced by Run Input Manifest.

---

## 7. Experiment execution uses existing Run

Do not add a second Run system.

~~~text
ExperimentDefinition
 -> Run Input Manifest
 -> ExecutionSpec
 -> Run / Attempt
 -> Run Receipt
~~~

Run Receipt records:
- resolved config;
- actual datasets;
- seeds;
- environment;
- metrics;
- produced model/checkpoint artifacts;
- logs;
- nondeterminism/deviation metadata.

---

## 8. MLModelArtifact

Add explicit Artifact specialization:

~~~text
MLModelArtifact
~~~

Metadata:
- model family/architecture;
- framework/format;
- input/output contract;
- training Run/ExperimentDefinition refs;
- dataset refs;
- preprocessing refs;
- model bytes digest;
- parameter/count/size metadata where useful;
- quantization/export lineage;
- compatibility/target constraints.

Aliases such as:
- latest;
- production;
- champion

are locators/labels, not identity.

---

## 9. Model transformation lineage

A deployed model may pass through:

~~~text
training checkpoint
 -> export
 -> quantization
 -> graph optimization
 -> compiler conversion
 -> packaging
 -> deployed model
~~~

Each byte-changing step creates a new Artifact and Transform Receipt.

Evaluation reuse across transforms is policy-driven.

Example:
- training loss may remain contextual;
- quantized-device accuracy/latency must be measured on the transformed model.

---

## 10. CalibrationArtifact

Embedded ML often depends on non-model bytes/settings:
- quantization calibration;
- normalization statistics;
- thresholds;
- VAD/KWS thresholds;
- frontend parameters;
- AGC/NS tuning;
- feature extractor constants.

Add:

~~~text
CalibrationArtifact
~~~

or a typed Configuration Artifact.

It is immutable/content-addressed and linked to:
- model;
- Target;
- Procedure;
- Evaluation Evidence.

Changing threshold/calibration changes the deployed Subject even if model bytes do not change.

---

## 11. Model Release Bundle

A model product release may need:

~~~text
ML Release Bundle
  MLModelArtifact
  CalibrationArtifact
  preprocessing/frontend Artifact
  label/tokenizer/vocabulary Artifact
  runtime/library version
  Target/compatibility
~~~

Release authority binds the exact bundle.

"Model v1.2" alone is insufficient.

---

## 12. Evaluation Evidence

Metrics are not self-authoritative.

Evaluation binds:
- exact model Artifact;
- exact DatasetArtifact/partition;
- Procedure/evaluator version;
- environment/Target;
- metric definition/version;
- raw predictions/results where retention permits;
- statistical method.

Output becomes Evidence.

Examples:
- FAR/FRR;
- accuracy;
- latency;
- memory;
- model size;
- robustness;
- per-keyword metrics;
- acoustic-condition slices.

---

## 13. MetricDefinition reuse

Use the existing MetricDefinition concept for ML metrics.

A reported value must identify:
- metric name/version;
- aggregation;
- threshold;
- population/slice;
- unit;
- confidence/statistical semantics.

This prevents metric-name drift such as two different definitions both called "FAR".

---

## 14. Model qualification vs experiment ranking

Experiment dashboards may rank candidates.

That ranking is not Release authority.

Formal selection:

~~~text
candidate models
 -> Evaluation Evidence
 -> Verification
 -> Selection Decision
 -> Release Bundle
~~~

AI/AutoML optimizer may propose the candidate.

Policy/Verification determines eligibility.

---

## 15. Model registry is a projection/catalog

MLflow/ClearML/W&B model registries can be useful for:
- experiment browsing;
- model discovery;
- aliases;
- collaboration.

But canonical release identity remains:
- Artifact digest;
- Subject Manifest;
- Verification;
- Release Decision.

External registry alias change cannot silently change engineering-platform production subject.

---

## 16. ML Metadata mapping

ML Metadata's core Artifact/Execution/Context lineage maps naturally:

~~~text
MLMD Artifact
 -> engineering Artifact

MLMD Execution
 -> Run / Attempt / Transform

MLMD Context
 -> Work/Task/Experiment grouping
~~~

No separate ML metadata authority database is required unless tooling integration demands one.

TraceLink can express additional semantic relationships.

---

## 17. Model Card / documentation

Model-card practices are useful for summarizing:
- intended use;
- limitations;
- training/evaluation data;
- metrics;
- ethical/safety considerations;
- constraints.

Reserve:

~~~text
ModelCardArtifact
~~~

as a generated/curated document citing exact MLModelArtifact, DatasetArtifacts and Evaluation Evidence.

It is documentation/assurance context, not Release authority.

---

## 18. Experiment tracking tools

Potential future integration:
- MLflow;
- ClearML;
- W&B;
- Metaflow;
- DVC/CML.

Strategy:
- ingest/export identifiers and artifacts;
- do not duplicate authoritative Run/Artifact/Release state;
- external experiment IDs remain typed locators/references.

No M1 dependency.

---

## 19. Audio/KWS-specific application

For audio/KWS pipelines:

~~~text
source corpora
 -> DatasetArtifact
 -> synthetic/augmentation transform
 -> derived DatasetArtifact
 -> ExperimentDefinition
 -> training Run
 -> MLModelArtifact
 -> export/quantization
 -> deployed MLModelArtifact
 -> CalibrationArtifact / thresholds
 -> real/synthetic Evaluation Evidence
 -> Verification
 -> Release Bundle
~~~

This directly supports:
- hard-negative replay;
- failure replay;
- keyword-specific balancing;
- real-human vs synthetic authority;
- target-DUT qualification;
- shipping threshold changes.

---

## 20. Calibration/config changes are real engineering changes

Changing:
- wake threshold;
- decoder threshold;
- NS floor;
- AGC limiter;
- model quantization scale;
- preprocessing parameters

must create a new configuration/calibration identity and trigger impact/reverification according to policy.

Do not hide behavior changes inside "same model version".

---

## 21. M0/M1 impact

M0 reserves:
- DatasetArtifact;
- ExperimentDefinition;
- MLModelArtifact;
- CalibrationArtifact;
- DatasetSeparationEvidence;
- ModelCardArtifact.

M1 does not implement an ML platform.

Existing generic Artifact/Run/Evidence code should be able to represent these types.

---

## 22. M2/M3 or ML-workstream impact

For audio/KWS real adoption:
- import existing training/eval assets;
- freeze dataset manifests/partitions;
- bind training runs;
- capture model/threshold lineage;
- generate exact qualification subject;
- stop relying on mutable paths and ad-hoc experiment names.

---

## 23. New invariants

1. **ML training uses the same formal Run identity as other engineering execution.**
2. **Dataset identity is an immutable snapshot/membership digest, not a directory path.**
3. **Evaluation data independence is proven where qualification requires it.**
4. **Every deployed model transform creates a new Artifact when bytes change.**
5. **Calibration/threshold changes change the deployed Subject.**
6. **Evaluation binds exact model + dataset + metric + procedure + environment.**
7. **Experiment ranking does not equal Release selection.**
8. **External model registries are catalogs/projections, not Release authority.**
9. **Model cards cite facts; they do not manufacture qualification.**
10. **Audio/KWS model, frontend, decoder/threshold and target qualification are one Release subject when behavior depends on all of them.**

---

## 24. Conclusion

Round 13 allows ML/audio/KWS workflows to fit the same engineering control plane without a parallel MLOps governance system.

The core mapping is:

> **Data and models are Artifacts; training is a Run; evaluation is Evidence; qualification is Verification; shipping is Release.**
