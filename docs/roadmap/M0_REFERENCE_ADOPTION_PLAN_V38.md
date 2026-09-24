# M0 Reference Adoption Plan v38

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V36.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V38.md

## 1. Principle

Rounds 37–38 add only schema/fixture coverage for:
- sample identity/custody;
- external laboratory imports;
- cross-enterprise traceability;
- Digital Product Passport projection.

No new M1 production service is required.

---

# Track A — Sample / Lab

## 2. Sample fixture

Create:
- one SampleInstance derived from a DeviceInstance;
- one SamplePreparationReceipt;
- two SampleCustodyReceipts.

Prove:
- sample identity differs from device identity;
- custody is append-only;
- preparation creating material change yields a new sample identity.

---

## 3. External-lab fixture

Create:
- ExternalLaboratoryRef;
- external report Artifact;
- LaboratoryResultImportReceipt.

Prove:
- imported Evidence binds exact SampleInstance;
- report filename/provider ID cannot substitute for content digest;
- importer/mapping version is retained.

---

# Track B — Supply-chain traceability

## 4. Supplier-trace fixture

Create imported SupplyChainTraceEvents for:
- supplier batch;
- parent-child relation;
- quality alert.

Combine with:
- ProductStructureSnapshot;
- ManufacturingResultReceipt.

Prove:
- external events do not rewrite internal BOM;
- PartGenealogyProjection identifies affected Device population;
- quality alert creates internal ImpactAnalysis/QualityCase proposal only.

---

# Track C — DPP

## 5. DPP profile fixture

Create synthetic:
- DPPProfile;
- product/variant/Target source records;
- DigitalProductPassportArtifact;
- DPPProjectionReceipt.

Prove:
- source versions/digests are explicit;
- confidential field is excluded by profile/privacy policy;
- DPP Artifact is reproducible from same source/profile.

---

## 6. DPP update fixture

Change:
- one support/compliance fact.

Create:
- new DPP Artifact;
- supersession link;
- synthetic DPPRegistrationReceipt.

Prove:
- old DPP remains immutable;
- registry status cannot rewrite internal facts.

---

# Track D — Existing M0

## 7. Retain all v36 requirements

All previous M0 work remains:
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
- SampleInstance/preparation/custody fixture;
- external-lab import fixture;
- part-genealogy projection from mixed internal/external trace;
- DPP projection/update fixture.

M0 does not require:
- LIMS;
- Catena-X/Tractus-X runtime;
- DPP Registry integration.

---

## 9. M1 target

M1 remains operationally unchanged.

The new contracts only guarantee laboratory/cross-enterprise/product-passport integration can attach later without redesigning core authority.
