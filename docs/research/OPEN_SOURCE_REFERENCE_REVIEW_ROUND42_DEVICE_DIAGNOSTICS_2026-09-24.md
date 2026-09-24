# Open Source / Standard Reference Review — Round 42: Device Diagnostics and Service Interfaces

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- ISO 17978-3:2026 / Eclipse OpenSOVD
- ASAM SOVD concepts
- ISO 14229 UDS open-source implementations such as iso14229
- existing DeviceInstance, DeviceStateSnapshot, RMA, Action Gateway, Interface Contract and ToolProfile semantics

## 1. Key conclusion

Device diagnostics should be modeled as a governed service surface, not as arbitrary shell/serial access.

The durable separation is:

~~~text
Diagnostic definition/profile
 -> read-only diagnostic observation
 -> immutable diagnostic snapshot
 -> investigation/reproduction
~~~

versus:

~~~text
state-changing diagnostic operation
 -> authorization
 -> exact operation request
 -> device/provider execution
 -> operation receipt
 -> observed-state reconciliation
~~~

---

## 2. DiagnosticServiceProfile

Add versioned:

~~~text
DiagnosticServiceProfile
~~~

Defines device diagnostic capabilities:
- identity/version queries;
- state/health/status queries;
- DTC/fault-code readout;
- logs/events;
- parameter/config read;
- routine execution;
- reset/restart;
- clear diagnostic state;
- memory/data upload/download;
- firmware/programming services;
- calibration/parameter write;
- bulk data;
- service-specific authorization/security.

Capabilities carry risk/read-write classification.

---

## 3. DiagnosticDataDefinition

Add:

~~~text
DiagnosticDataDefinition
~~~

for stable diagnostic identifiers/items:
- diagnostic item ID;
- semantic name;
- quantity/unit/type;
- read/write access;
- valid ranges;
- Target/Interface revision;
- privacy/sensitivity;
- persistence/clearing semantics;
- conversion/encoding;
- version/digest.

This may map from:
- UDS DIDs/DTCs/Routines;
- SOVD resources;
- custom MCU diagnostic registers;
- Linux service health endpoints.

---

## 4. DiagnosticSnapshotArtifact

Add immutable:

~~~text
DiagnosticSnapshotArtifact
~~~

Captures diagnostic state at one point/window:
- DeviceInstance;
- Target/Release/component versions;
- configuration/calibration;
- active/stored DTCs;
- health/status;
- counters;
- relevant logs;
- trust/attestation summary;
- capture Procedure/profile;
- timestamp;
- raw payload refs;
- content digest.

Use for:
- RMA AS_RETURNED;
- Incident;
- pre/post OTA;
- pre/post repair;
- HIL failure capture;
- field support.

---

## 5. Snapshot-before-mutation rule

Before destructive/state-changing diagnostics such as:
- clear DTC;
- reset;
- erase;
- reprogram;
- config write;
- calibration write;
- security/ownership reset;

policy may require a DiagnosticSnapshotArtifact.

This preserves evidence that would otherwise be destroyed.

---

## 6. DiagnosticQueryReceipt

Read-only query may produce:

~~~text
DiagnosticQueryReceipt
~~~

Records:
- DeviceInstance;
- profile/service/item;
- requester/Run;
- time;
- transport/provider;
- raw response;
- normalized result;
- trust/provenance;
- errors/timeouts.

High-volume queries may be grouped into one snapshot rather than one record each.

---

## 7. DiagnosticOperationReceipt

State-changing operation creates immutable:

~~~text
DiagnosticOperationReceipt
~~~

Binds:
- DeviceInstance;
- operation/service;
- exact request/parameters;
- authorization/capability Decision;
- requester/actor/Run;
- provider/transport;
- before-snapshot ref where required;
- external result;
- observed after-state;
- timestamps;
- errors/UNKNOWN;
- reconciliation status.

The receipt is not optional for privileged Formal operations.

---

## 8. Diagnostic operations use Action Gateway

Do not give Runtime unrestricted low-level diagnostic write access.

Path:

~~~text
Runtime/Human
 -> requested Diagnostic Operation
 -> OPA / capability / device trust policy
 -> Platform Action Gateway
 -> UDS/SOVD/custom backend
 -> DiagnosticOperationReceipt
 -> observed DeviceState
~~~

This reuses the existing privileged-action boundary.

---

## 9. Read versus write risk

DiagnosticServiceProfile should classify actions such as:

### OBSERVE
- read versions;
- read status;
- read DTCs;
- collect logs.

### CONTROLLED_MUTATION
- execute test routine;
- reset;
- clear DTC;
- write non-security parameter.

### HIGH_RISK
- firmware download/programming;
- secure credential/provisioning;
- anti-rollback/security state;
- ownership reset;
- production calibration.

Human Authority/policy requirements increase by class.

---

## 10. UDS practice

ISO 14229 UDS provides mature service concepts for classic embedded ECUs.

Open-source iso14229 demonstrates:
- portable C;
- client/server;
- static memory;
- CAN/ISO-TP transports;
- embedded/Zephyr/STM32/NXP examples;
- fuzz/unit testing.

Potential use for MCU products when diagnostic standardization brings value.

Do not require UDS merely to replace a simple internal debug protocol.

---

## 11. SOVD / OpenSOVD practice

SOVD standardizes service-oriented diagnostics for software-heavy devices/ECUs.

Eclipse OpenSOVD implements ISO 17978-3:2026 with Server/Client/Gateway concepts.

SOVD-style resources are useful where diagnostics includes:
- software/application health;
- logs;
- software update;
- parameter/bulk-data transfer;
- remote/proximity access.

### Decision

Use SOVD as a strong future diagnostic-gateway/API reference for Linux/software-heavy devices.

Do not force automotive URI/resource structure into M1.

---

## 12. Diagnostic transport is replaceable

Potential transport/backend:
- serial;
- CAN/UDS;
- DoIP;
- SOVD/HTTP;
- SSH/service API;
- vendor debug channel.

The Diagnostic Service semantics remain provider-neutral.

Transport identity/version is recorded on receipts.

---

## 13. Diagnostic security

Diagnostic access can expose:
- secrets;
- memory;
- personal data;
- security state;
- provisioning information.

Profile/policy must bind:
- actor;
- purpose;
- DeviceTrustProfile;
- environment;
- secure session/auth;
- data class;
- allowed services.

Diagnostic access is not a shortcut around ToolProfile/DataProcessing policy.

---

## 14. DTC/fault state lifecycle

Fault code state may be:
- ACTIVE;
- STORED;
- PENDING;
- CLEARED;
- HISTORICAL;
- UNKNOWN.

Clearing a fault does not erase the historical DiagnosticSnapshot/Incident.

Diagnostic state is operational observation, not Incident truth by itself.

---

## 15. Diagnostic definitions and Interface Contracts

DiagnosticServiceProfile/DataDefinition are versioned Interface Contracts.

Changes may be:
- wire/API compatible;
- behavioral breaking;
- data/persisted-state breaking.

Use v16 InterfaceCompatibilityPolicy/Evidence.

---

## 16. Digital Twin relationship

DeviceTwinProjection may summarize:
- current diagnostic health;
- active DTC count;
- software/version;
- selected status.

Canonical history remains:
- DiagnosticSnapshotArtifact;
- Diagnostic receipts;
- ObservedDeviceState;
- Incident/RMA records.

---

## 17. RMA / repair relationship

Recommended RMA path:

~~~text
ReturnReceipt
 -> AS_RETURNED DeviceStateSnapshot
 -> DiagnosticSnapshotArtifact
 -> FailureAnalysis
 -> controlled repair/diagnostic mutation
 -> RepairActionReceipt
 -> POST_REPAIR DiagnosticSnapshot
 -> Repair Verification
~~~

This operationalizes "never repair away the evidence."

---

## 18. OTA relationship

Pre/post-update diagnostic snapshots can support:
- current version verification;
- compatibility;
- health baseline;
- rollback diagnostics;
- update-path Evidence.

Firmware programming itself remains Release/Action Gateway governed, not ordinary diagnostic convenience.

---

## 19. M0/M1 impact

M0 reserves:
- DiagnosticServiceProfile;
- DiagnosticDataDefinition;
- DiagnosticSnapshotArtifact;
- DiagnosticQueryReceipt;
- DiagnosticOperationReceipt.

M1 requires no UDS/SOVD implementation.

A synthetic custom diagnostic profile is sufficient.

---

## 20. M2/M3 impact

For real products:
- define common diagnostic item IDs across Linux/MCUs;
- capture failure snapshots;
- integrate serial/custom initially;
- evaluate UDS for MCU diagnostic standardization;
- evaluate OpenSOVD for software-heavy remote service/gateway use.

---

## 21. Invariants

1. Diagnostic access is a governed service surface, not arbitrary debug access.
2. Read-only observations and state-changing operations are distinct risk classes.
3. Destructive diagnostics can require snapshot-before-mutation.
4. Every privileged diagnostic mutation produces an immutable receipt.
5. Runtime diagnostic write actions go through Action Gateway/policy.
6. Diagnostic transport is replaceable and recorded.
7. Diagnostic state is operational evidence, not Incident/root-cause authority.
8. Diagnostic schemas are versioned Interface Contracts.
9. Digital Twin displays diagnostic summaries but does not own history.
10. UDS/SOVD adoption is product/use-case driven.

## 22. Conclusion

The durable diagnostics rule is:

> **Observe first, preserve the diagnostic state, then authorize and receipt every state-changing action so debugging, repair and remote service never destroy the evidence needed to understand the device.**
