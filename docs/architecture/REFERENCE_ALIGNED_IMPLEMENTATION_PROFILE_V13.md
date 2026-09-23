# Reference-Aligned Implementation Profile v13

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V12.md

Research basis:
- Open Source Reference Review Rounds 1–13
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v13 integrates experiment/data/model/calibration workflows into the existing engineering authority model without creating a parallel MLOps control plane.

Core mapping:

~~~text
Dataset / Model / Calibration
 -> Artifact

Training / optimization
 -> Task + Formal Run

Evaluation
 -> Evidence

Qualification
 -> Verification

Shipping
 -> Release
~~~

---

## 2. DatasetArtifact

Add Artifact specialization:

~~~text
DatasetArtifact
~~~

Metadata:
- snapshot/content digest;
- source/provenance;
- generation/import procedure;
- license/usage constraints;
- confidentiality/privacy class;
- schema/feature definition;
- sample/object manifest;
- partition/split metadata;
- preprocessing/augmentation lineage;
- statistics/quality refs.

Directory/object-store path is only a locator.

---

## 3. Dataset partitions

Standard partition roles:

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

Each partition has exact membership/snapshot identity.

Membership change creates a new digest.

---

## 4. Dataset separation Evidence

When qualification depends on evaluation independence, use:

~~~text
DatasetSeparationEvidence
~~~

Possible checks:
- exact overlap;
- speaker/source/session overlap;
- device/site overlap;
- synthetic generator family;
- temporal split;
- provenance independence;
- contamination.

Different folders do not prove independence.

---

## 5. ExperimentDefinition

Immutable:

~~~text
ExperimentDefinition
~~~

Contains:
- source code/commit;
- dataset refs;
- training/evaluation config;
- preprocessing/augmentation;
- architecture;
- hyperparameters/search space;
- random seeds;
- framework/toolchain/environment;
- resource profile;
- expected output roles;
- evaluation plan.

Run Input Manifest references it.

---

## 6. Experiment execution uses existing Run

No separate ML Run authority.

~~~text
ExperimentDefinition
 -> Run Input Manifest
 -> ExecutionSpec
 -> Run / Attempt
 -> Run Receipt
~~~

Run Receipt records actual:
- config;
- datasets;
- seeds;
- environment;
- metrics;
- model/checkpoint Artifacts;
- nondeterminism/deviations.

---

## 7. MLModelArtifact

Explicit Artifact type:

~~~text
MLModelArtifact
~~~

Metadata:
- model family/architecture;
- framework/format;
- input/output contract;
- ExperimentDefinition/Run;
- datasets;
- preprocessing;
- bytes digest;
- size/parameter metadata;
- Target compatibility;
- transform/export lineage.

Registry aliases are locators/labels only.

---

## 8. ML model transform lineage

Each byte-changing stage creates new Artifact:

~~~text
checkpoint
 -> export
 -> quantize
 -> optimize/compile
 -> package
 -> deployed model
~~~

Transform Receipt binds input/output digests.

Evaluation reuse across transforms is policy-controlled.

---

## 9. CalibrationArtifact

Add:

~~~text
CalibrationArtifact
~~~

Examples:
- quantization calibration;
- normalization statistics;
- KWS/decoder thresholds;
- VAD thresholds;
- NS/AGC parameters;
- frontend constants;
- feature configuration.

Changing behavior-affecting calibration changes the deployment subject even if model bytes remain unchanged.

---

## 10. ML Release Bundle

A deployed ML subject may include:

~~~text
MLModelArtifact
CalibrationArtifact
preprocessing/frontend Artifact
tokenizer/label/vocabulary Artifact
runtime/library profile
Target Revision
RuntimeConfigurationSnapshot
~~~

Release authority binds the exact bundle.

---

## 11. Evaluation Evidence

Trusted evaluation binds:
- exact model Artifact;
- exact DatasetArtifact/partition;
- evaluator/Procedure revision;
- MetricDefinition version;
- Target/environment;
- raw predictions/results when retained;
- statistical method.

Example metrics:
- FAR/FRR;
- accuracy;
- latency;
- memory;
- model size;
- robustness;
- slice/per-keyword metrics.

---

## 12. Model qualification

Candidate experiment ranking is not Release selection.

Formal path:

~~~text
candidate models
 -> Evaluation Evidence
 -> Verification
 -> Selection Decision
 -> Release Bundle
~~~

AutoML/optimizer may propose candidate only.

---

## 13. Model registry integrations

MLflow/ClearML/W&B or similar systems may provide:
- experiment UI;
- tracking;
- model discovery;
- aliases;
- collaboration.

They remain external catalogs/projections.

Canonical identity:
- Artifact digest;
- Subject Manifest;
- Verification;
- Release Decision.

External alias change cannot change production subject.

---

## 14. ML metadata mapping

Generic mapping:

~~~text
external Artifact
 -> engineering Artifact

external Execution
 -> Run/Attempt/Transform

external Experiment/Context
 -> Work/Task/Experiment grouping
~~~

No second authoritative ML metadata DB is required by the domain model.

---

## 15. ModelCardArtifact

Reserve:

~~~text
ModelCardArtifact
~~~

Cites:
- exact model;
- datasets;
- intended use;
- limitations;
- Evaluation Evidence;
- Target/runtime constraints.

It is documentation/assurance context.

It does not approve release.

---

## 16. Audio/KWS profile

For audio/KWS:

~~~text
Source Dataset
 -> Dataset transforms/augmentation
 -> ExperimentDefinition
 -> Training Run
 -> MLModelArtifact
 -> export/quantization
 -> deployed MLModelArtifact
 -> CalibrationArtifact / thresholds
 -> Evaluation Evidence
 -> Verification
 -> Release Bundle
~~~

Supports:
- hard-negative replay;
- failure replay;
- real/synthetic authority separation;
- keyword slices;
- target-DUT qualification;
- shipping-threshold changes.

---

## 17. Milestone mapping

### M0
Reserve/freeze:
- DatasetArtifact;
- ExperimentDefinition;
- MLModelArtifact;
- CalibrationArtifact;
- DatasetSeparationEvidence;
- ModelCardArtifact.

### M1
No ML platform required.

Generic Artifact/Run/Evidence implementation must support these types.

### ML workstreams / M2+
Adopt actual dataset/model lineage for audio/KWS and future on-device ML pipelines.

---

## 18. Final rules added by v13

1. ML uses the same Run/Artifact/Evidence/Verification authority model.
2. Dataset identity is immutable snapshot membership, not path.
3. Qualification dataset independence is proven when required.
4. Model transforms create new Artifacts when bytes change.
5. Calibration/threshold changes change deployment subject.
6. Evaluation binds exact model, dataset, metric, procedure and environment.
7. Model ranking does not equal Release selection.
8. Model registry aliases are not production identity.
9. Model cards cite evidence; they do not manufacture qualification.
10. ML model + frontend/config/calibration + target form one behavioral release subject where applicable.

Architecture v1.2 remains canonical.
Profile v13 is the current implementation companion.
