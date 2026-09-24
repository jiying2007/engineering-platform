# Public / Regulatory Reference Review — Round 38: Digital Product Passport and Cross-Lifecycle Product Data

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- EU Ecodesign for Sustainable Products Regulation / Digital Product Passport
- EU DPP Registry and 2026 harmonised DPP standards
- Eclipse Tractus-X traceability/digital twin practices
- existing ProductStructure, Variant, Compliance, Repair, Material and Device lifecycle semantics

## 1. Key conclusion

A Digital Product Passport should be treated as an external interoperable product-information projection, not as the internal source of engineering truth.

The durable path is:

~~~text
Authoritative engineering/product records
 -> DPP projection
 -> signed/versioned DPP Artifact
 -> external Registry/identifier registration
 -> registration/publication receipt
~~~

---

## 2. DigitalProductPassportArtifact

Add optional immutable:

~~~text
DigitalProductPassportArtifact
~~~

May include, according to applicable product rules:
- product/variant identity;
- unique identifiers;
- manufacturer/economic-operator refs;
- ProductStructure/material information;
- compliance/certification refs;
- repairability/service information;
- spare-parts/service refs;
- safety/use/end-of-life information;
- environmental/sustainability information;
- support/security lifecycle information where required;
- exact source-record references;
- schema/profile version;
- content digest.

The exact contents are product/jurisdiction specific.

---

## 3. DPPProfile

Add versioned:

~~~text
DPPProfile
~~~

Defines:
- applicable regulation/product group;
- schema/standard versions;
- mandatory/optional fields;
- access-control classes;
- identifier/data-carrier rules;
- retention/persistence;
- source mapping;
- publication/registration requirements.

This avoids hard-coding one current EU DPP schema into generic domain logic.

---

## 4. DPPProjectionReceipt

Generate:

~~~text
DPPProjectionReceipt
~~~

Binds:
- exact source object versions/digests;
- DPPProfile;
- projector/version;
- generated DPP Artifact;
- validation result;
- omissions/exceptions.

DPP data can therefore be regenerated and audited from underlying facts.

---

## 5. DPPRegistrationReceipt

External registry publication/registration creates:

~~~text
DPPRegistrationReceipt
~~~

Fields:
- DPP Artifact digest;
- registry/jurisdiction;
- unique product/DPP identifier;
- registration time;
- submitter/economic operator;
- external receipt/status;
- API/provider ref;
- supersession/update refs.

Registry status does not mutate internal engineering history.

---

## 6. DPP is not ProductStructure authority

Keep separate:

~~~text
ProductStructureSnapshot
  internal engineering BOM/design baseline

ManufacturingResultReceipt
  actual as-built serial

DigitalProductPassportArtifact
  external lifecycle information projection
~~~

DPP may include BOM/material information but cannot rewrite the source structure.

---

## 7. DPP updates

Product information can change over lifecycle:
- certification;
- repair information;
- support status;
- security advisory/support window;
- material/compliance data;
- end-of-life information.

Do not mutate old published Artifact.

Create:
- new DPP Artifact/revision;
- DPPProjectionReceipt;
- external update/supersession receipt.

Historical registered versions remain traceable.

---

## 8. Access levels

Some DPP information may be public; other information may be restricted to:
- authorities;
- repairers;
- recyclers;
- supply-chain actors;
- service providers.

DPPProfile should classify fields by access category.

Do not export confidential engineering details merely because they exist internally.

---

## 9. Data minimization / privacy

DPP export follows DataAssetProfile / Privacy policy.

Personal data, secrets and internal-only engineering details are excluded unless an applicable requirement and policy allow inclusion.

DPP is not a dump of the internal knowledge graph.

---

## 10. Repair/RMA relationship

DPP may expose approved:
- repair instructions;
- service information;
- replacement/spare-part data;
- end-of-life information.

Technical RMA history for an individual customer device remains controlled by privacy/service policy.

Do not publish unrestricted per-device service history unless explicitly required and permitted.

---

## 11. Cross-enterprise traceability

Catena-X/Tractus-X style part genealogy can supply provenance inputs for DPP fields.

External supplier trace data remains:
- provenance-rich observation;
- partner-controlled data;
- policy/contract scoped.

It does not automatically become internally authoritative product data.

---

## 12. Current EU implementation note

As of 2026-09-24, the European Commission has launched the DPP Registry and testing environment.

The Commission has also published harmonised DPP standards covering areas such as:
- data exchange protocols;
- unique identifiers;
- data carriers;
- storage/persistence;
- APIs;
- interoperability.

Product-specific DPP obligations and timelines remain regulation/product dependent.

These requirements belong in DPPProfile / Compliance policy rather than hard-coded platform logic.

Official references:
- https://single-market-economy.ec.europa.eu/single-market/digital-product-passport_en
- https://single-market-economy.ec.europa.eu/single-market/digital-product-passport/dpp-registry_en
- https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:32026D1736

---

## 13. DPP and conformity

DPP publication may reference:
- CertificateArtifact;
- DeclarationOfConformityArtifact;
- MaterialComplianceArtifact;
- SecurityAdvisoryArtifact;
- ComplianceAssessmentResult.

It does not create those facts.

If source certification/compliance becomes invalid, ImpactAnalysis determines whether DPP update is required.

---

## 14. DPP provider integration

Future adapter may support:
- EU Registry;
- DPP service provider;
- enterprise/industry data space.

Adapter only handles:
- validation;
- registration;
- update;
- receipt retrieval.

Engineering-platform owns projection/source mapping.

---

## 15. M0/M1 impact

M0 reserves:
- DigitalProductPassportArtifact;
- DPPProfile;
- DPPProjectionReceipt;
- DPPRegistrationReceipt.

M1 requires no DPP integration.

---

## 16. M2/M3 impact

When product scope requires:
- choose exact applicable DPPProfile;
- map internal product facts;
- validate/export/register;
- track updates/supersession.

---

## 17. Invariants

1. DPP is a projection/export, not internal engineering authority.
2. DPP contents map to exact source record versions.
3. Product-specific DPP requirements live in versioned profiles/policy.
4. DPP updates create new immutable artifacts/receipts rather than rewriting history.
5. Access classification prevents accidental confidential-data publication.
6. DPP privacy obligations reuse existing DataAsset/Processing policy.
7. External Registry status is reconciled but cannot rewrite engineering facts.
8. ProductStructure and as-built receipts remain canonical internal sources.
9. Cross-enterprise trace data is provenance input, not automatic authority.
10. Certification/compliance changes can trigger DPP impact/update workflow.

## 18. Conclusion

The durable DPP rule is:

> **Build product passports from authoritative lifecycle facts, never make the passport itself the place where those facts are invented or maintained.**
