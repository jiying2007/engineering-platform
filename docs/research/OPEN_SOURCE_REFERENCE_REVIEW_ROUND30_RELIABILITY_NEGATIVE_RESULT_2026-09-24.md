# Open Source Reference Review — Round 30: Reliability / FMEA / FRACAS Landscape

Date: 2026-09-24
Status: **Archived negative-result research note**

## 1. Search scope

Targeted searches covered:
- FMEA / FMEDA;
- FRACAS;
- HALT / HASS;
- reliability growth;
- Weibull/life-data analysis;
- open-source reliability engineering platforms.

## 2. Result

No mature open-source platform was identified that meets the same adoption bar used for:
- Temporal;
- labgrid;
- OSCAL;
- OPA;
- in-toto/Sigstore;
- mature CI/test systems.

The search returned mostly small/special-purpose analysis repositories rather than a broadly proven lifecycle system.

## 3. Architecture decision

Do **not** add a new Reliability Plane, FRACAS service or FMEA-specific mandatory aggregate now.

Existing platform concepts already cover the essential lifecycle:

~~~text
Reliability Requirement / Acceptance Criterion
 -> Verification Plan
 -> Procedure
 -> TestVariant / environment / mission profile
 -> statistical Raw/Evaluation Evidence
 -> Verification
 -> Finding / Incident
 -> FailureAnalysisRecord
 -> QualityCase / CAPA
 -> EngineeringChangePackage
~~~

## 4. Optional future artifact types

Only when a real reliability program needs them, consider lightweight Artifact types:
- MissionProfileArtifact;
- FMEAArtifact;
- ReliabilityAnalysisArtifact;
- LifeDataArtifact.

These would remain inputs/evidence under existing Verification/Assurance semantics.

## 5. Reliability statistics

Weibull/life-distribution/reliability-growth tooling can be selected as analysis libraries per Procedure.

Tool choice does not justify a separate authority model.

Statistical Evidence must still bind:
- exact population/sample;
- censoring rules;
- mission/environment;
- test duration/cycles;
- method/model;
- confidence bounds;
- tool/version.

## 6. HALT/HASS

HALT/HASS-style environmental/stress workflows can be represented as normal versioned Procedures using:
- chamber/fixture resource identities;
- VerificationEnvironmentDefinition;
- stress profile;
- measurements;
- failure capture;
- DeviceStateSnapshot.

No separate workflow engine is required.

## 7. FRACAS

If field/factory failure volume later warrants a formal FRACAS view, build it as a projection over:
- Finding;
- Incident;
- RMA;
- FailureAnalysisRecord;
- CAPA;
- FixVerification;
- reliability metrics.

Do not duplicate those records into another source of truth.

## 8. Decision

No canonical implementation-profile change is required from Round 30.

This is intentional:

> **A search result that does not reveal a missing invariant, useful standard, or mature reusable mechanism should not expand the platform.**
