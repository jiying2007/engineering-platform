# M0 Reference Adoption Plan v39

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V38.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V39.md

## 1. Principle

Round 39 refines measurement semantics without adding a metrology service to M1.

M0 must prove that a formal test result preserves enough metrological context to remain comparable and auditable.

---

# Track A — Measurement schema

## 2. MeasurementDefinition fixture

Create definitions for:
- motor current (A);
- motor speed (rad/s);
- temperature (degC);
- latency (ms).

Include:
- tolerance;
- expected unit;
- resolution/uncertainty requirement;
- environmental constraints.

---

## 3. Unit validation fixture

Prove:
- mA -> A conversion works;
- rpm -> rad/s conversion works;
- valid Celsius/Kelvin conversion works;
- dimension mismatch fails;
- unmapped free-text unit is rejected for Formal Evidence.

---

# Track B — MeasurementResult

## 4. Raw/result fixture

Create:
- Raw Measurement Artifact;
- derived MeasurementResult;
- evaluation result.

Prove:
- raw data is immutable;
- changing evaluation threshold changes verdict but not raw/result history;
- derived result retains calculation version.

---

## 5. Uncertainty fixture

Create one acceptance boundary example.

Example:
- limit 1.50 A;
- measured value near boundary;
- non-zero uncertainty.

Test two policies:
- nominal comparison;
- guard-band/conservative policy.

Prove policy version is explicit and verdict differs without rewriting measurement.

---

# Track C — Calibration

## 6. Equipment calibration fixture

Create:
- EquipmentCalibrationRecord;
- DigitalCalibrationCertificateArtifact fixture;
- valid quantity/range;
- expired/out-of-range case.

Prove:
- current certificate but wrong measurement range is rejected;
- expired calibration blocks authoritative measurement according to policy.

---

## 7. Calibration substitution fixture

Replace one instrument with a different accuracy/resolution profile.

Prove:
- Station/Procedure impact is detected;
- old Evidence is not reused automatically;
- new calibration identity appears in MeasurementResult.

---

# Track D — Influence conditions

## 8. Environmental-condition fixture

Create same motor measurement under:
- valid ambient temperature;
- outside declared Procedure range.

Prove:
- out-of-range condition is visible/inapplicable according to policy;
- the numeric value itself is not silently treated as valid.

---

# Track E — Existing M0

## 9. Retain all v38 requirements

All prior M0 work remains:
- sample/custody/DPP;
- audit/recovery;
- certification/privacy/prognostics/compatibility/threat modeling;
- supplier/compliance/twin/FOSS;
- PLM/PSIRT/RMA/variants;
- AI governance/review;
- Device trust;
- ML lineage;
- Interface compatibility;
- ResolvedConfiguration;
- TraceLink;
- Run/Build/Integration/Release;
- OTel/CDEvents;
- formal model checking;
- failure injection.

---

## 10. M0 exit additions

M0 additionally requires:
- canonical MeasurementDefinition/MeasurementResult schema;
- machine-readable unit conversion/dimension tests;
- uncertainty-aware boundary fixture;
- calibration scope/expiry test;
- instrument substitution impact fixture;
- influence-condition applicability fixture.

No DCC server/calibration platform is required.

---

## 11. M1 target

M1 remains the same narrow formal engineering loop.

The measurement schema should already be usable by CI/performance tests, while full metrology/calibration integration arrives with M2 HIL/manufacturing.
