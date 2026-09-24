# Open Source Reference Review — Round 33: Field Telemetry, Anomaly Detection and Prognostics

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- NASA ProgPy
- Numenta Anomaly Benchmark (NAB)
- Eclipse Ditto / ThingsBoard observed-state practices
- existing Digital Twin, Incident, MLModelArtifact, MetricDefinition and Maintenance/RMA semantics

## 1. Key conclusion

Field telemetry can support anomaly detection and predictive maintenance, but predictions are probabilistic observations—not engineering authority.

The durable loop is:

~~~text
Telemetry / ObservedDeviceState
 -> feature/health extraction
 -> AnomalyFinding
 -> PrognosticEstimate
 -> MaintenanceRecommendation
 -> policy/human Decision
 -> Maintenance/Service Work
 -> observed outcome
 -> model/recommendation calibration
~~~

---

## 2. TelemetryWindowArtifact

For analysis, define immutable windows/snapshots:

~~~text
TelemetryWindowArtifact
~~~

Binds:
- DeviceInstance or population;
- metric/channel definitions;
- start/end times;
- sampling/aggregation;
- missing-data rules;
- source/provider;
- quality/freshness;
- content/object digest;
- DataAssetProfile/retention.

High-volume stream storage may remain external.

The Artifact is the exact input used for analysis.

---

## 3. HealthModelArtifact

Use ModelArtifact specialization:

~~~text
HealthModelArtifact
~~~

May contain:
- physics/prognostic model;
- anomaly detector;
- state estimator;
- degradation model;
- threshold/rule model.

Metadata:
- model/tool/version;
- training/calibration data;
- input/output schema;
- Target/variant scope;
- assumptions;
- qualification Evidence;
- content digest.

NASA ProgPy is a strong reference for state estimation, prediction and remaining-useful-life algorithms.

---

## 4. HealthStateEstimate

Add:

~~~text
HealthStateEstimate
~~~

Binds:
- DeviceInstance/population;
- TelemetryWindowArtifact;
- HealthModelArtifact;
- exact Runtime/config;
- estimated state variables;
- uncertainty/confidence;
- timestamp/forecast horizon;
- raw result Artifact.

Examples:
- battery state of health;
- motor bearing/vibration health;
- thermal margin;
- flash wear;
- sensor degradation.

It is an estimate, not a directly observed fact.

---

## 5. AnomalyFinding

Reuse generic Finding with subtype/category:

~~~text
ANOMALY
PERFORMANCE_REGRESSION
HEALTH_DEGRADATION
OUTLIER
~~~

Include:
- detector/model;
- score;
- threshold/profile;
- observed window;
- expected baseline;
- confidence;
- supporting Evidence.

NAB's benchmark practice reinforces that anomaly-detector quality depends on scoring profile and false-positive/false-negative tradeoffs.

---

## 6. PrognosticEstimate

Add:

~~~text
PrognosticEstimate
~~~

For forecasts such as:
- remaining useful life;
- probability of threshold crossing;
- expected failure time/range;
- degradation trajectory.

Binds:
- exact HealthModelArtifact;
- observed state/telemetry;
- assumptions;
- uncertainty distribution/range;
- prediction horizon;
- evaluation/calibration profile.

A point estimate without uncertainty is insufficient for high-impact action.

---

## 7. MaintenanceRecommendation

Add proposal:

~~~text
MaintenanceRecommendation
~~~

Examples:
- inspect;
- replace component;
- derate;
- recalibrate;
- schedule service;
- collect additional diagnostics.

Fields:
- triggering estimate/finding;
- action;
- urgency/window;
- expected risk reduction;
- model/profile;
- confidence;
- affected device/population.

Recommendation is not execution authority.

---

## 8. MaintenanceDecision

Use standard typed Decision to:
- accept recommendation;
- defer;
- reject;
- request diagnostics;
- quarantine;
- create Service/RMA Work.

High-impact autonomous actions remain policy/Human Authority governed.

---

## 9. MaintenanceActionReceipt

For proactive field service:

~~~text
MaintenanceActionReceipt
~~~

Binds:
- DeviceInstance;
- recommendation/Decision;
- actual action;
- replaced/adjusted component/config;
- technician/provider;
- time/location;
- post-maintenance state;
- Verification Evidence.

This complements RepairActionReceipt for failure-driven RMA.

---

## 10. Model qualification

A predictive model must be qualified on appropriate data.

Use existing:
- DatasetArtifact;
- DatasetSeparationEvidence;
- MLModelArtifact/ModelArtifact;
- Evaluation Evidence;
- RuntimeQualification concepts.

Metrics may include:
- false alarm rate;
- missed-event rate;
- detection delay;
- RUL error;
- calibration;
- confidence coverage;
- lead time.

---

## 11. Population drift

Health-model validity may change with:
- new hardware revision;
- supplier part;
- firmware;
- calibration;
- operating environment;
- geography;
- user behavior;
- aging population.

Trace/Impact Analysis can mark model applicability stale when relevant Target/Variant inputs change.

---

## 12. Ground truth feedback

Maintenance/RMA outcomes provide labels:
- true positive;
- false positive;
- missed failure;
- confirmed degradation;
- no-fault-found.

Feed these into:
- Runtime/model qualification;
- failure replay;
- retraining;
- threshold tuning.

Do not rewrite original predictions after outcome is known.

---

## 13. Digital Twin relationship

DeviceTwinProjection may show:
- latest health estimate;
- anomaly state;
- maintenance recommendation.

But twin state remains a read model.

Canonical historical records remain:
- TelemetryWindowArtifact;
- HealthStateEstimate;
- Findings;
- PrognosticEstimate;
- Decisions;
- MaintenanceActionReceipt.

---

## 14. Battery use case

PyBaMM is useful for:
- battery electrochemical/thermal simulation;
- parameter studies;
- experiment simulation.

ProgPy-style prognostics can support:
- health estimation;
- remaining-useful-life forecasting.

Physical safety/certification and actual pack calibration remain separate Evidence requirements.

Model estimates do not substitute for battery safety qualification.

---

## 15. M0/M1 impact

M0 reserves:
- TelemetryWindowArtifact;
- HealthModelArtifact;
- HealthStateEstimate;
- PrognosticEstimate;
- MaintenanceRecommendation;
- MaintenanceActionReceipt.

M1 requires no prognostics service.

---

## 16. M2/M3 impact

When fleet telemetry exists:
- build quality-controlled telemetry windows;
- start with simple anomaly/rule models;
- qualify false-positive/false-negative behavior;
- create maintenance recommendations;
- close loop with service/RMA outcomes.

---

## 17. Invariants

1. Telemetry stream and analysis snapshot are distinct.
2. Health estimates/predictions are model outputs with uncertainty.
3. Anomaly threshold/profile is part of analysis identity.
4. Prognostic estimates do not automatically authorize maintenance actions.
5. Model qualification is Target/variant/population scoped.
6. Hardware/software/config changes can invalidate model applicability.
7. Ground-truth service/RMA outcome calibrates models without rewriting historical predictions.
8. Digital Twin displays prognostic state but does not own it.
9. Predictive maintenance actions produce receipts and post-action Verification.
10. Safety/certification Evidence is not replaced by health prediction.

## 18. Conclusion

The durable prognostics rule is:

> **Treat predictions as versioned, uncertainty-bearing Evidence inputs, then make maintenance decisions through normal policy and service workflows.**
