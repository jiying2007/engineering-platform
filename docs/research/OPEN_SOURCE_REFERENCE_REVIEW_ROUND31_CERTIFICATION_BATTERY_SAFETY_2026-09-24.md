# Open Source / Regulatory Reference Review — Round 31: Product Certification and Battery Safety Qualification

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- EU CE marking / manufacturer conformity-assessment guidance
- UNECE Manual of Tests and Criteria, section 38.3 for lithium cells/batteries
- PyBaMM
- existing Verification, Compliance, ModelArtifact, Test/Procedure and Manufacturing semantics

## 1. Key conclusion

Product certification should not be represented as a checkbox or uploaded PDF.

The durable chain is:

~~~text
Applicable regulation/standard
 -> product/variant scope
 -> Conformity Assessment Plan
 -> exact test/design Evidence
 -> external/internal assessment
 -> certificate/declaration/report
 -> applicability/effectivity
 -> change-impact / re-certification
~~~

engineering-platform should manage traceability and evidence applicability, while notified bodies, labs and regulatory systems remain external authorities for their roles.

---

## 2. ConformityAssessmentCase

Add:

~~~text
ConformityAssessmentCase
~~~

Fields:
- case ID;
- product family/variant/Target scope;
- market/jurisdiction;
- applicable ComplianceProfile / standards;
- assessment route;
- responsible owner;
- external lab/notified-body refs;
- technical-file refs;
- test/assessment status;
- certificates/declarations;
- effectivity;
- expiry/review where applicable;
- current applicability status.

Suggested states:
- PLANNING;
- EVIDENCE_GATHERING;
- TESTING;
- EXTERNAL_REVIEW;
- CONFORMANT;
- NONCONFORMANT;
- SUSPENDED;
- SUPERSEDED;
- EXPIRED.

---

## 3. CertificationRequirementSet

Add immutable:

~~~text
CertificationRequirementSet
~~~

Binds:
- regulation/directive/standard refs;
- exact editions/amendments;
- essential requirements/control IDs;
- product applicability rationale;
- harmonized/alternative standards used;
- market/jurisdiction;
- content digest.

This can reuse ComplianceFrameworkArtifact/ComplianceProfile underneath.

---

## 4. ConformityAssessmentPlan

Specialized plan may reuse ComplianceAssessmentPlan and Verification Plan.

It defines:
- exact product/variant/revision;
- required tests/analyses;
- sample configuration;
- test laboratories;
- procedures/standards;
- acceptance criteria;
- document outputs;
- assessor independence;
- required witness/signature.

The plan is versioned.

---

## 5. CertificationTestReportArtifact

Add Artifact specialization:

~~~text
CertificationTestReportArtifact
~~~

Binds:
- exact tested sample/DeviceInstance or representative sample;
- TargetRevision / ProductVariantDefinition;
- hardware/software/configuration;
- laboratory/station identity;
- calibration status;
- procedure/standard edition;
- raw Evidence refs;
- report digest;
- issuer/signature.

A PDF filename alone is not enough.

---

## 6. CertificateArtifact and DeclarationOfConformityArtifact

Add:
- CertificateArtifact
- DeclarationOfConformityArtifact

Metadata:
- issuer;
- product/model scope;
- regulation/standard;
- certificate/declaration number;
- issue date;
- validity/status;
- covered revisions/variants;
- supporting assessment case;
- signed document digest.

These are external/legal/compliance artifacts, not generic engineering Release decisions.

---

## 7. TechnicalDocumentationPackage

CE-style conformity practice strongly depends on technical documentation.

Add optional:

~~~text
TechnicalDocumentationPackage
~~~

which may index:
- product description;
- requirements/standards;
- risk analysis;
- design drawings/schematics;
- ProductStructureSnapshot;
- firmware/software Release;
- test reports;
- security/SBOM/VEX records;
- instructions/labels;
- declaration/certificate refs.

Package is a snapshot/export over authoritative records.

Do not manually duplicate facts into a second database.

---

## 8. Certification applicability after change

A certificate/test report applies only within its declared product/configuration scope.

Add:

~~~text
CertificationDeltaAssessment
~~~

Triggered by:
- EngineeringChangePackage;
- alternate part;
- component supplier/process change;
- firmware/security change;
- radio/Wi-Fi module change;
- enclosure/mechanical change;
- battery/cell change;
- calibration/config change.

Result:
- NO_IMPACT;
- DOCUMENT_UPDATE;
- PARTIAL_RETEST;
- FULL_REASSESSMENT;
- EXTERNAL_AUTHORITY_REVIEW;
- INCONCLUSIVE.

This is a typed Decision with evidence.

---

## 9. Representative sample identity

Certification often tests representative samples rather than every manufactured unit.

Represent:

~~~text
CertificationSample
~~~

with:
- DeviceInstance/sample ID;
- exact as-built ProductStructureSnapshot;
- component lots where material;
- firmware/config/calibration;
- deviations from nominal Target;
- sample selection rationale.

Evidence from one sample does not automatically apply to later variants without scope/equivalence.

---

## 10. EU CE practice

Current EU manufacturer guidance explicitly places responsibility on the manufacturer to:
- identify applicable EU legislation/standards;
- perform conformity assessment;
- prepare technical documentation;
- issue the EU declaration of conformity;
- affix CE marking.

This validates modeling technical documentation and conformity assessment as engineering lifecycle artifacts rather than a final checkbox.

Official reference:
https://single-market-economy.ec.europa.eu/single-market/goods/ce-marking/manufacturers_en

---

## 11. Battery transport qualification

UNECE Manual of Tests and Criteria section 38.3 defines transport-related test requirements for lithium cells/batteries and is actively maintained.

Official reference:
https://unece.org/transport/dangerous-goods/rev8-files

### Platform implication

Battery qualification records must bind:
- exact cell/battery design and revision;
- cell supplier/model/lot where relevant;
- protection electronics/BMS;
- mechanical construction;
- sample identity;
- test sequence/standard edition;
- state of charge/conditioning;
- raw measurements/report;
- result.

A supplier's certificate for a different pack/design is not automatically applicable.

---

## 12. BatteryModelArtifact

PyBaMM demonstrates a mature open-source battery-modeling approach with explicit:
- model;
- parameters;
- experiment definition;
- solver/configuration.

Use existing:

~~~text
ModelArtifact
~~~

for battery electrochemical/thermal models.

Model/simulation Evidence can support design/diagnosis but does not replace required physical certification/safety tests.

---

## 13. Battery operating/stress profile

For battery/charging/thermal tests, Procedure/Environment should freeze:
- charge/discharge profile;
- voltage/current limits;
- thermal conditions;
- rest periods;
- cycle count;
- load profile;
- fault injection;
- charger/power-supply profile.

No new workflow engine is needed.

Where useful, store the reusable profile as a TestProfile/Model Artifact referenced by ProcedureRevision.

---

## 14. Thermal/charging safety

Safety-sensitive properties may require:
- overcharge/overcurrent behavior;
- external/internal short behavior where applicable;
- charger compatibility;
- temperature monitoring/cutoff;
- abnormal charging;
- thermal recovery;
- power-path behavior;
- pack/BMS/firmware interaction.

The exact required test set depends on applicable standards/product/market.

Do not hard-code standards into generic domain code.

---

## 15. Software changes and certification

A software/firmware change can affect certified behavior even if hardware is unchanged.

EngineeringChangePackage must allow CertificationDeltaAssessment to inspect:
- charging limits;
- protection thresholds;
- RF behavior;
- safety control;
- user-visible instructions;
- cybersecurity requirements.

Certification applicability is therefore not hardware-only.

---

## 16. External laboratory identity

Reuse Station/Issuer concepts but support:

~~~text
ExternalAssessmentProviderRef
~~~

for:
- accredited laboratory;
- notified body;
- certification body;
- external assessor.

Store:
- external report/certificate refs;
- provider identity;
- scope/accreditation metadata where available;
- signatures/receipts.

The platform does not self-declare external accreditation.

---

## 17. M0/M1 impact

M0 reserves:
- ConformityAssessmentCase;
- CertificationRequirementSet;
- ConformityAssessmentPlan;
- CertificationTestReportArtifact;
- CertificateArtifact;
- DeclarationOfConformityArtifact;
- TechnicalDocumentationPackage;
- CertificationDeltaAssessment;
- CertificationSample.

M1 requires no certification platform or lab integration.

---

## 18. M2/M3 impact

When product certification begins:
- bind actual certification samples;
- import test reports/certificates;
- generate technical-documentation package;
- connect Engineering Change to recertification impact;
- track market/variant applicability.

---

## 19. Invariants

1. Certification is scope/revision/variant bound, not a product-name checkbox.
2. Technical documentation is generated from authoritative engineering facts.
3. External certificate/report identity and signature are preserved.
4. Representative-sample evidence has explicit applicability scope.
5. Engineering changes trigger certification delta assessment.
6. Battery supplier certificates are not blindly reusable across changed pack/BMS/configuration.
7. Simulation/model evidence does not replace required physical certification tests.
8. Standards/editions are versioned inputs.
9. Software/configuration changes may affect conformity scope.
10. Compliance/conformity status and Release Authority remain distinct.

## 20. Conclusion

The durable certification rule is:

> **Every conformity claim must be traceable to the exact product configuration, standards edition, representative sample and evidence package that justified it, and every material engineering change must explicitly assess whether that claim still applies.**
