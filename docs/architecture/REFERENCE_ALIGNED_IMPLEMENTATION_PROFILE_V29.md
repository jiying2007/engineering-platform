# Reference-Aligned Implementation Profile v29

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V25.md

Research basis:
- Open Source Reference Review Rounds 1–29
- Architecture v1.2

## 1. Goal

Profile v29 adds four durable lifecycle capabilities:

1. supplier/component lifecycle, alternate-part qualification and CAPA;
2. machine-readable compliance/assessment;
3. Digital Twin / observed device-state semantics;
4. third-party/FOSS license compliance.

No new mandatory M1 runtime service is introduced.

---

# Part A — Supplier / Component / CAPA

## 2. ComponentLifecycleRecord

Add:

~~~text
ComponentLifecycleRecord
~~~

Binds:
- internal Part;
- ManufacturerPartRef;
- lifecycle state;
- source/provider;
- effective dates;
- observed_at;
- source document refs.

States:
- ACTIVE
- NRND
- EOL_ANNOUNCED
- LAST_TIME_BUY
- OBSOLETE
- UNKNOWN

Lifecycle is provenance/time bound.

---

## 3. SupplierChangeNotice

Add immutable:

~~~text
SupplierChangeNotice
~~~

For:
- PCN;
- PDN;
- EOL;
- process/site/material/package change.

Notice is source Evidence/input.
It does not mutate Target/ProductStructure automatically.

---

## 4. AlternatePartQualificationDecision

Distinguish:
- purchasing substitute;
- engineering-qualified alternate.

Decision binds:
- original/alternate part;
- affected ProductStructure line;
- scope;
- comparison/Verification Evidence;
- lot/supplier restrictions;
- effectivity;
- authority.

Results:
- APPROVED_EQUIVALENT;
- APPROVED_RESTRICTED;
- REJECTED;
- INCONCLUSIVE.

---

## 5. QualityCase / CAPA

Add:
- QualityCase
- CAPAPlan
- CAPAEffectivenessEvidence

Sources:
- non-conformance;
- supplier defect;
- incoming inspection;
- manufacturing alert;
- RMA population issue;
- repeated HIL failure.

CAPA distinguishes:
- containment;
- correction;
- corrective action;
- preventive action.

Closure requires effectiveness Evidence.

---

## 6. MaterialComplianceArtifact

Immutable compliance documents may include:
- RoHS;
- REACH/SVHC;
- conflict minerals;
- material composition;
- supplier declaration;
- environmental certificate.

Bind exact:
- part;
- revision;
- supplier/manufacturer;
- lot where applicable;
- standard/version;
- validity.

---

## 7. Material lot state

Optional:

~~~text
MaterialLotStatus
~~~

States:
- ACCEPTED;
- HOLD;
- QUARANTINED;
- REJECTED;
- CONSUMED;
- UNKNOWN.

A generic approved part can still have a bad/quarantined lot.

---

# Part B — Machine-Readable Compliance

## 8. ComplianceFrameworkArtifact

Add immutable:

~~~text
ComplianceFrameworkArtifact
~~~

Represents exact version of:
- regulation/control catalog;
- security baseline;
- company standard;
- industry framework.

OSCAL-compatible forms are preferred where suitable.

---

## 9. ComplianceProfile

Tailors applicable controls to:
- product;
- Target;
- market/jurisdiction;
- project;
- Assurance Profile;
- support lifecycle.

Applicability is explicit/versioned.

---

## 10. ControlImplementationStatement

Per control:
- implementation description;
- owner;
- scope;
- related Procedures;
- Evidence refs;
- limitations/status.

An implementation statement is not proof.

---

## 11. ComplianceAssessment

Add:
- ComplianceAssessmentPlan
- ComplianceAssessmentResult
- ComplianceFinding
- CompliancePackageArtifact

Assessment reuses existing Evidence/Review/independence semantics.

Evidence is referenced, not duplicated.

---

## 12. Compliance projection

A CompliancePackageArtifact can export:
- framework/profile;
- implementation statements;
- Evidence index;
- assessment results;
- findings;
- remediation;
- decisions/signatures.

It is archive/interchange, not mutable business authority.

---

# Part C — Digital Twin / Observed State

## 13. ObservedDeviceState

Immutable observation record:

~~~text
ObservedDeviceState
~~~

Includes:
- DeviceInstance;
- observed_at;
- component versions/digests;
- configuration/flags;
- health/connectivity;
- trust/provenance;
- source provider;
- raw telemetry refs.

---

## 14. DeviceTwinProjection

Derived read model:

~~~text
DeviceTwinProjection
~~~

Combines:
- desired Release/config;
- latest trusted observed state;
- health;
- trust/attestation summary;
- freshness;
- drift status.

States may include:
- IN_SYNC;
- DRIFTED;
- STALE;
- UNKNOWN;
- UNREACHABLE;
- PARTIAL.

---

## 15. Twin trust/freshness

Last-known state is never assumed current.

Each observation has:
- source;
- observed time;
- freshness;
- trust class.

High-risk decisions may require attestation/provider receipt beyond self-report.

---

## 16. Twin command boundary

Commands still flow:

~~~text
Control Plane policy
 -> Action Gateway
 -> Device/Fleet backend
 -> actual observed state
 -> reconciliation
~~~

Twin mutation/desired-property update is an execution mechanism, not authority.

---

# Part D — FOSS / Third-Party License Compliance

## 17. ThirdPartyComponentRecord

Normalized third-party component:
- package/version;
- source/provenance;
- dependency path;
- declared/detected license;
- copyright;
- upstream/supplier;
- BOM refs.

---

## 18. LicenseFinding

Scanner fact:
- exact subject;
- detected license/copyright;
- scanner/version;
- location/component;
- ambiguity/confidence;
- raw report.

Finding is not legal/release Decision.

---

## 19. LicensePolicy / DistributionProfile

License obligations depend on distribution/use context.

Add:
- LicensePolicy
- DistributionProfile

Contexts may include:
- internal;
- SaaS/server;
- binary distribution;
- embedded device shipment;
- SDK/source;
- container.

---

## 20. LicenseComplianceAssessment

Binds:
- Release/Artifact/BOM;
- ThirdPartyComponentRecord set;
- LicensePolicy;
- DistributionProfile;
- Findings;
- exceptions;
- result.

Results:
- PASS;
- CONDITIONAL;
- FAIL;
- INCONCLUSIVE.

---

## 21. Shipment obligations

Add:
- NoticeArtifact
- SourceOfferPackageArtifact
- LicenseExceptionDecision

These bind exact Release/component set.

Notice/source-offer generated for one Release cannot silently apply to another.

---

## 22. Tool ecosystem

Potential implementations:
- FOSSology;
- ScanCode Toolkit;
- OSS Review Toolkit;
- SPDX/CycloneDX.

Strategy:
- scanners discover facts;
- policy evaluates facts;
- Release Admission checks required obligations;
- tools never inherit Human/legal authority.

---

# Part E — Milestone Strategy

## 23. M0 reserves

Add/freeze:
- ComponentLifecycleRecord
- SupplierChangeNotice
- AlternatePartQualificationDecision
- QualityCase
- CAPAPlan
- CAPAEffectivenessEvidence
- MaterialComplianceArtifact
- MaterialLotStatus
- ComplianceFrameworkArtifact
- ComplianceProfile
- ControlImplementationStatement
- ComplianceAssessmentPlan/Result
- CompliancePackageArtifact
- ObservedDeviceState
- DeviceTwinProjection schema
- ThirdPartyComponentRecord
- LicenseFinding
- LicensePolicy
- DistributionProfile
- LicenseComplianceAssessment
- NoticeArtifact
- SourceOfferPackageArtifact
- LicenseExceptionDecision

Synthetic fixtures only.

---

## 24. M1

No new services:
- no QMS;
- no component-intelligence provider;
- no OSCAL platform;
- no Digital Twin service;
- no FOSSology/ORT.

Schemas must simply remain extensible.

---

## 25. M2/M3

Adopt as needed:
- supplier lifecycle/PCN feed;
- CAPA workflow integration;
- compliance assessment/export;
- fleet/device twin backend;
- FOSS scanning/notice/source-offer pipeline.

---

## 26. Final rules added by v29

1. Supplier/manufacturer/part/lot identities are distinct.
2. Supplier notices trigger impact analysis; they do not silently mutate engineering baselines.
3. Purchasing substitutes are not engineering alternates until qualified.
4. CAPA closure requires effectiveness Evidence.
5. Material compliance is exact-part/revision/scope bound.
6. Compliance controls, implementation and assessment are machine-readable but do not form a second authority universe.
7. Evidence revocation propagates into compliance assessments.
8. Digital Twin is a freshness-bound read model, not Release authority.
9. Desired state and observed state remain distinct and reconciled.
10. License scans produce Findings; distribution-aware policy produces compliance assessment.
11. Notice/source-offer obligations bind exact shipped Release.
12. External QMS/compliance/twin/license tools remain mechanisms/projections behind the platform authority model.

Architecture v1.2 remains canonical.
Profile v29 is the current implementation companion.
