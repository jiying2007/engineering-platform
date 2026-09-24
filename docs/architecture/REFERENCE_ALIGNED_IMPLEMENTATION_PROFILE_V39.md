# Reference-Aligned Implementation Profile v39

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V38.md

Research basis:
- Open Source Reference Review Rounds 1–39
- Open Source / Public Practice Synthesis Rounds 31–39
- Architecture v1.2

## 1. Goal

Profile v39 retains all v38 semantics and adds machine-readable metrology:

- MeasurementDefinition with quantity/unit/uncertainty requirements;
- canonical MeasurementResult;
- DigitalCalibrationCertificateArtifact;
- calibration-scope checks;
- MeasurementTraceabilityProjection.

No new mandatory M1 runtime service is introduced.

---

## 2. MeasurementDefinition

Versioned with ProcedureRevision:

~~~text
MeasurementDefinition
~~~

Contains:
- measurement ID/name;
- measurand/quantity;
- expected unit/dimension;
- allowed unit representations;
- sampling/aggregation;
- tolerance/specification;
- uncertainty requirement;
- resolution requirement;
- required influence-condition range;
- instrument capability/calibration requirements;
- evaluation rule/version.

---

## 3. MeasurementResult

Canonical result:

~~~text
MeasurementResult
~~~

Binds:
- MeasurementDefinition;
- Sample/Device/TestAttempt;
- measured value/series;
- machine-readable unit;
- timestamp/range;
- standard/expanded uncertainty where applicable;
- coverage factor/probability where applicable;
- resolution;
- method;
- equipment/instrument;
- calibration record/certificate;
- environment/influence conditions;
- raw data Artifact;
- content/result digest.

Display strings are projections.

---

## 4. Unit semantics

Formal Evidence requires:
- known quantity dimension;
- validated unit;
- deterministic conversion;
- rejection of dimension mismatch.

Examples:
- mA to A;
- rpm to rad/s;
- Celsius/Kelvin with correct affine semantics.

Free-text unit strings are not sufficient unless explicitly mapped.

---

## 5. Tolerance, accuracy, uncertainty and resolution remain distinct

Do not conflate:
- DUT specification/tolerance;
- instrument accuracy/specification;
- measurement uncertainty;
- repeatability;
- display/ADC resolution.

Evaluation policy may use all of them.

---

## 6. Guard bands / boundary decisions

When uncertainty overlaps an acceptance threshold, policy may require:
- guard band;
- conservative fail;
- re-measurement;
- better instrument;
- manual review;
- conditional result.

Nominal-value comparison alone is not enough for high-assurance measurement.

---

## 7. DigitalCalibrationCertificateArtifact

Add optional Artifact specialization:

~~~text
DigitalCalibrationCertificateArtifact
~~~

May use PTB DCC-compatible serialization.

Metadata:
- equipment identity;
- calibration laboratory;
- certificate/schema version;
- calibration date;
- validity/review;
- quantity/ranges;
- measurement results;
- method/conditions;
- uncertainty;
- issuer/signature;
- digest.

Existing EquipmentCalibrationRecord references the certificate.

---

## 8. Calibration applicability

Calibration applicability checks:
- exact equipment identity;
- quantity;
- range;
- mode/configuration;
- date/validity;
- location/environment if relevant.

A valid certificate does not mean every function/range of an instrument is valid for every Procedure.

---

## 9. MeasurementTraceabilityProjection

Derived:

~~~text
MeasurementResult
 -> Equipment
 -> EquipmentCalibrationRecord
 -> DigitalCalibrationCertificateArtifact
 -> calibration laboratory / reference standard refs
~~~

This is a traceability/read model, not a new source of authority.

---

## 10. Influence conditions

Measurement/TestAttempt records relevant:
- ambient temperature;
- humidity;
- supply voltage;
- fixture preload;
- orientation;
- warm-up;
- airflow;
- state of charge;
- other known influence quantities.

Where material, influence conditions become part of Evidence applicability.

---

## 11. Raw versus calculated measurements

Keep:

~~~text
Raw Measurement Artifact
 -> calculation/aggregation transform
 -> MeasurementResult
 -> evaluation/verdict
~~~

Changing calculation or evaluation logic does not rewrite raw data.

---

## 12. Derived quantities

Derived MeasurementResult retains:
- formula/version;
- input MeasurementResult refs;
- constants/config;
- unit conversions;
- uncertainty propagation method;
- calculation receipt.

---

## 13. Calibration expiry/substitution

Policy handles:
- calibration expiry during long test;
- instrument substitution;
- fixture replacement;
- changed acquisition chain.

These changes can trigger:
- Evidence invalidation;
- Impact Analysis;
- remeasurement/requalification.

---

## 14. External lab import

LaboratoryResultImportReceipt should normalize where available:
- unit;
- uncertainty;
- method;
- instrument/calibration;
- influence conditions.

Original report remains immutable source Artifact.

---

## 15. Implementation strategy

M1:
- internal canonical MeasurementDefinition/Result schema;
- simple unit library/validation;
- no DCC service.

M2:
- HIL/production measurement integration;
- DCC import/export where useful;
- equipment calibration scope enforcement.

M3:
- certification/metrology integrations as actual product needs demand.

---

## 16. Final rules added by v39

1. Formal measurements always identify quantity and machine-readable unit.
2. Dimension mismatch is rejected.
3. DUT tolerance, instrument accuracy, uncertainty and resolution stay distinct.
4. Boundary decisions define how uncertainty affects pass/fail.
5. Measurements bind the equipment and applicable calibration scope when required.
6. Raw data remains immutable beneath derived results.
7. Derived quantities retain formula/input/unit/uncertainty provenance.
8. Influence conditions participate in applicability when material.
9. Calibration expiry or instrument substitution can invalidate later Evidence.
10. DCC is an interoperability Artifact, not a new authority plane.

Architecture v1.2 remains canonical.
Profile v39 is the current implementation companion.
