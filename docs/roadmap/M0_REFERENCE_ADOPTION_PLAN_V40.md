# M0 Reference Adoption Plan v40

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V39.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V40.md

## 1. Principle

Round 40 adds system-safety schema/fixtures without introducing a safety-analysis service into M1.

The objective is to prove that hazard/control constraints can trace into normal Requirements and Verification.

---

# Track A — Safety analysis

## 2. Synthetic loss/hazard fixture

Create one robot/motor scenario:

~~~text
Loss:
  human/property harm due uncontrolled robot motion

Hazard:
  drive torque remains active when motion must be inhibited
~~~

Bind:
- TargetRevision;
- operating context;
- assumptions.

---

## 3. Unsafe control-action fixture

Create examples:
- stop command not issued;
- drive-enable issued while docked/unsafe;
- stop occurs too late;
- torque remains applied too long.

Prove UnsafeControlAction can link to:
- Interface Contract;
- controller software;
- sensor/feedback;
- timing Measurement.

---

## 4. Loss scenario fixture

Create one scenario involving:
- stale feedback;
- controller process-model mismatch;
- delayed MCU communication.

Trace:
- UnsafeControlAction;
- SafetyConstraint;
- affected Requirement/AC;
- TestDefinition/Procedure.

---

# Track B — Safe state / Verification

## 5. SafeStateDefinition fixture

Define one motor safe state:
- no driving torque;
- bounded transition time;
- specific brake/lock behavior;
- required valid conditions.

Prove:
- "shutdown" and "safe state" are not hard-coded synonyms;
- different context can select a different SafeStateDefinition.

---

## 6. Safety transition fixture

Create calibrated Measurement Evidence:
- stop trigger;
- actual torque/current/speed transition;
- transition time;
- environment;
- instrument/calibration;
- pass/fail rule.

Prove v39 MeasurementResult semantics can support a safety requirement.

---

# Track C — Freshness / change impact

## 7. Hazard-review fixture

Change one:
- motor control timing;
- feedback interface;
- charger threshold;
- ProductVariant.

Create HazardAnalysisReviewReceipt.

Prove:
- old analysis can become UPDATE_REQUIRED;
- Hazard/Constraint history remains immutable;
- change triggers Impact Analysis rather than silent carry-forward.

---

# Track D — Relationship with security/FMEA

## 8. Overlap fixture

Create one spoofed/stale temperature example:
- ThreatScenario for malicious/stale data;
- LossScenario for unsafe charging;
- one shared SafetyConstraint.

Prove:
- Threat Model and Safety Analysis do not duplicate Requirement authority;
- same Verification Evidence can trace to both where valid.

---

# Track E — Existing M0

## 9. Retain all v39 requirements

All prior M0 work remains:
- metrology/calibration;
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
- one SafetyLoss/Hazard/UCA/LossScenario chain;
- one SafetyConstraint traced into formal Requirement/AC;
- one SafeStateDefinition;
- one metrology-backed SafetyTransitionEvidence fixture;
- one hazard freshness/re-review fixture;
- one Threat/Safety shared-constraint example.

No XSTAMPP/OSATE/STPA service is required.

---

## 11. M1 target

M1 remains operationally unchanged.

The added safety fixture ensures future robot/motor/charger safety work can use the same Requirement/Evidence/Verification chain rather than growing a parallel safety database.
