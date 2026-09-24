# Open Source Reference Review — Round 35: Threat Modeling and Security-by-Design

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- OWASP Threat Dragon
- OWASP pytm
- OWASP Threat Modeling Playbook
- MITRE ATT&CK STIX data
- OWASP ASVS
- existing ProductSecurityCase, TraceLink, Compliance and Knowledge freshness semantics

## 1. Key conclusion

Product-security response after release is not enough.

Security-by-design requires a pre-release chain:

~~~text
Architecture / Data Flows
 -> Threat Model
 -> Threat Scenarios
 -> Security Requirements / Mitigations
 -> Implementation
 -> Security Verification
 -> Residual Risk / Decision
 -> Release
~~~

Threat modeling should remain lightweight and revision-bound, not become a separate security design authority.

---

## 2. ThreatModelArtifact

Add immutable:

~~~text
ThreatModelArtifact
~~~

May include:
- architecture/data-flow model;
- trust boundaries;
- assets;
- actors/external entities;
- entry points;
- data classifications;
- threat catalog references;
- assumptions;
- mitigations;
- content digest;
- tool/version.

Supported formats may include:
- OWASP Threat Dragon model;
- pytm model/report;
- custom structured model.

---

## 3. ThreatScenario

Add:

~~~text
ThreatScenario
~~~

Fields:
- threat ID;
- affected asset/component/data flow;
- attacker/actor assumptions;
- preconditions;
- attack path;
- consequence;
- severity/risk;
- source/catalog refs;
- status;
- mitigations;
- verification refs.

Threat scenario is distinct from an observed vulnerability/Incident.

---

## 4. SecurityRequirement

Security mitigations should become normal versioned Requirements/ACs.

Examples:
- authenticated update;
- replay protection;
- secret isolation;
- least-privilege tool access;
- rate limiting;
- secure boot;
- data minimization.

Use TraceLink:

~~~text
ThreatScenario
 -> MITIGATED_BY
 -> Requirement / Control / Design
 -> VERIFIED_BY
 -> Security Evidence
~~~

No parallel "security requirement database" is needed.

---

## 5. ThreatMitigationDecision

Add typed:

~~~text
ThreatMitigationDecision
~~~

Possible dispositions:
- MITIGATE;
- ACCEPT;
- TRANSFER;
- AVOID;
- NOT_APPLICABLE.

Binds:
- ThreatScenario;
- rationale;
- mitigation/Requirement refs;
- residual risk;
- authority;
- review date/effectivity.

AI/tool suggestions do not make the Decision.

---

## 6. Threat-model freshness

Threat model applicability can become stale when:
- architecture/data flow changes;
- Interface Contract changes;
- ToolProfile/MCP server changes;
- external provider changes;
- data classification/purpose changes;
- new Target/Variant;
- new privilege/capability;
- new attack knowledge;
- critical Incident/security Finding.

Add:

~~~text
ThreatModelReviewReceipt
~~~

which binds:
- ThreatModelArtifact;
- current architecture/Target/Interface digests;
- review Procedure;
- reviewer;
- result;
- new/changed threats;
- Evidence.

---

## 7. OWASP pytm practice

pytm is valuable because threat modeling is code-like and can generate:
- data-flow diagrams;
- sequence diagrams;
- applicable threats;
- reports.

Its stale-days feature explicitly recognizes that threat models drift away from implementation.

### Absorb

ThreatModelArtifact should reference source/component revisions, and change detection should be able to trigger re-review.

Do not depend on file modification dates alone; use exact engineering digests/TraceLinks.

---

## 8. Threat Dragon practice

Threat Dragon provides:
- graphical DFD;
- trust boundaries;
- threats;
- mitigations/countermeasures;
- repository/local model storage.

Potential future UI/tooling integration.

Platform authority remains in:
- ThreatModelArtifact snapshot;
- ThreatScenario;
- Requirements;
- Evidence;
- Risk/Decision.

---

## 9. Threat catalogs

External catalogs such as MITRE ATT&CK may provide:
- tactics/techniques;
- ICS/mobile/enterprise threat knowledge;
- machine-readable STIX releases.

Use them as:
- versioned Reference/Knowledge artifacts;
- threat-generation inputs.

Catalog membership does not imply a threat applies to every product.

---

## 10. Security verification baseline

OWASP ASVS illustrates the value of:
- versioned security requirements;
- stable requirement identifiers;
- multiple verification levels.

For applicable software/web/cloud components, ASVS-like control sets can become ComplianceFrameworkArtifact/ComplianceProfile inputs.

Embedded-specific security controls may use product/company profiles instead.

---

## 11. SecurityVerificationEvidence

Security tests remain normal Evidence:
- penetration test;
- fuzzing;
- static analysis;
- auth/permission tests;
- cryptographic/update tests;
- secure-boot/attestation tests;
- protocol abuse tests.

Each Evidence item traces back to ThreatScenario/SecurityRequirement where possible.

---

## 12. Residual risk

A threat can remain partially mitigated.

Use existing Risk/Decision semantics to record:
- residual likelihood/impact;
- assumptions;
- compensating controls;
- expiry/review;
- release applicability.

"Mitigation implemented" does not automatically mean risk eliminated.

---

## 13. ProductSecurityCase relationship

Keep pre-release and post-release distinct:

~~~text
ThreatModel
  design-time expected threats

ProductSecurityCase
  actual discovered vulnerability/security issue lifecycle
~~~

A ProductSecurityCase may reveal:
- missing ThreatScenario;
- ineffective mitigation;
- changed threat assumptions.

It can trigger ThreatModelReviewReceipt and new Security Requirements.

---

## 14. AI/MCP threat modeling

ToolProfile and AI Runtime introduce threat classes such as:
- prompt/tool-output injection;
- confused deputy;
- tool privilege escalation;
- secret exfiltration;
- malicious MCP server/schema/description;
- poisoned context/knowledge;
- provider data leakage.

These threats should be modeled explicitly for the platform itself and qualified in Runtime/Reviewer security corpus.

---

## 15. M0/M1 impact

M0 reserves:
- ThreatModelArtifact;
- ThreatScenario;
- ThreatMitigationDecision;
- ThreatModelReviewReceipt.

M1 should create one threat model for engineering-platform's own:
- Control Plane;
- Worker;
- Session Gateway;
- Action Gateway;
- Artifact/Evidence paths;
- Tool/MCP boundary.

No Threat Dragon/pytm service is required.

---

## 16. M2/M3 impact

For products:
- threat-model OTA/device identity;
- local network/cloud/app interfaces;
- camera/audio/data paths;
- physical/debug interfaces;
- manufacturing/provisioning path.

---

## 17. Invariants

1. Threat modeling is design-time security analysis, distinct from PSIRT/vulnerability response.
2. Threat scenarios trace to normal Security Requirements and Verification Evidence.
3. Threat mitigations require typed Decisions and residual-risk handling.
4. Threat models are revision-bound and can become stale.
5. Architecture/interface/tool/data-flow changes trigger threat-model impact analysis.
6. Threat catalogs are reference inputs, not applicability authority.
7. Security tests remain ordinary Evidence under trusted Procedures.
8. Implemented mitigation does not imply zero residual risk.
9. Post-release security cases feed back into threat-model review.
10. AI/MCP/runtime tool boundaries are part of the platform threat model.

## 18. Conclusion

The durable security-by-design rule is:

> **Every material trust boundary or data flow should have an explicit threat model whose mitigations trace to requirements and verification, and whose applicability is re-checked when the architecture changes.**
