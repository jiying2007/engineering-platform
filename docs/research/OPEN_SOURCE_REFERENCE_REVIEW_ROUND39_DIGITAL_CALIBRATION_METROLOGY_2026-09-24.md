# Public Standard Reference Review — Round 39: Digital Calibration, Measurement Units and Uncertainty

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- PTB Digital Calibration Certificate (DCC)
- ISO/IEC 17025-oriented DCC structure
- existing EquipmentCalibrationRecord, Measurement, Procedure, TestAttempt and Evidence semantics

## 1. Key conclusion

An engineering measurement is not just:

~~~text
name = current
value = 1.50
~~~

For authoritative Evidence it should preserve:
- what was measured;
- unit;
- uncertainty/resolution;
- method;
- instrument;
- calibration status;
- environment/conditions;
- exact sample/device;
- timestamp;
- raw data lineage.

This is necessary for comparability, thresholds and re-analysis.

---

## 2. MeasurementDefinition

Refine existing Procedure measurement definitions:

~~~text
MeasurementDefinition
~~~

Fields:
- measurement ID/name;
- measurand/quantity;
- expected unit/dimension;
- allowed unit representations;
- sampling/aggregation;
- tolerance/specification;
- uncertainty requirement;
- resolution requirement;
- environmental-condition requirements;
- required instrument capability/calibration class;
- evaluation rule/version.

The definition is versioned with ProcedureRevision.

---

## 3. MeasurementResult

Canonical result:

~~~text
MeasurementResult
~~~

Binds:
- MeasurementDefinition;
- Sample/Device/TestAttempt;
- measured value or series;
- unit;
- timestamp/time range;
- standard/expanded uncertainty where available;
- coverage factor/probability where applicable;
- resolution;
- method;
- instrument/equipment ref;
- calibration record/certificate ref;
- environment/influence conditions;
- raw data Artifact;
- result digest.

Do not store only formatted display strings.

---

## 4. Unit identity

Unit must be machine-readable and dimension-aware.

Rules:
- "A" and "mA" can be converted;
- "°C" and "K" require correct affine conversion;
- "rad/s" and "rpm" are convertible with explicit formula;
- dimension mismatch fails validation;
- free-text units are not accepted for Formal Evidence unless explicitly mapped.

Use a canonical unit vocabulary/URI/profile appropriate to implementation.

---

## 5. Measurement uncertainty

For measurements where uncertainty matters, distinguish:
- measured value;
- standard uncertainty;
- expanded uncertainty;
- coverage factor/confidence;
- resolution;
- tolerance/specification.

Do not conflate:
- tolerance of DUT;
- accuracy of instrument;
- measurement uncertainty;
- repeatability;
- resolution.

These are different engineering quantities.

---

## 6. Decision near limits

Evaluation policy should define handling when uncertainty overlaps the acceptance boundary.

Possible policies:
- conservative fail;
- guard band;
- conditional/manual review;
- measurement re-run with better instrument.

Do not silently compare nominal value only.

---

## 7. DigitalCalibrationCertificateArtifact

Add optional Artifact specialization:

~~~text
DigitalCalibrationCertificateArtifact
~~~

Metadata:
- equipment identity;
- calibration laboratory;
- certificate/schema version;
- calibration date;
- validity/review;
- measurement results;
- method;
- conditions;
- uncertainty;
- signer/issuer;
- content digest.

PTB DCC is the preferred interoperability candidate where applicable.

Existing EquipmentCalibrationRecord may reference this Artifact.

---

## 8. Calibration applicability

Calibration record/certificate applies to:
- exact equipment identity;
- quantity/range;
- method/configuration;
- date/validity;
- sometimes location/environment.

A current certificate does not mean every capability/range of the instrument is calibrated for every Procedure.

Procedure policy checks capability/scope.

---

## 9. MeasurementTraceabilityProjection

Generate a read/proof chain:

~~~text
MeasurementResult
 -> Instrument / Equipment
 -> EquipmentCalibrationRecord
 -> DigitalCalibrationCertificateArtifact
 -> reference standard / calibration laboratory
~~~

This is metrological traceability metadata.

It is not a separate authority database.

---

## 10. Environmental / influence conditions

DCC's explicit influence-condition model is valuable.

MeasurementResult/TestAttempt should capture where relevant:
- ambient temperature;
- humidity;
- supply voltage;
- fixture preload;
- orientation;
- warm-up time;
- state-of-charge;
- airflow;
- other known influences.

Conditions may be:
- measured values;
- accepted ranges;
- environment-definition refs.

---

## 11. Raw data vs calculated result

Keep:

~~~text
Raw Measurement Artifact
 -> calculation/aggregation Procedure
 -> MeasurementResult
 -> evaluation
 -> TestAttempt/Verdict/Evidence
~~~

Changing calculation/evaluation logic can recompute result/verdict without rewriting raw data.

---

## 12. Derived/calculated quantities

For derived values:
- formula/version;
- input measurement refs;
- constants/config;
- unit conversion;
- uncertainty propagation method

must be retained.

Example:
- wheel speed converted to rad/s;
- efficiency derived from voltage/current/speed;
- battery internal resistance;
- latency percentiles.

---

## 13. Calibration expiry during long tests

If an instrument calibration expires during:
- soak test;
- long qualification;
- repeated production run

policy defines applicability.

Do not silently treat a measurement after expiry as equivalent to one before expiry.

---

## 14. Instrument substitution

Replacing an instrument/fixture may change:
- accuracy;
- bandwidth;
- resolution;
- calibration;
- loading effect.

Use Equipment/Station/Procedure Impact Analysis before reusing old Evidence.

---

## 15. External laboratory measurement import

LaboratoryResultImportReceipt should normalize:
- measurement units;
- uncertainty;
- method;
- calibration references;
- conditions

where the external report provides them.

Preserve the original report Artifact as source.

---

## 16. PTB DCC practice

PTB DCC provides a machine-readable schema with:
- administrative data;
- calibration laboratory/persons;
- items;
- methods/equipment;
- measurement results;
- influence conditions;
- uncertainty-related structures.

### Decision

Use DCC-compatible import/export for calibration workflows where beneficial.

Do not force DCC as internal canonical representation for every simple engineering measurement.

Official references:
- https://www.ptb.de/dcc/
- https://www.ptb.de/dcc/v3.3.0/autogenerated-docs/dcc_xsd.html

---

## 17. M0/M1 impact

M0 refines/freezes:
- MeasurementDefinition;
- MeasurementResult;
- uncertainty/unit fields;
- DigitalCalibrationCertificateArtifact;
- MeasurementTraceabilityProjection schema.

M1 needs no calibration-certificate service.

---

## 18. M2/M3 impact

Apply to:
- HIL current/voltage/temperature;
- motor speed/torque;
- audio level/SNR;
- battery/charger testing;
- production calibration;
- certification laboratory imports.

---

## 19. Invariants

1. Formal MeasurementResult always identifies quantity and machine-readable unit.
2. Unit dimension mismatch is rejected.
3. Tolerance, instrument accuracy, uncertainty and resolution remain distinct.
4. Uncertainty handling near limits is explicit policy.
5. Measurement binds instrument and applicable calibration scope where required.
6. Raw data remains immutable beneath derived/evaluated results.
7. Derived quantities retain formula/input/unit/uncertainty provenance.
8. Influence conditions are part of Evidence where they affect validity.
9. Calibration expiry/substitution can invalidate future measurement applicability.
10. Digital calibration certificates are interoperable artifacts, not a new authority plane.

## 20. Conclusion

The durable metrology rule is:

> **A trustworthy test result must make the measurement reproducible in meaning—not only preserve the number, but also the quantity, unit, uncertainty, instrument, calibration and conditions that made that number credible.**
