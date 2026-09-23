# Open Source Reference Review — Round 26: Supplier Change, Component Lifecycle, CAPA and Material Compliance

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- InvenTree manufacturer/supplier/substitute BOM semantics
- ERPNext Non Conformance / Quality Review / Quality Action
- Odoo Quality Alerts
- CycloneDX HBOM
- earlier ProductStructureSnapshot / EngineeringChangePackage / Effectivity rules

## 1. Key conclusion

Component/supplier lifecycle risk should not be hidden in email, spreadsheets or procurement notes.

engineering-platform needs enough typed records to convert supplier/material changes into:
- impact analysis;
- alternate-part qualification;
- Engineering Change;
- Verification;
- manufacturing effectivity.

It should not replace procurement/QMS/component intelligence platforms.

---

## 2. ComponentLifecycleRecord

Add:

~~~text
ComponentLifecycleRecord
~~~

Binds:
- internal Part/Component identity;
- manufacturer part number;
- manufacturer;
- lifecycle state;
- source/provider;
- observed_at;
- effective dates;
- evidence/document refs.

Suggested states:
- ACTIVE;
- NRND;
- EOL_ANNOUNCED;
- LAST_TIME_BUY;
- OBSOLETE;
- UNKNOWN.

State is source/provenance-bound and may differ between suppliers/providers.

---

## 3. SupplierChangeNotice

Add:

~~~text
SupplierChangeNotice
~~~

For PCN/PDN/EOL or similar external change notice.

Fields:
- supplier/manufacturer;
- MPN/part refs;
- notice ID/type;
- published/effective dates;
- affected lots/sites/processes/materials/packages;
- description;
- source document Artifact;
- recommended action;
- confidentiality;
- status.

Notice is source evidence, not automatic Engineering Change authority.

---

## 4. Alternate/Substitute parts

InvenTree demonstrates explicit substitute BOM line items.

Platform should distinguish:

~~~text
CandidateAlternate
QualifiedAlternate
ApprovedSubstitute
~~~

A purchasing substitute is not automatically an engineering-equivalent part.

---

## 5. AlternatePartQualificationDecision

Add:

~~~text
AlternatePartQualificationDecision
~~~

Binds:
- original part revision;
- alternate manufacturer/part revision;
- exact affected BOM/ProductStructure lines;
- dimensional/electrical/thermal/software/interface comparison;
- supplier/lot restrictions;
- Verification Evidence;
- effectivity;
- authority;
- result.

Results:
- APPROVED_EQUIVALENT;
- APPROVED_RESTRICTED;
- REJECTED;
- INCONCLUSIVE.

---

## 6. Substitution scope

Approval may be limited by:
- SKU/variant;
- hardware revision;
- factory;
- serial/lot range;
- product region;
- firmware version;
- operating environment.

Do not treat an alternate as globally equivalent unless explicitly proven.

---

## 7. Supplier change impact

SupplierChangeNotice triggers candidate traversal through:
- ProductStructureSnapshot;
- approved substitutes;
- TargetRevision;
- manufactured populations;
- open ProductionRecipeRevision;
- fleet population;
- RMA/Incident history.

ImpactAnalysisDecision determines:
- no impact;
- documentation only;
- sample qualification;
- full engineering requalification;
- stop-build/quarantine;
- last-time-buy/business action.

---

## 8. QualityCase

Add generic engineering quality case:

~~~text
QualityCase
~~~

Sources:
- Non-Conformance;
- manufacturing quality alert;
- incoming inspection;
- HIL failure pattern;
- RMA population issue;
- supplier defect.

Fields:
- affected part/product/population;
- source records;
- severity;
- owner;
- containment;
- root cause refs;
- corrective/preventive actions;
- effectiveness verification;
- closure.

External QMS may remain authority for corporate quality process; platform stores engineering linkage/receipts.

---

## 9. CAPAPlan

Add:

~~~text
CAPAPlan
~~~

Contains:
- QualityCase;
- containment;
- correction;
- corrective action;
- preventive action;
- owners/dates;
- affected engineering objects;
- required Verification;
- effectiveness criteria.

Corrective and preventive actions are distinct.

---

## 10. CAPA effectiveness

CAPA is not closed when actions are merely completed.

Require:

~~~text
CAPAEffectivenessEvidence
~~~

Example:
- repeated manufacturing runs;
- failure-rate reduction;
- updated test coverage;
- RMA trend;
- incoming quality results;
- supplier process confirmation.

Closure cites Evidence and Decision.

---

## 11. Containment versus permanent fix

Keep distinct:
- CONTAINMENT: temporary risk reduction;
- CORRECTION: fix current nonconforming units;
- CORRECTIVE_ACTION: remove root cause;
- PREVENTIVE_ACTION: reduce recurrence/related risks.

This prevents a temporary screening step from being mistaken for final resolution.

---

## 12. MaterialComplianceArtifact

Add immutable:

~~~text
MaterialComplianceArtifact
~~~

Kinds may include:
- RoHS declaration;
- REACH/SVHC declaration;
- conflict-mineral declaration;
- material composition;
- supplier compliance certificate;
- environmental/regulatory certificate;
- CUSTOM.

Binds:
- part/supplier/lot/revision;
- standard/regulation/version;
- valid/effective period;
- issuer;
- document digest.

---

## 13. HBOM relationship

CycloneDX HBOM can be used as an interoperable hardware composition Artifact.

Recommended mapping:

~~~text
ProductStructureSnapshot
 -> authoritative engineering structure

HBOMArtifact
 -> interoperable hardware composition export/projection

MaterialComplianceArtifact
 -> supplier/regulatory proof attached to components
~~~

HBOM does not replace engineering BOM authority.

---

## 14. Supplier/Manufacturer identity

Manufacturer and Supplier are distinct.

One MPN may be:
- made by one manufacturer;
- sold by multiple suppliers.

Supplier commercial availability and manufacturer technical identity should not be collapsed.

Use typed:
- ManufacturerRef;
- SupplierRef;
- ManufacturerPartRef;
- SupplierPartRef.

---

## 15. Lot-specific quality

A component may be approved generally but a specific lot can be quarantined.

Add optional:

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

Manufacturing receipt links actual lot where traceability is required.

---

## 16. M0/M1 impact

M0 reserves:
- ComponentLifecycleRecord;
- SupplierChangeNotice;
- AlternatePartQualificationDecision;
- QualityCase;
- CAPAPlan;
- CAPAEffectivenessEvidence;
- MaterialComplianceArtifact;
- MaterialLotStatus.

M1 needs no supplier/QMS platform.

---

## 17. M2/M3 impact

For real hardware production:
- ingest PCN/EOL notices from chosen component data source;
- qualify alternates;
- connect supplier lots to ManufacturingResultReceipt where needed;
- use QualityCase/CAPA for repeated production/RMA issues;
- export HBOM/compliance packages.

---

## 18. Invariants

1. Supplier availability and manufacturer technical identity are distinct.
2. Supplier notice is evidence/input, not automatic Engineering Change.
3. Substitute purchasing permission is not engineering equivalence.
4. Alternate-part qualification is scope/effectivity limited.
5. Component lifecycle state is provenance/time bound.
6. CAPA closure requires effectiveness Evidence, not task completion alone.
7. Containment and permanent corrective action remain distinct.
8. Material compliance declarations are immutable scoped Artifacts.
9. Specific lots can be quarantined independently of generic part approval.
10. Supplier/component changes traverse ProductStructure/Target/Device populations through explicit Impact Analysis.

## 19. Conclusion

The durable supplier-quality rule is:

> **Every material change or alternate-part decision that can alter product behavior must become an explicit, scoped engineering decision with effectivity and Verification—not an invisible purchasing substitution.**
