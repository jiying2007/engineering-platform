# Reference-Aligned Implementation Profile v42

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V41.md

Research basis:
- Open Source / Public Standard Reference Review Rounds 1–42
- Architecture v1.2

## 1. Goal

Profile v42 retains all v41 semantics and adds provider-neutral device-diagnostics contracts.

The design supports custom serial diagnostics, UDS/DoIP and SOVD-style service interfaces without requiring any one protocol in M1.

---

## 2. DiagnosticServiceProfile

Add versioned:

~~~text
DiagnosticServiceProfile
~~~

Defines diagnostic capabilities:
- identity/version;
- health/status;
- DTC/fault state;
- logs/events;
- parameter/config read;
- routine execution;
- reset/restart;
- clear fault state;
- upload/download;
- programming/update;
- calibration/config write;
- security/provisioning functions.

Every capability is classified by risk and read/write behavior.

---

## 3. DiagnosticDataDefinition

Add versioned:

~~~text
DiagnosticDataDefinition
~~~

Metadata:
- stable diagnostic ID;
- semantic name;
- quantity/unit/type;
- access mode;
- valid range;
- persistence/clearing behavior;
- Target/Interface revision;
- sensitivity/privacy;
- encoding/conversion;
- content digest.

Possible mappings:
- UDS DID/DTC/Routine;
- SOVD resource;
- MCU diagnostic register;
- Linux health/service endpoint.

---

## 4. DiagnosticSnapshotArtifact

Add immutable:

~~~text
DiagnosticSnapshotArtifact
~~~

Captures:
- DeviceInstance;
- Target/Release/component versions;
- configuration/calibration;
- active/stored DTCs;
- health/counters;
- logs/attachments;
- trust/attestation summary;
- profile/Procedure;
- timestamp;
- raw payload;
- digest.

Primary use:
- RMA;
- Incident;
- pre/post OTA;
- HIL failure;
- repair;
- field support.

---

## 5. Snapshot-before-mutation

Policy may require DiagnosticSnapshotArtifact before:
- DTC clear;
- reset;
- erase;
- reprogram;
- config/calibration write;
- security/ownership reset;
- other evidence-destroying operation.

This extends the existing "never repair away the evidence" invariant.

---

## 6. DiagnosticQueryReceipt

Read-only diagnostic access may create:

~~~text
DiagnosticQueryReceipt
~~~

Binds:
- Device;
- service/item;
- requester/Run;
- transport/provider;
- raw response;
- normalized result;
- time;
- provenance/trust;
- errors/timeouts.

Repeated/high-volume queries can be aggregated into a snapshot.

---

## 7. DiagnosticOperationReceipt

State-changing diagnostics produce:

~~~text
DiagnosticOperationReceipt
~~~

Binds:
- Device;
- exact operation/parameters;
- authorization Decision;
- actor/Run;
- transport/provider;
- before snapshot where required;
- external result;
- observed after-state;
- timestamps;
- UNKNOWN/reconciliation status.

Privileged Formal diagnostic mutation has no receipt-less path.

---

## 8. Action Gateway integration

Diagnostic mutation follows:

~~~text
Runtime/Human
 -> requested diagnostic operation
 -> OPA / DeviceTrust / capability policy
 -> Platform Action Gateway
 -> UDS/SOVD/custom backend
 -> DiagnosticOperationReceipt
 -> observed DeviceState
~~~

Runtime never receives unrestricted write access merely because it can inspect diagnostics.

---

## 9. Diagnostic risk classes

Suggested:

~~~text
OBSERVE
CONTROLLED_MUTATION
HIGH_RISK
~~~

Examples:

### OBSERVE
- read version/status/DTC/log.

### CONTROLLED_MUTATION
- controlled routine;
- reset;
- clear DTC;
- non-security parameter write.

### HIGH_RISK
- programming;
- secure credential/provisioning;
- anti-rollback/security state;
- ownership reset;
- production calibration.

Human Authority/policy scales accordingly.

---

## 10. Transport/backend neutrality

Possible backend:
- serial;
- CAN/UDS;
- DoIP;
- SOVD/HTTP;
- SSH/service API;
- vendor transport.

Transport/provider version is always recorded.

Diagnostic semantic identity does not depend on one transport.

---

## 11. UDS and OpenSOVD strategy

### UDS
Good candidate for MCU/ECU standardized diagnostics when it reduces custom protocol work.

### SOVD/OpenSOVD
Strong future reference for Linux/software-heavy devices, remote service gateways and service-oriented diagnostics.

Eclipse OpenSOVD currently implements ISO 17978-3:2026 concepts.

Neither is required for M1.

---

## 12. Interface Contract integration

DiagnosticServiceProfile/DataDefinition are Interface Contracts.

Changes use:
- InterfaceCompatibilityPolicy;
- Consumer Contract;
- version negotiation;
- Target/Release impact analysis.

Diagnostic schema compatibility is machine-verifiable, not assumed.

---

## 13. Privacy/security

Diagnostic policy binds:
- actor;
- purpose;
- DeviceTrustProfile;
- environment;
- data class;
- allowed services;
- secure session/auth.

Diagnostic API is not a privacy/security bypass around:
- ToolProfile;
- DataProcessingActivity;
- Credential Broker;
- Action Gateway.

---

## 14. Digital Twin / RMA / OTA

Digital Twin may display diagnostic summary only.

Canonical history remains:
- DiagnosticSnapshotArtifact;
- Query/Operation Receipt;
- DeviceStateSnapshot;
- Incident/RMA;
- observed/reconciled state.

Recommended RMA path:

~~~text
AS_RETURNED
 -> diagnostic snapshot
 -> failure analysis
 -> controlled mutation/repair
 -> post-repair snapshot
 -> Verification
~~~

Pre/post OTA snapshots may also support UpdatePathEvidence.

---

## 15. M0/M1 strategy

### M0
Freeze:
- DiagnosticServiceProfile;
- DiagnosticDataDefinition;
- DiagnosticSnapshotArtifact;
- DiagnosticQueryReceipt;
- DiagnosticOperationReceipt.

Create one synthetic custom diagnostic profile.

### M1
No UDS/SOVD service.

### M2/M3
Evaluate:
- UDS for MCU diagnostics;
- OpenSOVD/SOVD for Linux/software-heavy service diagnostics.

---

## 16. Final rules added by v42

1. Diagnostics is a governed service surface, not arbitrary debug access.
2. Observation and mutation are distinct risk classes.
3. Evidence-destroying diagnostics can require snapshot-before-mutation.
4. Every privileged diagnostic mutation is receipted.
5. Diagnostic writes go through Action Gateway/policy.
6. Transport/provider is replaceable and recorded.
7. Diagnostic state is Evidence/observation, not Incident/root-cause authority.
8. Diagnostic definitions are versioned Interface Contracts.
9. Digital Twin only projects diagnostic summaries.
10. UDS/SOVD adoption is product/use-case driven.

Architecture v1.2 remains canonical.
Profile v42 is the current implementation companion.
