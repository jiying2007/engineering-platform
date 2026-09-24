# M0 Reference Adoption Plan v42

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V41.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V42.md

## 1. Principle

Round 42 adds diagnostic service/snapshot/receipt semantics without adding UDS/SOVD runtime dependencies to M1.

---

# Track A — Diagnostic profile

## 2. DiagnosticServiceProfile fixture

Create a synthetic product profile covering:
- version query;
- health query;
- DTC read;
- log collection;
- reset;
- DTC clear;
- config write;
- firmware programming.

Classify each:
- OBSERVE;
- CONTROLLED_MUTATION;
- HIGH_RISK.

Prove policy can distinguish the risk classes.

---

## 3. DiagnosticDataDefinition fixture

Define:
- software version;
- motor overcurrent counter;
- board temperature;
- one stored fault code;
- one writable threshold.

Prove:
- units/types/ranges are versioned;
- privacy/sensitivity can restrict one diagnostic item;
- contract change produces a new digest.

---

# Track B — Snapshot / mutation

## 4. Snapshot-before-mutation fixture

Simulate:
- device with active fault;
- DiagnosticSnapshotArtifact;
- clear-DTC request;
- DiagnosticOperationReceipt;
- post-operation observed state.

Prove:
- policy blocks clear without required pre-snapshot;
- historical fault state remains after clear.

---

## 5. High-risk operation fixture

Simulate firmware programming request.

Prove:
- ordinary Runtime diagnostic access is insufficient;
- Action Gateway authorization required;
- exact Release/Artifact subject is checked;
- receipt records UNKNOWN if response is lost;
- reconciliation occurs before retry.

---

# Track C — RMA / OTA integration

## 6. RMA diagnostic fixture

Create:

~~~text
ReturnReceipt
 -> AS_RETURNED DeviceStateSnapshot
 -> DiagnosticSnapshotArtifact
 -> FailureAnalysisRecord
 -> controlled repair
 -> POST_REPAIR DiagnosticSnapshot
 -> Repair Verification
~~~

Prove no repair step erases pre-repair diagnostic evidence.

---

## 7. OTA diagnostic fixture

Create:
- pre-update diagnostic snapshot;
- SystemUpdateManifest execution;
- post-update snapshot;
- UpdatePathEvidence.

Prove:
- post-update health/version observations are reconciled;
- diagnostic snapshot alone does not replace Release confirmation.

---

# Track D — Existing M0

## 8. Retain all v41 requirements

All previous M0 work remains:
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

## 9. M0 exit additions

M0 additionally requires:
- DiagnosticServiceProfile fixture;
- DiagnosticDataDefinition fixture;
- snapshot-before-clear/mutation enforcement;
- high-risk diagnostic operation via Action Gateway;
- RMA pre/post diagnostic evidence fixture;
- OTA pre/post diagnostic reconciliation fixture.

No UDS/SOVD server/client is required.

---

## 10. M1 target

M1 remains operationally unchanged.

The diagnostic contracts ensure later RMA, field support, OTA and remote-service features do not fall back to unaudited shell/debug operations.
