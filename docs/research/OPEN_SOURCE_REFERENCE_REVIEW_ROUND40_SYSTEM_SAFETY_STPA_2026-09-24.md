# Open Source Reference Review — Round 40: System Safety, STPA and Hazard Analysis

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- XSTAMPP / STPA
- OSATE / AADL Error Model Annex
- Resolute
- earlier FMEA negative result, ThreatModelArtifact, AssuranceCaseArtifact, TraceLink and Verification semantics

## 1. Key conclusion

System safety is distinct from:
- cybersecurity threat modeling;
- component-centric FMEA;
- generic Incident/Risk;
- compliance certification.

For control-intensive products such as robots, motors, chargers and batteries, the durable chain is:

~~~text
Loss
 -> Hazard
 -> Unsafe Control Action / inadequate feedback
 -> Loss Scenario
 -> Safety Constraint / Requirement
 -> Design / Implementation
 -> Verification Evidence
 -> Residual Safety Risk / Decision
 -> Release
~~~

No separate Safety Plane is required.

---

## 2. SafetyAnalysisArtifact

Add immutable:

~~~text
SafetyAnalysisArtifact
~~~

May contain:
- system boundary;
- losses;
- hazards;
- control structure;
- controllers/controlled processes;
- control actions;
- feedback;
- assumptions;
- analysis method/tool/version;
- content digest.

Possible methods:
- STPA;
- fault tree;
- AADL/EMV2;
- project-specific system hazard analysis.

---

## 3. Loss

Add stable safety concept:

~~~text
SafetyLoss
~~~

Represents an unacceptable outcome, for example:
- human injury;
- fire;
- battery thermal event;
- property damage;
- uncontrolled robot movement;
- loss of critical braking/stopping;
- unsafe charging condition.

Loss is outcome-oriented, not a component failure mode.

---

## 4. SafetyHazard

Add:

~~~text
SafetyHazard
~~~

A system state/condition that, together with worst-case environment, can lead to a SafetyLoss.

Fields:
- hazard ID;
- statement;
- related Losses;
- operating context;
- affected Target/Variant;
- severity/risk;
- assumptions;
- status;
- TraceLinks.

Hazard remains revision/scope bound.

---

## 5. UnsafeControlAction

For control-intensive systems add:

~~~text
UnsafeControlAction
~~~

Captures cases such as:
- required control action not provided;
- unsafe control action provided;
- action provided too early/late/out of sequence;
- action applied too long/stopped too soon.

Examples:
- motor torque remains enabled while robot is expected locked;
- charger enables charge outside safe temperature window;
- docking logic commands wheel movement during an unsafe state;
- emergency stop feedback is ignored/stale.

This is a key STPA concept not naturally represented by FMEA.

---

## 6. LossScenario

Add:

~~~text
LossScenario
~~~

Explains how:
- controller logic;
- process model;
- sensor/feedback;
- actuator;
- timing;
- communication;
- component failure;
- environment

can produce an UnsafeControlAction or unsafe outcome.

It can cite:
- Interface Contract;
- Target;
- component;
- Test/Finding/Incident;
- formal model;
- ThreatScenario where malicious cause overlaps.

---

## 7. SafetyConstraint

Add:

~~~text
SafetyConstraint
~~~

A required constraint derived from hazard analysis.

Examples:
- motor drive shall transition to non-driving safe state within bounded time after stop condition;
- charging shall remain inhibited when measured temperature is outside validated range;
- stale/invalid safety sensor data shall not be interpreted as safe;
- rollback shall not place bootloader/application into incompatible safety-control state.

SafetyConstraint should normally map into ordinary Requirement/AC.

Use TraceLink:
- DERIVES_FROM hazard/scenario;
- IMPLEMENTS design;
- VERIFIES Procedure/Evidence.

---

## 8. Safety requirement stays in normal Requirement authority

Do not create a separate safety requirement database.

Path:

~~~text
SafetyHazard / LossScenario
 -> SafetyConstraint
 -> Requirement / AC
 -> Task / Design
 -> Procedure
 -> Evidence
 -> Verification
~~~

The safety analysis gives provenance/rationale.

Formal Requirement Revision remains the normative delivery contract.

---

## 9. FMEA versus STPA

Keep complementary:

### FMEA
Good for:
- component failure modes;
- local effects;
- failure propagation;
- detectability/severity.

### STPA
Good for:
- unsafe control interactions;
- software/logic/timing;
- inadequate feedback;
- unsafe interactions even without component failure.

Do not force one method to replace the other.

Round 30's negative result remains valid: no new mandatory FMEA/FRACAS subsystem.

---

## 10. Threat Model versus Safety Analysis

Threat Model:
- adversarial/malicious abuse;
- security trust/data flows.

Safety Analysis:
- unacceptable losses/hazards/control behavior;
- accidental/systemic causes.

Overlap is allowed.

Example:
- spoofed temperature sensor can be both ThreatScenario and LossScenario.

Use TraceLinks rather than duplicating the underlying control requirement.

---

## 11. HazardAnalysisReviewReceipt

Safety analysis can become stale.

Add:

~~~text
HazardAnalysisReviewReceipt
~~~

Triggers:
- architecture/control-loop change;
- actuator/sensor substitution;
- timing/interface change;
- software control-law change;
- battery/charger change;
- safety Incident;
- ProductVariant change;
- changed operating environment.

Result:
- CURRENT;
- UPDATE_REQUIRED;
- NEW_HAZARD_IDENTIFIED;
- INCONCLUSIVE.

---

## 12. SafetyVerificationEvidence

No new base Evidence class is required.

Safety-relevant Evidence may include:
- HIL/physical test;
- fault injection;
- timing measurement;
- formal verification;
- simulation with validated fidelity;
- environmental test;
- certification report.

Evidence is tagged/scoped to SafetyConstraint/Requirement through TraceLink.

---

## 13. Safe-state definition

For relevant Target/component define a versioned:

~~~text
SafeStateDefinition
~~~

May specify:
- actuator outputs;
- power state;
- braking/drive state;
- charging state;
- degraded-mode behavior;
- maximum transition time;
- conditions under which safe state is valid.

Different hazards may require different safe states.

Avoid the vague assumption that "shutdown" is always safe.

---

## 14. Safety transition Evidence

Where safety depends on transition timing/state:

~~~text
SafetyTransitionEvidence
~~~

binds:
- initial state;
- trigger;
- exact firmware/config;
- safe-state definition;
- measured transition time;
- environment;
- equipment/calibration;
- result.

This integrates directly with Round 39 metrology semantics.

---

## 15. Assurance Case relationship

For A3/A4/high-risk products:

~~~text
SafetyLoss / Hazard
 -> SafetyConstraint
 -> Verification Evidence
 -> optional AssuranceCaseArtifact
 -> independent Review / Decision
~~~

Assurance Case cites safety evidence and assumptions.

It does not create them.

---

## 16. XSTAMPP practice

XSTAMPP is an open-source STAMP/STPA platform and demonstrates practical representation of:
- losses/accidents;
- hazards;
- control structures;
- unsafe control actions;
- causal scenarios.

The original implementation is older; XSTAMPP 4 explores collaborative hazard analysis.

Decision:
- use it as STPA data-model/workflow reference;
- do not make it an M1/M2 platform dependency.

---

## 17. OSATE/AADL practice

OSATE remains an actively maintained AADL environment.

Its Error Model Annex and analyses support architecture-level:
- error propagation;
- fault trees;
- hazard analysis.

Its ALISA/Resolute ecosystem also demonstrates integrating requirements, verification and assurance claims with architecture models.

Decision:
- useful for complex/safety-critical architecture programs;
- no required adoption for ordinary PCR-style projects.

---

## 18. M0/M1 impact

M0 reserves:
- SafetyAnalysisArtifact;
- SafetyLoss;
- SafetyHazard;
- UnsafeControlAction;
- LossScenario;
- SafetyConstraint;
- HazardAnalysisReviewReceipt;
- SafeStateDefinition;
- SafetyTransitionEvidence.

M1 requires no STPA/safety tool.

Create one synthetic product-safety fixture only.

---

## 19. M2/M3 impact

For robot/motor/charger pilots:
- model one control loop;
- identify unsafe control actions;
- bind safety constraints to existing Requirements;
- verify safe-state transition on HIL/physical device;
- re-review after hardware/software/control changes.

---

## 20. Invariants

1. System safety is distinct from cybersecurity and component failure analysis.
2. Safety Hazards describe unsafe system conditions, not merely failed components.
3. Unsafe Control Actions capture timing/omission/sequence/feedback hazards.
4. Safety Constraints trace into normal Requirement/Verification authority.
5. Hazard analysis is revision/freshness bound.
6. "Shutdown" is not assumed universally safe; safe state is explicit.
7. Safety transition/timing measurements use normal calibrated Measurement Evidence.
8. FMEA, STPA and Threat Modeling are complementary, not interchangeable.
9. Safety Evidence may include formal/simulation/HIL/physical methods according to fidelity policy.
10. Assurance Case may organize safety evidence but never manufacture it.

## 21. Conclusion

The durable system-safety rule is:

> **For every unacceptable loss, trace the hazardous control behavior to explicit safety constraints and prove those constraints on the exact product configuration—especially across timing, feedback, degraded and transition states.**
