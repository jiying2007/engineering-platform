# Open Source Reference Review — Round 29: Open-Source License and Third-Party Compliance

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- FOSSology
- OSS Review Toolkit
- ScanCode Toolkit
- SPDX
- OpenChain reference practices
- earlier SBOM/BOM/Compliance models

## 1. Key conclusion

License/open-source compliance needs its own evidence and policy semantics, but should reuse:
- BOM/SBOM;
- Finding;
- ComplianceProfile;
- Evidence;
- Release Admission.

A scanner output is not a legal conclusion or release approval.

---

## 2. ThirdPartyComponentRecord

Add normalized component record:

~~~text
ThirdPartyComponentRecord
~~~

Fields:
- package/component identity;
- version;
- source/provenance;
- source repository/archive;
- declared/detected licenses;
- copyrights;
- supplier/upstream;
- dependency path;
- Artifact/BOM refs.

This may be projected from SPDX/CycloneDX/ORT/ScanCode/FOSSology results.

---

## 3. LicenseFinding

Add normalized:

~~~text
LicenseFinding
~~~

Binds:
- exact source/package/Artifact;
- detected license expression;
- scanner/tool/version;
- file/range or package;
- copyright holder;
- confidence/ambiguity;
- raw report.

Finding is a factual scan result, not a legal policy decision.

---

## 4. LicensePolicy

Add versioned:

~~~text
LicensePolicy
~~~

Defines:
- allowed/restricted/prohibited licenses;
- context/distribution type;
- linking/combination assumptions;
- source-offer requirements;
- notice/attribution rules;
- approval/escalation;
- company exceptions.

Policy interpretation should involve designated legal/compliance authority where necessary.

---

## 5. LicenseComplianceAssessment

Add:

~~~text
LicenseComplianceAssessment
~~~

Binds:
- Release/Artifact/BOM subject;
- ThirdPartyComponentRecord set;
- LicensePolicy;
- Findings;
- evaluator/tool/version;
- exceptions/Decisions;
- result.

Results:
- PASS;
- CONDITIONAL;
- FAIL;
- INCONCLUSIVE.

---

## 6. NoticeArtifact

Add immutable:

~~~text
NoticeArtifact
~~~

May contain:
- third-party notices;
- attribution;
- copyright statements;
- license text bundle;
- generated README/NOTICE.

Binds exact Release/component set.

A notice generated for Release A is not automatically correct for Release B.

---

## 7. SourceOfferPackageArtifact

For licenses/distribution modes that require source offer/source provision:

~~~text
SourceOfferPackageArtifact
~~~

Binds:
- binary/distribution Release;
- exact corresponding source;
- build scripts/patches where required;
- third-party source archives;
- offer text/process refs;
- content digest.

ORT's ability to create source archives is a useful reference.

---

## 8. Distribution context matters

License obligations may differ for:
- internal tooling;
- SaaS/server use;
- binary distribution;
- embedded device shipment;
- SDK/source distribution;
- container image.

Therefore assessment binds a:

~~~text
DistributionProfile
~~~

rather than treating license policy as package-only.

---

## 9. Exceptions are typed Decisions

Legal/compliance exceptions must be explicit:

~~~text
LicenseExceptionDecision
~~~

Binds:
- exact component/license finding;
- exact distribution/release scope;
- authority;
- rationale;
- validity/effectivity.

No hidden allowlist entry in a scanner config should become release authority.

---

## 10. SPDX / CycloneDX

Use standard BOM formats for:
- component identity;
- licenses;
- relationships;
- metadata exchange.

SPDX is particularly strong for license/compliance semantics.
CycloneDX remains useful for broader software/hardware/security BOM.

Platform may retain internal normalized records and export both.

---

## 11. FOSSology / ScanCode

Strong candidates for deep license/copyright detection.

They produce:
- raw scan artifacts;
- LicenseFinding;
- copyright/origin metadata.

They do not own Release approval.

---

## 12. ORT

ORT demonstrates a mature pipeline:

~~~text
Analyze
 -> Download
 -> Scan
 -> Advise
 -> Evaluate
 -> Report
~~~

This strongly matches:
- facts;
- policy evaluation;
- reports/notices.

ORT can become a future implementation/integration candidate for license-compliance automation.

---

## 13. OpenChain

OpenChain practices emphasize that software-compliance quality is a process capability:
- policy;
- roles;
- training;
- supplier communication;
- SBOM quality;
- repeatable workflow.

### Absorb

License compliance should not be only a CI scanner gate.
ComplianceProfile may include organizational/process controls in addition to per-Release checks.

---

## 14. Release Admission

Production/distribution Release Admission may require:
- current SBOM;
- LicenseComplianceAssessment PASS/CONDITIONAL;
- required NoticeArtifact;
- required SourceOfferPackageArtifact;
- approved exceptions;
- no unresolved prohibited license Finding.

This is policy-driven.

---

## 15. Dependency changes

Dependency update can change:
- version;
- license;
- transitive dependencies;
- source location;
- notice obligations.

Therefore dependency lock/change triggers:
- BOM refresh;
- license scan;
- assessment refresh.

"Same package name" is not sufficient reuse.

---

## 16. Supplier binaries / SDKs

Vendor SDKs, prebuilt libraries and blobs may have:
- proprietary license;
- redistribution limits;
- missing source;
- unknown provenance.

Record them explicitly as ThirdPartyComponentRecord/Artifact.

Do not let "vendor SDK" bypass BOM/license inventory.

---

## 17. Generated / AI-produced code

AI-generated code provenance alone does not establish license status.

If generated content may reproduce third-party material:
- normal source scan/review policy still applies;
- provenance/AI usage is additional context;
- Release compliance relies on actual source/component findings and company policy.

---

## 18. M0/M1 impact

M0 reserves:
- ThirdPartyComponentRecord;
- LicenseFinding;
- LicensePolicy;
- LicenseComplianceAssessment;
- NoticeArtifact;
- SourceOfferPackageArtifact;
- DistributionProfile;
- LicenseExceptionDecision.

M1 does not require FOSSology/ORT/ScanCode.

---

## 19. M2/M3 impact

For distribution/production:
- generate SBOM;
- scan licenses/copyrights;
- evaluate policy;
- generate notices/source offer;
- gate Release Admission.

ORT/FOSSology/ScanCode are candidates according to organizational need.

---

## 20. Invariants

1. License scanner output is a Finding, not legal/release authority.
2. License obligations bind exact component/version and distribution context.
3. Release-specific Notice/Attribution artifacts are immutable.
4. Source-offer packages bind exact shipped binaries/Release.
5. Compliance exceptions are explicit scoped Decisions.
6. Dependency updates trigger compliance re-evaluation.
7. Vendor binaries/SDKs remain visible third-party components.
8. SPDX/CycloneDX are interchange, not mutable authority.
9. AI-generated source remains subject to normal provenance/license policy.
10. Release Admission can require license-compliance Evidence without delegating authority to scanner tools.

## 21. Conclusion

The durable FOSS-compliance rule is:

> **Discover facts with scanners, evaluate them under an explicit distribution-aware policy, and ship the exact notices/source obligations with the exact Release.**
