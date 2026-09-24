> **Historical/reference profile.** This accumulated research profile is no longer the current implementation checklist. Active Core scope is defined by [Embedded AI Engineering Platform — Core Architecture v1](EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md) and the [Extension Catalog](../extensions/EXTENSION_CATALOG_V1.md).

# Reference-Aligned Implementation Profile v43

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V42.md

Research basis:
- Open Source / Public Standard Reference Review Rounds 1–43
- Architecture v1.2

## 1. Goal

Profile v43 retains all v42 semantics and adds a small, generic external-identifier layer plus EPCIS-compatible supply-chain event semantics.

No GS1 service or new identity platform is required.

---

## 2. Internal identity remains canonical

Internal objects retain stable platform identity:
- DeviceInstance;
- Part/Component;
- SampleInstance;
- TargetRevision;
- Artifact;
- Release;
- Station.

External identifiers never replace internal primary identity.

---

## 3. ExternalIdentifierBinding

Add:

~~~text
ExternalIdentifierBinding
~~~

Fields:
- internal typed ref;
- identifier scheme;
- namespace/issuer;
- identifier value;
- qualifier/type;
- valid_from;
- valid_until/effectivity;
- source/provenance;
- verification/status;
- locator/resolver refs.

Qualifiers may include:
- PRODUCT;
- VARIANT;
- SERIAL;
- LOT_BATCH;
- ASSET;
- LOCATION;
- DOCUMENT;
- SHIPMENT;
- COMPONENT;
- SERVICE;
- CUSTOM.

---

## 4. Identifier namespaces

Examples:
- GS1 GTIN;
- GS1 GTIN + serial;
- GS1 lot/batch;
- GLN;
- GIAI;
- SSCC;
- manufacturer part number;
- supplier part number;
- PLM revision ID;
- MES serial;
- DPP identifier;
- customer/service ID.

Scheme + issuer/namespace are part of identity.

---

## 5. External identifier validation

A scanned/imported identifier is an observation until validated.

Validation may check:
- syntax/profile;
- namespace/issuer;
- duplicate/collision;
- external registration;
- mapping to expected internal entity;
- effectivity.

High-risk operations cannot rely on a raw barcode string alone.

---

## 6. Identifier lifecycle

Historical bindings remain immutable.

Use:
- validity/effectivity;
- supersession refs;
- typed migration/reassignment receipts where a scheme permits changes.

Never silently reuse one serial/identifier for a different physical entity.

---

## 7. EPCIS-compatible SupplyChainTraceEvent

Refine existing:

~~~text
SupplyChainTraceEvent
~~~

with:

### What
Objects/parts/batches/serials/documents involved.

### When
- business event time;
- timezone;
- external record time;
- platform ingest time.

### Where
- read point;
- business location;
- partner/site context.

### Why
- business step;
- disposition;
- transaction/order context;
- parties/roles.

### How
- sensor/environmental conditions where available.

---

## 8. Supply-chain event semantics

Possible actions/events:
- commission/create;
- observe;
- aggregate/disaggregate;
- transform;
- ship/receive;
- install/remove;
- repair;
- inspect;
- quarantine;
- consume/scrap.

The internal model remains provider-neutral.
EPCIS is an import/export projection where useful.

---

## 9. Digital Link

GS1 Digital Link is treated as:
- identifier/URI encoding;
- locator/resolver mechanism;
- possible DPP/service/manual entry point.

A Digital Link URL is not the product's internal identity.

---

## 10. Part genealogy

PartGenealogyProjection normalizes external supplier/OEM/MES/GS1 identifiers through ExternalIdentifierBinding.

This enables cross-system impact queries without creating one giant global ID namespace.

---

## 11. Location identity

External location identifiers such as GLN may bind to:
- Site;
- Factory;
- Line;
- Station;
- Warehouse/Service location.

External location binding does not grant StationProfile trust/capability.

---

## 12. Privacy

Some identifier bindings may expose:
- customer/device association;
- location;
- service history.

Publication/resolution follows:
- DataAssetProfile;
- DPPProfile;
- privacy/access policy.

Public product identifiers and private service/account identifiers stay distinct.

---

## 13. M0/M1 strategy

### M0
Freeze:
- ExternalIdentifierBinding;
- identifier scheme/namespace profile;
- EPCIS-compatible SupplyChainTraceEvent fields.

### M1
No GS1/EPCIS dependency.

### M2/M3
Use for:
- production barcode/serial;
- supplier lot traceability;
- RMA/recall;
- DPP;
- cross-enterprise supply-chain exchange.

---

## 14. Final rules added by v43

1. Internal canonical ID and external ecosystem identifier are distinct.
2. External identifiers are issuer/namespace/effectivity scoped.
3. Historical bindings are append-only/superseded rather than silently rewritten.
4. Scanned/imported ID is an observation until validated.
5. Physical event time, external record time and platform ingest time remain distinct.
6. EPCIS/GS1 is interoperability, not Control Plane authority.
7. Digital Link URI is a locator/resolution mechanism, not internal identity.
8. Part genealogy may normalize many external namespaces through bindings.
9. External location identity does not imply station trust.
10. Identifier publication follows privacy/access policy.

Architecture v1.2 remains canonical.
Profile v43 is the current implementation companion.
