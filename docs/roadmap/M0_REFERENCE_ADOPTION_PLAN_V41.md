# M0 Reference Adoption Plan v41

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V40.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V41.md

## 1. Principle

Round 41 adds measurement/test interoperability schemas without adding ASAM infrastructure to M1.

M0 proves that standard files/interfaces can map into existing Procedure/Measurement/Evidence semantics without becoming a second source of truth.

---

# Track A — Measurement data

## 2. MeasurementDataArtifact fixture

Create one synthetic high-rate capture with:
- 3 channels;
- timebase;
- quantity/unit;
- raw-to-physical conversion;
- TestAttempt/Device refs.

Store as a simple internal fixture format.

Optionally create MDF/MF4 export if tooling is easy.

Prove:
- raw capture digest is immutable;
- converted MeasurementResult retains source/conversion refs;
- format does not affect authority.

---

## 3. Signal catalog fixture

Create:

~~~text
SignalCatalogArtifact
~~~

for:
- motor current;
- wheel speed;
- temperature.

Prove:
- stable signal identity is independent of display name;
- quantity/unit mismatch is detected;
- conversion changes create new catalog digest.

---

# Track B — Procedure implementation

## 4. ProcedureImplementationRef fixture

Map one ProcedureRevision to:
- pytest implementation;
- synthetic alternate test-sequence Artifact.

Prove:
- Procedure identity remains unchanged while implementation differs;
- policy can accept one implementation/issuer and reject another.

---

## 5. TestSequenceArtifact fixture

Create one structured TestSequenceArtifact.

No OTX parser required.

Prove:
- sequence bytes/version are immutable;
- Procedure-to-implementation mapping is explicit;
- external sequence cannot silently redefine Requirement/acceptance semantics.

---

# Track C — Bench abstraction

## 6. TestBenchInterfaceProfile fixture

Describe one representative HIL bench with:
- measurement/read/write;
- serial/network;
- fault injection;
- synchronized acquisition.

Prove:
- required Procedure capabilities can be matched against profile;
- unsupported capability causes scheduling failure;
- bench profile does not grant authority by self-report alone.

---

## 7. Timebase fixture

Create two measurement sources:
- MCU log clock;
- external instrument clock.

Define synchronization/alignment metadata.

Prove:
- cross-source timing result cannot be authoritative without declared timebase;
- changed synchronization method changes analysis provenance.

---

# Track D — Trace/test archive

## 8. TraceDataArtifact fixture

Create one timing trace Artifact and derive:
- latency MeasurementResult;
- performance Evidence.

Prove:
- raw trace remains immutable;
- derived percentile/latency result records calculation version.

---

## 9. External test archive fixture

Create synthetic TestDataArchiveRef.

Prove:
- external dataset ID/URL is locator/reference only;
- Formal Evidence retains exact digest/snapshot or imported Artifact when required.

---

# Track E — Existing M0

## 10. Retain all v40 requirements

All prior M0 work remains:
- system safety/STPA fixture;
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

## 11. M0 exit additions

M0 additionally requires:
- MeasurementDataArtifact fixture;
- SignalCatalogArtifact fixture;
- ProcedureImplementationRef mapping;
- TestBenchInterfaceProfile capability check;
- multi-source timebase/synchronization fixture;
- TraceDataArtifact to derived Measurement/Evidence fixture.

No MDF/OTX/XIL/ODS/TDF production dependency is required.

---

## 12. M1 target

M1 remains operationally unchanged.

The new contracts ensure future HIL/factory/certification data can adopt standard measurement/test formats without changing internal authority.
