# Open Source Reference Review — Round 37: Laboratory Samples and Cross-Enterprise Traceability

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- SENAITE LIMS
- Eclipse Tractus-X Trace-X
- Eclipse Tractus-X Item Relationship Service
- Eclipse Tractus-X Digital Twin Registry
- existing CertificationSample, DeviceInstance, ProductStructureSnapshot, ManufacturingResultReceipt and TraceLink semantics

## 1. Key conclusion

Laboratory sample identity and cross-enterprise supply-chain traceability are valuable, but they should not create a second engineering authority.

The durable additions are:
- explicit physical sample identity and custody;
- external laboratory execution references;
- cross-enterprise part/batch/serial trace events as imported observations/projections;
- part genealogy projections over immutable engineering/manufacturing facts.

---

## 2. SampleInstance

Add:

~~~text
SampleInstance
~~~

Represents a physical sample used for:
- certification;
- material/component qualification;
- battery testing;
- incoming inspection;
- destructive analysis;
- reliability/stress testing;
- supplier validation.

Fields:
- sample_id;
- sample type;
- source Device/Part/Lot/ProductStructure refs;
- collection/preparation Procedure;
- collected_at;
- collector/station;
- quantity/mass/count;
- condition/state;
- storage requirements;
- current custody;
- disposition;
- labels/external IDs.

A SampleInstance is not necessarily a full DeviceInstance.

---

## 3. SamplePreparationReceipt

Sample preparation can change what is tested.

Add:

~~~text
SamplePreparationReceipt
~~~

Examples:
- cut section;
- depopulated PCB;
- prepared cell;
- conditioned battery;
- potted sample;
- environmental preconditioning;
- extracted material.

Receipt binds:
- source sample;
- Procedure;
- station/operator;
- inputs/conditions;
- output sample identity;
- timestamps;
- Evidence/attachments.

Destructive preparation creates a new SampleInstance or terminal disposition as appropriate.

---

## 4. SampleCustodyReceipt

Add append-only:

~~~text
SampleCustodyReceipt
~~~

Records:
- SampleInstance;
- from/to custodian/location;
- timestamp;
- reason;
- seal/package state;
- transport/storage conditions where relevant;
- external courier/lab reference;
- handler identity;
- anomalies/damage.

This supports chain of custody for certification, failure analysis and external labs.

---

## 5. ExternalLaboratoryRef

Add lightweight:

~~~text
ExternalLaboratoryRef
~~~

Fields:
- lab/provider;
- project/order/case ID;
- sample IDs;
- test request ID;
- report ID;
- external status;
- accreditation/scope metadata refs where supplied;
- timestamps/locators.

External LIMS owns its own scheduling/sample workflow.
engineering-platform owns product/sample/evidence applicability.

---

## 6. LaboratoryResultImportReceipt

When importing external lab results:

~~~text
LaboratoryResultImportReceipt
~~~

Binds:
- external case/report;
- SampleInstance;
- exact Procedure/standard edition where known;
- report Artifact;
- raw data attachments;
- mapping/conversion version;
- imported Evidence;
- importer/validator.

The importer must not silently infer tested product configuration from a report filename.

---

## 7. Sample-to-product applicability

A laboratory sample may represent:
- one DeviceInstance;
- one material lot;
- one supplier part;
- one Target/Variant;
- one ProductStructure revision.

Applicability is explicit.

If sample construction differs materially from production target:
- CertificationDeltaAssessment / ImpactAnalysis applies;
- Evidence does not silently generalize.

---

## 8. SENAITE practice

SENAITE demonstrates the value of a dedicated LIMS for enterprise lab workflows.

Decision:
- do not implement LIMS scheduling/results UI in engineering-platform;
- keep SampleInstance/custody/result references interoperable;
- integrate with SENAITE or another LIMS only when lab scale warrants it.

---

## 9. Cross-enterprise part traceability

Tractus-X Trace-X models:
- serialized parts;
- batches;
- BoM AsPlanned;
- BoM AsBuilt;
- usage/relationship;
- quality alerts/investigations.

### Absorb

Introduce imported:

~~~text
SupplyChainTraceEvent
~~~

or external reference/projection for:
- manufactured part;
- supplier part;
- batch/lot;
- parent-child relation;
- shipment/receipt;
- quality notification.

These events describe supply-chain observations.

They do not rewrite internal ProductStructureSnapshot or ManufacturingResultReceipt.

---

## 10. PartGenealogyProjection

Generate:

~~~text
PartGenealogyProjection
~~~

from:
- ProductStructureSnapshot;
- ManufacturingResultReceipt;
- RepairActionReceipt;
- supplier/lot records;
- imported SupplyChainTraceEvents.

Queries:
- which devices contain this supplier lot?
- which parent assemblies contain this serialized part?
- where did this returned part originate?
- which customers/fleet cohort may be affected?

It is a read/impact model.

---

## 11. As-planned vs as-built

Keep explicit:

~~~text
AsPlanned
  ProductStructureSnapshot / ProductionRecipe

AsBuilt
  ManufacturingResultReceipt / actual lot/serial substitutions

External supplier AsBuilt
  imported trace event / external twin relationship
~~~

Do not overwrite nominal BOM to reflect one field/manufacturing substitution.

---

## 12. Supplier quality propagation

Imported cross-company quality alerts can create:
- SupplierChangeNotice;
- QualityCase;
- Incident;
- ImpactAnalysis;
- quarantine/containment.

The external alert is a source signal.
Internal typed Decision controls effect.

---

## 13. Data sovereignty / external trust

Cross-company trace systems may enforce:
- data-sharing policy;
- partner identity;
- access rules;
- data contracts.

engineering-platform stores:
- external provider/partner ref;
- policy/contract ref;
- received snapshot/event digest;
- trust/provenance.

External network authorization is not local Release authority.

---

## 14. M0/M1 impact

M0 reserves:
- SampleInstance;
- SamplePreparationReceipt;
- SampleCustodyReceipt;
- ExternalLaboratoryRef;
- LaboratoryResultImportReceipt;
- SupplyChainTraceEvent;
- PartGenealogyProjection schema.

M1 requires no LIMS or Catena-X integration.

---

## 15. M2/M3 impact

Adopt when needed:
- external certification/battery lab;
- supplier material qualification;
- destructive RMA analysis;
- cross-tier supplier lot traceability;
- quality-alert propagation.

---

## 16. Invariants

1. Physical sample identity is distinct from Device/Product identity.
2. Sample preparation and custody are append-only, auditable events.
3. External lab workflow is referenced, not duplicated.
4. Imported lab Evidence binds exact sample and report identity.
5. Sample evidence does not silently generalize to unmatched product configurations.
6. External supply-chain events are observations, not internal BOM authority.
7. As-planned and as-built structures remain distinct.
8. Part genealogy is a derived read/impact model.
9. Supplier quality alerts trigger internal typed analysis/Decisions rather than direct mutation.
10. Cross-enterprise trust/data-sharing policy remains separate from engineering Release authority.

## 17. Conclusion

The durable lab/supply-chain rule is:

> **Know exactly which physical sample or serialized/batch part produced each piece of evidence, preserve every custody/preparation change, and treat cross-enterprise trace data as provenance-rich observations rather than as a new source of engineering truth.**
