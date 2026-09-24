# M0 Reference Adoption Plan v43

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V42.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V43.md

## 1. Principle

Round 43 adds external-identifier and EPCIS-compatible trace-event semantics without introducing GS1 infrastructure into M1.

---

# Track A — Identifier binding

## 2. ExternalIdentifierBinding fixture

Create one DeviceInstance with:
- internal device ID;
- MES serial;
- GS1-style product+serial binding;
- DPP identifier binding.

Prove:
- internal ID remains stable/canonical;
- multiple external identifiers may coexist;
- identifier scheme + issuer/namespace are required;
- one expired/superseded binding remains historical.

---

## 3. Scan/import validation fixture

Simulate barcode/QR scan.

Cases:
- valid identifier bound to expected Device;
- valid syntax but unknown binding;
- duplicate/collision;
- expired binding.

Prove:
- scan does not directly authorize privileged action;
- binding validation is explicit.

---

# Track B — EPCIS-style trace events

## 4. SupplyChainTraceEvent fixture

Create:
- manufacturing/commission event;
- shipment event;
- receipt event;
- quality/quarantine event.

Record:
- What;
- When;
- Where;
- Why;
- optional How.

Prove:
- physical event time differs from ingest time;
- external event cannot rewrite ProductStructure/ManufacturingResultReceipt;
- part genealogy can consume the events.

---

## 5. Location fixture

Bind:
- internal Station/Site;
- external location ID such as GLN-like identifier.

Prove:
- external location binding does not grant station trust/capability;
- station identity/trust remains internal.

---

# Track C — DPP / privacy

## 6. Digital Link fixture

Create one external URI/locator bound to DPP/product identifier.

Prove:
- URI change/redirect does not change internal product identity;
- public resolver data cannot expose restricted service/customer identifiers.

---

# Track D — Existing M0

## 7. Retain all v42 requirements

All previous M0 work remains:
- diagnostics;
- HIL/ASAM interoperability;
- system safety/STPA;
- metrology/calibration;
- sample/custody/DPP;
- audit/recovery;
- certification/privacy/prognostics/compatibility/threat modeling;
- supplier/compliance/twin/FOSS;
- PLM/PSIRT/RMA/variants;
- AI governance/review;
- Device trust;
- ML lineage;
- Interface compatibility;
- ResolvedConfiguration;
- TraceLink;
- Run/Build/Integration/Release;
- OTel/CDEvents;
- formal model checking;
- failure injection.

---

## 8. M0 exit additions

M0 additionally requires:
- one object with multiple external identifier bindings;
- scan/import validation fixture;
- event/record/ingest time distinction;
- EPCIS-style trace-event fixture;
- location binding without trust escalation;
- Digital Link/locator != identity test.

No GS1 resolver/EPCIS repository is required.

---

## 9. M1 target

M1 remains operationally unchanged.

The external-identity contracts ensure future PLM/MES/DPP/supplier/RMA integrations can align identities without replacing internal canonical IDs.
