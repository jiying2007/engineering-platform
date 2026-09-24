# Reference-Aligned Implementation Profile v40

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V39.md

Research basis:
- Open Source Reference Review Rounds 1–40
- Architecture v1.2

## 1. Goal

Profile v40 retains all v39 semantics and adds lightweight system-safety/STPA concepts for control-intensive products.

No new Safety Plane or mandatory M1 service is introduced.

---

## 2. SafetyAnalysisArtifact

Immutable:

~~~text
SafetyAnalysisArtifact
~~~

Captures:
- system boundary;
- control structure;
- controllers/controlled processes;
- control actions/feedback;
- losses;
- hazards;
- assumptions;
- method/tool/version;
- content digest.

Possible methods:
- STPA;
- fault tree;
- AADL/EMV2;
- project-specific hazard analysis.

---

## 3. SafetyLoss

Add stable unacceptable outcome:

~~~text
SafetyLoss
~~~

Examples:
- injury;
- fire;
- uncontrolled motion;
- unsafe charging;
- property damage;
- loss of critical stop/brake function.

Loss is outcome-oriented, not a component failure mode.

---

## 4. SafetyHazard

Add:

~~~text
SafetyHazard
~~~

A system condition that can lead to a SafetyLoss under relevant environment/context.

Binds:
- Loss refs;
- Target/Variant;
- operating context;
- severity/risk;
- assumptions;
- TraceLinks;
- status.

---

## 5. UnsafeControlAction

For control-intensive products:

~~~text
UnsafeControlAction
~~~

Covers:
- required action omitted;
- unsafe action provided;
- action too early/late/out of sequence;
- action applied too long/stopped too soon.

This captures software/timing/control-loop hazards that component FMEA alone may miss.

---

## 6. LossScenario

Add:

~~~text
LossScenario
~~~

Explains causal path through:
- controller/process model;
- sensor/feedback;
- actuator;
- interface/timing;
- environment;
- component failure;
- security ThreatScenario where overlapping.

LossScenario is evidence/rationale input, not Release authority.

---

## 7. SafetyConstraint

Add:

~~~text
SafetyConstraint
~~~

Derived from Hazard/LossScenario.

Normally promotes into ordinary Requirement/AC.

Use TraceLinks:

~~~text
Hazard
 -> DERIVES
 -> SafetyConstraint
 -> Requirement / AC
 -> Implementation
 -> Verification Evidence
~~~

No separate safety-requirement authority is introduced.

---

## 8. SafeStateDefinition

Add versioned:

~~~text
SafeStateDefinition
~~~

May specify:
- actuator state;
- braking/drive output;
- power/charging state;
- degraded behavior;
- maximum transition time;
- validity conditions.

Do not assume "power off" or "shutdown" is always safe.

---

## 9. SafetyTransitionEvidence

For safety transitions, bind:
- exact initial state;
- trigger;
- firmware/config;
- SafeStateDefinition;
- measured transition time;
- environment;
- equipment/calibration;
- result.

Uses v39 MeasurementDefinition/Result semantics.

---

## 10. HazardAnalysisReviewReceipt

Safety analysis freshness is explicit.

Triggers include:
- architecture/control change;
- sensor/actuator substitution;
- interface/timing change;
- firmware control logic change;
- battery/charger change;
- Incident;
- ProductVariant change.

Result:
- CURRENT;
- UPDATE_REQUIRED;
- NEW_HAZARD_IDENTIFIED;
- INCONCLUSIVE.

---

## 11. FMEA / Threat Model relationship

Keep complementary:

~~~text
FMEA
  component failure modes/effects

STPA/System Safety
  hazardous control interaction/timing/feedback

Threat Model
  adversarial/security causes
~~~

They may converge on the same SafetyConstraint/Requirement.

No duplicate Requirement ownership.

---

## 12. Assurance integration

For high-risk/A3/A4:

~~~text
SafetyLoss/Hazard
 -> SafetyConstraint
 -> Verification Evidence
 -> optional AssuranceCaseArtifact
 -> independent Review
 -> Decision
~~~

Safety Analysis and Assurance Case remain inputs to normal authority.

---

## 13. Tool strategy

References:
- XSTAMPP/STPA;
- OSATE/AADL Error Model Annex;
- Resolute assurance claims.

No tool is mandatory.

Use only when product complexity/risk justifies it.

---

## 14. M0/M1 strategy

### M0
Freeze:
- SafetyAnalysisArtifact;
- SafetyLoss;
- SafetyHazard;
- UnsafeControlAction;
- LossScenario;
- SafetyConstraint;
- SafeStateDefinition;
- SafetyTransitionEvidence;
- HazardAnalysisReviewReceipt.

Create one synthetic motor/charger safety fixture.

### M1
No STPA/OSATE/XSTAMPP dependency.

---

## 15. Final rules added by v40

1. System safety is distinct from cybersecurity and component-centric failure analysis.
2. Hazards describe unsafe system conditions, not only component failure.
3. Unsafe control action semantics include omission, timing, order and duration.
4. Safety constraints ultimately enter normal Requirement/Verification authority.
5. Hazard analysis is revision/freshness bound.
6. Safe state is explicit and context-dependent.
7. Safety transition/timing uses calibrated Measurement Evidence.
8. FMEA/STPA/Threat Modeling are complementary.
9. Safety Evidence may span formal, simulation, HIL and physical methods according to fidelity policy.
10. Safety analysis never self-authorizes Release.

Architecture v1.2 remains canonical.
Profile v40 is the current implementation companion.
