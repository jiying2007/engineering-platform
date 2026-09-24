# Reference-Aligned Implementation Profile v38

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V36.md

Research basis:
- Open Source Reference Review Rounds 1–38
- Architecture v1.2

## 1. Goal

Profile v38 retains all v36 semantics and adds:

1. physical Sample identity / preparation / chain-of-custody;
2. external laboratory result import;
3. cross-enterprise part genealogy as an observation/read model;
4. Digital Product Passport as an immutable external projection.

No new mandatory M1 runtime service is introduced.

---

# Part A — Laboratory Samples

## 2. SampleInstance

Add:

~~~text
SampleInstance
~~~

Represents a physical sample distinct from DeviceInstance.

Use cases:
- certification;
- supplier/material qualification;
- battery testing;
- destructive analysis;
- incoming inspection;
- environmental/reliability tests.

Fields:
- sample identity/type;
- source Device/Part/Lot/ProductStructure;
- collection/preparation refs;
- quantity/state;
- collection time;
- custodian/location;
- storage requirements;
- external labels/IDs;
- disposition.

---

## 3. SamplePreparationReceipt

Add append-only:

~~~text
SamplePreparationReceipt
~~~

Records:
- source sample;
- preparation Procedure;
- station/operator;
- conditioning/material changes;
- timestamps;
- output sample identity;
- attachments/Evidence.

Destructive or materially changing preparation creates a new sample identity or terminal disposition.

---

## 4. SampleCustodyReceipt

Add:

~~~text
SampleCustodyReceipt
~~~

Records each custody transfer:
- sample;
- from/to custodian/location;
- time;
- purpose;
- package/seal state;
- transport/storage conditions where relevant;
- anomalies;
- external courier/lab reference.

This is important for certification, supplier disputes and destructive RMA analysis.

---

## 5. ExternalLaboratoryRef

Use lightweight:

~~~text
ExternalLaboratoryRef
~~~

for external LIMS/lab workflow:
- provider;
- case/order;
- sample IDs;
- test request/report IDs;
- accreditation/scope refs if provided;
- status/timestamps.

engineering-platform does not replicate lab scheduling.

---

## 6. LaboratoryResultImportReceipt

Imported lab result binds:
- exact SampleInstance;
- external case/report;
- Procedure/standard edition where known;
- report Artifact;
- raw attachments;
- mapping/conversion version;
- imported Evidence;
- importer/validator.

Never infer tested configuration only from filenames/manual description.

---

# Part B — Cross-Enterprise Traceability

## 7. SupplyChainTraceEvent

Add imported observation:

~~~text
SupplyChainTraceEvent
~~~

Potential event types:
- serialized/batch part produced;
- parent-child relationship;
- shipment/receipt;
- supplier batch assignment;
- quality notification;
- usage/as-built relation.

Includes:
- partner/provider;
- external identity;
- event time;
- part/batch/serial refs;
- relationship;
- source payload digest;
- contract/policy/trust refs.

It does not overwrite internal BOM/manufacturing facts.

---

## 8. PartGenealogyProjection

Derived read model combining:
- ProductStructureSnapshot;
- ManufacturingResultReceipt;
- RepairActionReceipt;
- supplier lot/status;
- SupplyChainTraceEvents.

Supports queries such as:
- which devices contain supplier lot X?
- where was serialized part Y used?
- which customers/population may be impacted?
- what is the parent/child lineage?

It is not a separate authority database.

---

## 9. As-planned / As-built / External trace

Keep distinct:

~~~text
AsPlanned
  engineering ProductStructure / ProductionRecipe

AsBuilt internal
  ManufacturingResultReceipt

AsBuilt external
  partner trace observations / data-space records
~~~

One supplier trace event cannot rewrite nominal ProductStructure.

---

# Part C — Digital Product Passport

## 10. DigitalProductPassportArtifact

Add optional immutable:

~~~text
DigitalProductPassportArtifact
~~~

May contain according to applicable profile:
- product/variant identity;
- manufacturer/economic-operator refs;
- unique identifiers;
- material/product-structure information;
- certification/compliance refs;
- service/repairability/end-of-life information;
- safety/use information;
- sustainability/environmental data;
- support/security lifecycle information;
- exact source-record refs;
- schema/profile version;
- content digest.

---

## 11. DPPProfile

Versioned:

~~~text
DPPProfile
~~~

Defines:
- jurisdiction/regulation/product group;
- schema/standard versions;
- required fields;
- identifier/data-carrier rules;
- field access classes;
- persistence/retention;
- source mapping;
- registration/publication requirements.

No EU-specific logic is hard-coded into the generic domain.

---

## 12. DPPProjectionReceipt

DPP generation is a transform/projection:

~~~text
authoritative product records
 -> projector/profile
 -> DigitalProductPassportArtifact
~~~

Receipt records:
- exact source object versions/digests;
- profile;
- projector/version;
- output digest;
- validation result;
- omissions/exceptions.

---

## 13. DPPRegistrationReceipt

External registration/publication creates:

~~~text
DPPRegistrationReceipt
~~~

Binds:
- DPP Artifact;
- registry/provider;
- identifier;
- submitter/economic operator;
- time;
- external status/receipt;
- supersession/update refs.

External Registry is not product engineering authority.

---

## 14. DPP privacy/access

DPPProfile classifies fields for access:
- public;
- authority;
- repair/service;
- supply-chain;
- restricted.

Projection respects:
- DataAssetProfile;
- privacy policy;
- confidentiality;
- embargo/security constraints.

DPP is not an export of the whole internal graph.

---

## 15. M0/M1 strategy

### M0
Reserve/freeze:
- SampleInstance;
- SamplePreparationReceipt;
- SampleCustodyReceipt;
- ExternalLaboratoryRef;
- LaboratoryResultImportReceipt;
- SupplyChainTraceEvent;
- PartGenealogyProjection schema;
- DigitalProductPassportArtifact;
- DPPProfile;
- DPPProjectionReceipt;
- DPPRegistrationReceipt.

### M1
No LIMS, Catena-X/data-space or DPP Registry integration.

---

## 16. M2/M3

Adopt when needed:
- certification/material lab integration;
- supplier/batch traceability;
- external quality alerts;
- DPP projection/registration;
- repair/recycling/product-lifecycle data exchange.

---

## 17. Final rules added by v38

1. Physical Sample identity differs from Device/Product identity.
2. Sample preparation and custody are append-only and auditable.
3. External laboratory systems remain referenced execution/workflow systems.
4. Lab Evidence binds exact sample/report identity.
5. Supplier/cross-company trace events are observations, not internal BOM authority.
6. Part genealogy is derived from immutable internal/external provenance.
7. Digital Product Passport is a projection over authoritative product facts.
8. DPP source mappings/profile/version are explicit and reproducible.
9. DPP updates create new immutable Artifacts/receipts.
10. DPP access/privacy policy prevents indiscriminate internal-data export.

Architecture v1.2 remains canonical.
Profile v38 is the current implementation companion.
