# Reference-Aligned Implementation Profile v35

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V29.md

Research basis:
- Open Source Reference Review Rounds 1–35
- Architecture v1.2

## 1. Goal

Profile v35 adds five lifecycle capabilities without increasing M1 service count:

1. product certification / conformity and battery qualification;
2. privacy and engineering-data lifecycle;
3. field anomaly/prognostics and predictive maintenance;
4. long-term compatibility/version-skew/support policy;
5. threat modeling and security-by-design.

---

# Part A — Product Certification / Conformity

## 2. ConformityAssessmentCase

Add:

~~~text
ConformityAssessmentCase
~~~

Binds:
- product/variant/Target scope;
- market/jurisdiction;
- applicable ComplianceProfile/standards;
- assessment route;
- external lab/body refs;
- evidence/test status;
- reports/certificates/declarations;
- applicability/effectivity.

Certification is exact-scope/revision bound.

---

## 3. Certification artifacts

Add:
- CertificationRequirementSet;
- ConformityAssessmentPlan;
- CertificationTestReportArtifact;
- CertificateArtifact;
- DeclarationOfConformityArtifact;
- TechnicalDocumentationPackage;
- CertificationSample;
- CertificationDeltaAssessment.

Engineering Change can trigger delta assessment:
- NO_IMPACT;
- DOCUMENT_UPDATE;
- PARTIAL_RETEST;
- FULL_REASSESSMENT;
- EXTERNAL_AUTHORITY_REVIEW;
- INCONCLUSIVE.

---

## 4. Battery qualification

Battery/cell/pack evidence binds exact:
- cell/pack design;
- supplier/model/lot as relevant;
- BMS/protection;
- firmware/config;
- sample;
- standard edition;
- conditioning/state;
- Procedure;
- report/Evidence.

Battery simulation uses ModelArtifact.
Simulation never replaces required physical conformity/safety Evidence.

---

# Part B — Privacy / Data Lifecycle

## 5. DataAssetProfile

Add:

~~~text
DataAssetProfile
~~~

for:
- telemetry;
- logs;
- audio/video;
- RMA data;
- prompts/tool outputs;
- datasets;
- diagnostic/security data.

Contains:
- sensitivity;
- owner;
- allowed processing purposes;
- export/provider restrictions;
- retention;
- redaction/de-identification policy.

---

## 6. DataProcessingActivity

Add:

~~~text
DataProcessingActivity
~~~

Binds:
- data asset;
- purpose;
- processing steps;
- provider/tool/runtime;
- recipients/destinations;
- storage;
- retention;
- output refs;
- policy version.

Data availability does not imply all-purpose usage permission.

---

## 7. Retention / privacy transforms

Add:
- DataRetentionPolicy;
- DataDispositionReceipt;
- SensitiveDataFinding;
- PrivacyTransformReceipt;
- optional PrivacyRiskAssessmentArtifact.

Data disposition may be:
- DELETE;
- SECURE_ERASE;
- ANONYMIZE;
- ARCHIVE;
- RETAIN_ON_HOLD.

Privacy transforms are immutable provenance-bearing transforms.

---

## 8. AI/RMA privacy

ToolProfile/TelemetryProfile/DataProcessingActivity govern:
- external provider transfer;
- prompt/source retention;
- full tool-output capture;
- sensitive-data redaction;
- data-location restrictions.

RMA/debug is an explicit processing purpose, not a blanket right to extract user data.

---

# Part C — Field Prognostics

## 9. TelemetryWindowArtifact

Analysis consumes exact immutable windows:

~~~text
TelemetryWindowArtifact
~~~

with:
- population/device;
- channels;
- time range;
- sampling/aggregation;
- missing-data rules;
- source;
- quality/freshness;
- digest.

Stream storage may remain external.

---

## 10. Health/prognostic records

Add:
- HealthModelArtifact;
- HealthStateEstimate;
- PrognosticEstimate;
- MaintenanceRecommendation;
- MaintenanceActionReceipt.

Anomaly uses normalized Finding categories.

Every estimate includes uncertainty/confidence and exact model/input identity.

---

## 11. Maintenance authority

Prognostic output is proposal/evidence input.

Maintenance/derating/quarantine uses normal Decision/policy/Human Authority.

Service/RMA outcomes feed ground truth back into:
- model qualification;
- threshold calibration;
- failure replay.

Historical predictions are never rewritten after outcome is known.

---

# Part D — Long-Term Compatibility

## 12. CompatibilityEnvelope

Add:

~~~text
CompatibilityEnvelope
~~~

Defines valid multi-component combinations:
- component versions;
- interface revisions;
- allowed skew;
- Target/variant predicates;
- unsupported sets;
- upgrade-order constraints;
- deprecation dates.

This is broader than pairwise interface compatibility.

---

## 13. Support-policy records

Add:
- VersionSkewPolicy;
- UpgradeOrderConstraint;
- ProductSupportPolicy;
- CrossVersionQualificationEvidence;
- optional ConformanceProfile.

Technical compatibility and current support status are separate.

---

## 14. Multi-generation fleet

Supported targeting uses:
- SupportedVariantSet;
- CompatibilityEnvelope;
- ProductSupportPolicy;
- actual Device component versions;
- SystemUpdateManifest.

Runtime negotiation can be Evidence but cannot bypass unsupported policy.

---

# Part E — Threat Modeling / Security-by-Design

## 15. ThreatModelArtifact

Add immutable:

~~~text
ThreatModelArtifact
~~~

May contain:
- architecture/data flows;
- trust boundaries;
- assets/data classes;
- actors/entry points;
- threat references;
- assumptions;
- mitigations;
- tool/version/digest.

---

## 16. ThreatScenario / mitigation

Add:
- ThreatScenario;
- ThreatMitigationDecision;
- ThreatModelReviewReceipt.

ThreatScenario traces to normal:
- Requirement;
- Security control;
- implementation;
- Verification Evidence;
- Risk/Decision.

No parallel security-requirement database is introduced.

---

## 17. Threat-model freshness

Threat-model review can be triggered by:
- architecture/data-flow change;
- Interface Contract change;
- ToolProfile/MCP change;
- provider/data processing change;
- new Target/Variant;
- new privilege;
- security Incident/Finding.

A stale threat model cannot silently satisfy high-assurance security gates.

---

# Part F — Milestone Strategy

## 18. M0 reserves

Add/freeze:
- ConformityAssessmentCase
- CertificationRequirementSet
- ConformityAssessmentPlan
- CertificationTestReportArtifact
- CertificateArtifact
- DeclarationOfConformityArtifact
- TechnicalDocumentationPackage
- CertificationDeltaAssessment
- CertificationSample
- DataAssetProfile
- DataProcessingActivity
- DataRetentionPolicy
- DataDispositionReceipt
- SensitiveDataFinding
- PrivacyTransformReceipt
- TelemetryWindowArtifact
- HealthModelArtifact
- HealthStateEstimate
- PrognosticEstimate
- MaintenanceRecommendation
- MaintenanceActionReceipt
- CompatibilityEnvelope
- VersionSkewPolicy
- UpgradeOrderConstraint
- ProductSupportPolicy
- CrossVersionQualificationEvidence
- ConformanceProfile
- ThreatModelArtifact
- ThreatScenario
- ThreatMitigationDecision
- ThreatModelReviewReceipt

Synthetic fixtures only.

---

## 19. M1

No new mandatory services:
- no certification platform;
- no privacy suite;
- no prognostics service;
- no compatibility server;
- no threat-modeling server.

M1 should create one platform ThreatModelArtifact and privacy-aware data policy defaults.

---

## 20. M2/M3

Adopt as actual needs emerge:
- certification report/lab imports;
- battery/charging qualification;
- PII detection/redaction;
- fleet health analytics;
- cross-version support matrices;
- product threat models.

---

## 21. Final rules added by v35

1. Conformity evidence binds exact product/variant/sample/standard edition.
2. Engineering changes trigger explicit certification applicability assessment.
3. Sensitive data use is purpose/policy scoped.
4. Retention/deletion/redaction produce auditable receipts.
5. Predictive-maintenance outputs are uncertainty-bearing Evidence inputs, not autonomous authority.
6. Multi-component support uses explicit compatibility envelopes and directional version-skew policy.
7. Upgrade order is part of compatibility, not an implementation detail.
8. Technical compatibility and support status are distinct.
9. Threat modeling is design-time analysis distinct from PSIRT.
10. Threat scenarios trace to normal Requirements and Verification.
11. Threat models are revision/freshness bound.
12. Simulation/privacy/prognostic/threat tools remain mechanisms below the same authority model.

Architecture v1.2 remains canonical.
Profile v35 is the current implementation companion.
