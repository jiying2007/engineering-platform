# Open Source Reference Review — Round 22: PLM, Hardware Design and Engineering Change

Date: 2026-09-24
Status: **Archived research / implementation input**

References reviewed:
- Odoo PLM / Engineering Change Orders
- InvenTree Parts / BOM / Part Revisions / Build Orders
- Part-DB
- KiCad
- KiBot
- LibrePCB

## 1. Key conclusion

engineering-platform should not become a full PLM, CAD/PDM or inventory system.

It should own the immutable engineering identities and change/evidence chain that PLM/PDM/manufacturing systems reference.

---

## 2. ExternalPLMRef

Add a lightweight reference:

~~~text
ExternalPLMRef
  provider
  product/part ID
  revision
  ECO/ECN/ECR ID
  BOM/revision ID
  external URL/locator
  observed_at
~~~

External PLM is a lifecycle system of record for its own domain, not Engineering Verification/Release authority.

---

## 3. HardwareDesignArtifact

Use Artifact specialization for:
- schematic source/snapshot;
- PCB layout;
- symbol/footprint/library snapshot;
- mechanical/STEP;
- assembly drawing;
- BOM;
- fabrication data;
- pick-and-place;
- stencil/panel output;
- test-point/netlist data.

Every artifact is content-addressed and tied to exact EDA/tool version where relevant.

---

## 4. EDA source vs manufacturing outputs

KiBot demonstrates a strong pattern:

~~~text
KiCad source
 -> scripted/repeatable export
 -> fabrication/documentation outputs
~~~

Add:

~~~text
DesignExportReceipt
~~~

which binds:
- exact EDA source digests;
- exporter/tool version;
- export config;
- output roles;
- output digests;
- warnings/errors.

Manufacturing never relies on "whatever files were manually exported last".

---

## 5. Hardware checks as Evidence

EDA automation may produce:
- ERC result;
- DRC result;
- BOM consistency;
- footprint/library checks;
- manufacturing output validation.

These become Evidence when executed by trusted Procedure/Issuer.

A clean ERC/DRC does not itself approve a hardware release.

---

## 6. ProductStructureSnapshot

Add immutable:

~~~text
ProductStructureSnapshot
~~~

Represents an exact product/BOM structure:
- assembly/part revisions;
- quantities;
- reference designators;
- approved alternates where applicable;
- subassemblies;
- hardware design refs;
- effective scope;
- content digest.

It may be imported/projected from:
- InvenTree;
- ERP/PLM;
- Part-DB;
- other BOM systems.

---

## 7. Revision semantics

InvenTree's practice is valuable:
- a new revision is a distinct entity;
- historical build/stock/order references remain bound to the old revision.

Apply the same rule:

> Never mutate a hardware/part revision in place once used by Formal engineering/manufacturing.

---

## 8. EngineeringChangePackage

Odoo ECO practice shows the value of:
- proposed revision separate from production revision;
- explicit comparison;
- verification/approval before apply;
- effectivity.

Add immutable:

~~~text
EngineeringChangePackage
~~~

Fields:
- change ID/type;
- old Target/ProductStructure refs;
- proposed new refs;
- changed HardwareDesignArtifacts;
- BOM delta;
- Interface changes;
- reason/rationale;
- affected SKUs/Targets;
- verification requirements;
- impact analysis;
- approvals/decisions;
- effectivity;
- status.

---

## 9. Change states

Suggested:

~~~text
PROPOSED
 -> ANALYZED
 -> REVISION_READY
 -> VERIFYING
 -> APPROVED
 -> EFFECTIVE
    | REJECTED
    | CANCELLED
    | SUPERSEDED
~~~

"Approved" and "effective in production" are distinct.

---

## 10. Effectivity

Add:

~~~text
EffectivityRule
~~~

Possible scopes:
- date/time;
- serial range;
- lot/batch;
- factory/site;
- SKU/variant;
- supplier/material;
- production order.

A hardware change may be approved before it becomes effective on a particular production cohort.

---

## 11. Engineering baseline

TargetRevision should bind:
- exact ProductStructureSnapshot;
- exact HardwareDesignArtifact revisions;
- exact Interface Contract revisions;
- behavior-affecting configuration.

This creates a complete "as-designed" baseline.

ManufacturingResultReceipt later records the "as-built" device.

---

## 12. As-designed vs as-built

Keep distinct:

~~~text
AsDesigned
  TargetRevision + ProductStructureSnapshot

AsBuilt
  per DeviceInstance manufacturing/material/configuration receipt
~~~

Substitution/rework in manufacturing can cause as-built to differ from nominal design and must be traceable.

---

## 13. PLM change does not automatically invalidate everything

EngineeringChangePackage feeds Impact Analysis via:
- TraceLinks;
- product structure;
- interface contracts;
- variant model;
- existing Verification.

ImpactAnalysisDecision determines:
- unaffected;
- partial requalification;
- full requalification;
- coordinated software/hardware release.

---

## 14. M0/M1 impact

M0 reserves:
- ExternalPLMRef;
- HardwareDesignArtifact kinds;
- DesignExportReceipt;
- ProductStructureSnapshot;
- EngineeringChangePackage;
- EffectivityRule.

M1 does not require PLM, KiCad or KiBot integration.

---

## 15. M2/M3 impact

For hardware-heavy projects:
- automate KiCad/KiBot export;
- ingest exact BOM/design snapshots;
- tie TargetRevision to hardware baseline;
- integrate PLM/ERP by typed references.

---

## 16. Invariants

1. PLM/PDM identity is referenced, not silently copied into engineering authority.
2. Hardware design revisions used formally are immutable.
3. Manufacturing outputs are reproducibly exported from exact design sources.
4. ERC/DRC/manufacturing checks are Evidence, not release authority.
5. Product structure/BOM is content-addressed and revisioned.
6. Engineering approval and production effectivity are separate.
7. Hardware changes are subject to explicit Impact Analysis.
8. As-designed and as-built states are distinct and traceable.

## 17. Conclusion

The durable boundary is:

> **PLM manages product lifecycle records; engineering-platform proves exactly which hardware/design/BOM revision was engineered, verified and allowed to become effective.**
