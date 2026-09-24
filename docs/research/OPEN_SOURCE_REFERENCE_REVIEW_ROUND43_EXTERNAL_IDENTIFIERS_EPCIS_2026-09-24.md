# Public Standard Reference Review — Round 43: External Identifiers and EPCIS Trace Events

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- GS1 EPCIS / CBV 2.0
- GS1 Global Traceability Standard
- GS1 Digital Link
- Tractus-X supply-chain traceability
- existing DeviceInstance, Part, Sample, SupplyChainTraceEvent and DPP semantics

## 1. Key conclusion

Internal engineering identity and external ecosystem identifiers are different concerns.

Use:

~~~text
Internal canonical identity
  DeviceInstance / Part / Sample / Target / Artifact

ExternalIdentifierBinding
  GS1 / supplier / PLM / MES / DPP / customer / regulator identity
~~~

Do not let an external identifier scheme become the internal primary key.

---

## 2. ExternalIdentifierBinding

Add:

~~~text
ExternalIdentifierBinding
~~~

Fields:
- internal typed ref;
- identifier scheme;
- namespace/issuer;
- identifier value;
- qualifier:
  product
  variant
  serial
  lot/batch
  asset
  location
  document
  shipment
  component
  other;
- valid_from;
- valid_until/effectivity;
- source/provenance;
- verification/status;
- locator/resolver refs where relevant.

One internal entity may have multiple external identifiers.

---

## 3. Identifier schemes

Examples:
- GS1 GTIN;
- GTIN + serial;
- batch/lot;
- GLN/location;
- GIAI/asset;
- SSCC/logistics unit;
- manufacturer part number;
- supplier part number;
- PLM part/revision;
- ERP/MES serial;
- DPP identifier;
- customer/service identifier.

Scheme name alone is insufficient.
Issuer/namespace/effectivity prevent collisions.

---

## 4. Identifier binding verification

External identity may be:
- imported;
- assigned internally;
- scanned from barcode/QR/NFC;
- provided by supplier;
- registered externally.

Where risk requires, create a verification/receipt that proves:
- identifier syntax/namespace valid;
- identifier belongs to intended entity;
- duplicate/collision checks pass;
- external registration/issuer confirms it where applicable.

Do not trust a scanned string solely because it parses.

---

## 5. Identifier changes

Some external IDs are immutable; others may be replaced/superseded.

Use:
- validity/effectivity;
- SUPERSEDES TraceLink where appropriate;
- historical bindings retained.

Never reassign an identity value to a different physical serial without an explicit scheme rule and audit trail.

---

## 6. EPCIS-compatible SupplyChainTraceEvent

Refine Round 37:

~~~text
SupplyChainTraceEvent
~~~

with standard dimensions inspired by EPCIS:

### What
- objects/parts/batches/serials/documents involved.

### When
- event time;
- timezone;
- record/ingest time.

### Where
- read point;
- business/location context.

### Why
- business step;
- disposition/state;
- transaction/order context;
- parties.

### How
- sensor/environmental condition where available.

This is a strong interoperability model for cross-company trace events.

---

## 7. Event types / actions

Supply-chain trace event may represent:
- observe;
- commission/create;
- aggregate/disaggregate;
- transform;
- ship/receive;
- install/remove;
- repair;
- inspect;
- quarantine;
- consume/scrap;
- custom.

Internal semantics remain typed and provider-neutral.

EPCIS export/import maps where a standard event representation fits.

---

## 8. Event time versus record time

Preserve separately:
- when physical/business event happened;
- when external system recorded it;
- when engineering-platform ingested it.

This matters for:
- late supplier data;
- recall investigation;
- sequence reconstruction;
- stale/retroactive events.

Do not use ingest time as event truth.

---

## 9. External location identity

Locations may have:
- internal site/station ID;
- GLN or supplier/customer location ID;
- factory/line/station hierarchy.

Use ExternalIdentifierBinding.

Location resolution does not redefine StationProfile authority/trust class.

---

## 10. GS1 Digital Link

GS1 Digital Link provides URI syntax/resolution for GS1 identifiers and linked web resources.

Potential uses:
- product QR/data carrier;
- DPP access;
- service/manual/repair resources;
- lifecycle information routing.

### Decision

Treat Digital Link URI/resolver as:
- external identifier/locator mechanism;
- DPP/service integration.

Do not make resolver URL the product identity.

---

## 11. DPP relationship

DigitalProductPassportArtifact/DPPRegistrationReceipt may reference an ExternalIdentifierBinding.

Example:

~~~text
ProductVariant / Device
 -> GS1 identifier binding
 -> DPP identifier/data carrier
 -> resolver/registry
~~~

Internal product/variant/revision identity remains independent.

---

## 12. RMA/manufacturing relationship

Barcode/QR scans can resolve:
- DeviceInstance;
- component serial;
- lot/batch;
- ProductionRecipe;
- ReturnReceipt.

But scanning is an observation.

The binding must be validated against manufacturing/service history before privileged decisions.

---

## 13. Part genealogy

PartGenealogyProjection can normalize partner identifiers through ExternalIdentifierBinding.

This allows:
- supplier part ID;
- OEM internal part ID;
- GS1 component ID;
- MES serial

to refer to one known entity without copying all systems into one namespace.

---

## 14. Identifier privacy

Some identifiers may expose:
- customer/device association;
- location;
- service history.

DataAsset/Privacy policy determines:
- public DPP identifier;
- internal serial;
- pseudonymized telemetry ID;
- restricted service identifier.

Do not publish internal serial/account mappings by default.

---

## 15. M0/M1 impact

M0 reserves:
- ExternalIdentifierBinding;
- identifier-scheme registry/profile;
- EPCIS-compatible fields on SupplyChainTraceEvent.

M1 can use repository/Run internal IDs only.
No GS1 service is required.

---

## 16. M2/M3 impact

Adopt when needed:
- factory barcode/serial integration;
- supplier lot trace;
- DPP/data-carrier;
- recall/RMA;
- cross-enterprise EPCIS exchange.

---

## 17. Invariants

1. Internal canonical ID and external ecosystem ID are distinct.
2. External identifiers are namespace/issuer/effectivity scoped.
3. Historical identifier bindings are not silently rewritten.
4. A parsed/scanned identifier is an observation until binding is validated.
5. Supply-chain event time, record time and ingest time remain distinct.
6. EPCIS/GS1 is an interoperability projection, not internal authority.
7. Digital Link URL/resolver is a locator, not product identity.
8. Part genealogy can normalize multiple external namespaces through bindings.
9. External location identifiers do not grant station trust/capability.
10. Identifier publication follows privacy/data policy.

## 18. Conclusion

The durable identity rule is:

> **Keep one stable internal identity for engineering objects, bind external identifiers explicitly with issuer and effectivity, and use standards such as EPCIS/GS1 to exchange trace events without surrendering internal authority.**
