# Open Source Reference Review — Round 27: Machine-Readable Compliance and Assessment Evidence

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- NIST OSCAL
- OpenControl / Compliance Masonry
- earlier CRA/PSIRT, AssuranceCase, TraceLink, Evidence and Release models

## 1. Key conclusion

Compliance should be represented as a structured mapping from:
- requirement/control;
- implementation statement;
- Evidence;
- assessment;
- finding;
- remediation/decision.

It should not become a second verification universe.

---

## 2. ComplianceFrameworkArtifact

Add immutable:

~~~text
ComplianceFrameworkArtifact
~~~

Represents:
- regulation/standard/control catalog;
- version;
- jurisdiction/scope;
- source;
- content digest;
- effective dates.

Examples:
- OSCAL catalog/profile;
- CRA-derived internal control profile;
- ISO/industry control mapping;
- company secure-development baseline.

The platform does not claim official legal conformance merely by importing a framework.

---

## 3. ComplianceProfile

Add:

~~~text
ComplianceProfile
~~~

Selects/tailors applicable controls for:
- product family;
- Target;
- organization/project;
- market/jurisdiction;
- Assurance Profile;
- support lifecycle.

References exact ComplianceFrameworkArtifact version.

This is analogous to OSCAL profile/tailoring concepts.

---

## 4. ControlImplementationStatement

Add:

~~~text
ControlImplementationStatement
~~~

For each applicable control:
- control ID;
- product/system scope;
- implementation description;
- responsible owner;
- relevant architecture/domain objects;
- supporting Procedures;
- Evidence refs;
- limitations;
- status;
- version.

Statement describes how a control is implemented.

It does not by itself prove effectiveness.

---

## 5. ComplianceAssessmentPlan

Add:

~~~text
ComplianceAssessmentPlan
~~~

Defines:
- controls to assess;
- Procedures;
- sampling;
- environments;
- assessor independence;
- required Evidence;
- timing/frequency;
- acceptance rules.

Can reuse existing Verification Plan semantics where appropriate.

---

## 6. ComplianceAssessmentResult

Add:

~~~text
ComplianceAssessmentResult
~~~

Binds:
- ComplianceProfile;
- exact assessment plan;
- assessor/issuer;
- subject scope;
- Evidence set;
- control-by-control results;
- findings;
- date;
- validity/review window.

Results may include:
- SATISFIED;
- PARTIALLY_SATISFIED;
- NOT_SATISFIED;
- NOT_APPLICABLE;
- INCONCLUSIVE.

---

## 7. ComplianceFinding

A compliance gap is a typed Finding.

Examples:
- missing secure-development control;
- unsupported product in required security support window;
- missing SBOM/VEX process;
- stale vulnerability-response Procedure;
- insufficient review independence.

Finding can generate:
- Risk;
- Work;
- CAPA;
- ProductSecurityCase;
- Policy update.

---

## 8. Compliance evidence reuse

Existing Evidence may satisfy multiple controls.

Do not copy it.

Use TraceLink / typed refs:

~~~text
Evidence
 -> supports Control A
 -> supports Control B
~~~

If Evidence becomes STALE/REVOKED:
- dependent assessment applicability changes;
- compliance projection updates.

---

## 9. CompliancePackageArtifact

Add optional export:

~~~text
CompliancePackageArtifact
~~~

Contains immutable references/snapshots:
- framework/profile;
- implementation statements;
- assessment results;
- Evidence index;
- findings;
- remediation status;
- decisions;
- signatures;
- audit metadata.

OSCAL-like serialization can be used where it fits.

This is an export/archive package, not the mutable authority database.

---

## 10. Regulation policy stays externalized

Regulatory details change.

Use versioned policy/configuration:
- deadlines;
- applicability;
- markets;
- required controls;
- reporting channels.

Do not hard-code current CRA/other legal rules inside generic domain code.

---

## 11. Compliance and Assurance Case

For high-assurance work:

~~~text
Compliance controls
 -> Evidence / Assessment
 -> optional AssuranceCaseArtifact
 -> Human/independent review
~~~

Compliance status and engineering Release decision remain separate.

A product can satisfy an engineering test while still missing a regulatory control.

---

## 12. Audit/assessor independence

Assessment policy may require:
- independent team;
- external auditor;
- designated security role;
- evidence issuer separation.

Use existing Review/Authority/independence mechanisms.

---

## 13. Continuous compliance

Some controls can be continuously evaluated:
- SBOM present/current;
- vulnerability scan current;
- required reviews complete;
- signing keys valid;
- support window active;
- Knowledge/Runbook fresh.

Others remain periodic/manual.

Continuous checks produce Evidence/Findings, not silent self-certification.

---

## 14. OSCAL practice

OSCAL's key value is layered machine-readable representation for:
- control catalogs;
- profiles/tailoring;
- implementation descriptions;
- assessment plans/results.

### Decision

Prefer OSCAL-compatible export/import when useful for security/compliance programs.

Do not force every engineering Requirement or Evidence into OSCAL.

---

## 15. M0/M1 impact

M0 reserves:
- ComplianceFrameworkArtifact;
- ComplianceProfile;
- ControlImplementationStatement;
- ComplianceAssessmentPlan;
- ComplianceAssessmentResult;
- CompliancePackageArtifact.

M1 requires no compliance platform.

---

## 16. M2/M3 impact

As product/regulatory needs mature:
- map engineering Evidence to controls;
- automate selected continuous controls;
- export audit packages;
- connect PSIRT/CRA/security lifecycle.

---

## 17. Invariants

1. Imported regulation/control catalog is versioned and immutable.
2. Applicability/tailoring is explicit in ComplianceProfile.
3. Implementation statements are not proof by themselves.
4. Assessments cite exact Evidence.
5. Compliance findings become normal typed engineering work/risk inputs.
6. Evidence revocation propagates to dependent compliance assessments.
7. Legal/regulatory rules are policy/configuration, not generic hard-coded state logic.
8. Continuous checks produce Evidence/Findings, not automatic certification.
9. Compliance packages are export/archive projections.
10. Compliance status and Release Authority remain distinct.

## 18. Conclusion

The durable compliance rule is:

> **Represent controls, implementation, evidence and assessment explicitly so compliance can be audited and regenerated from engineering facts—without creating a parallel source of truth.**
