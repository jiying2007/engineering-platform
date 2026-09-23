# Open Source Reference Review — Round 28: Digital Twin and Observed Device State

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- Eclipse Ditto
- Eclipse Kanto
- ThingsBoard
- earlier DeviceInstance / PromotionPlan / DevicePromotionAttempt / DeviceStateSnapshot semantics

## 1. Key conclusion

Digital Twin / Device Shadow is a useful operational read model, but must not become engineering authority.

The platform must distinguish:

~~~text
Desired engineering state
Observed device state
Last-known twin state
Authoritative release/manufacturing history
~~~

---

## 2. DeviceTwinProjection

Add optional derived/read model:

~~~text
DeviceTwinProjection
~~~

Contains:
- DeviceInstance;
- desired Release/config refs;
- observed software/component versions;
- observed configuration;
- health/connectivity;
- trust/attestation summary;
- operational attributes;
- last_seen;
- source/telemetry provider;
- freshness/quality.

It is reconstructable from device/fleet observations.

---

## 3. ObservedDeviceState

Add immutable observation record:

~~~text
ObservedDeviceState
~~~

Binds:
- DeviceInstance;
- observation time;
- observed component versions/digests where available;
- configuration/feature state;
- device-reported properties;
- environment/status;
- source channel/provider;
- trust/provenance;
- raw telemetry/evidence refs.

This is stronger than one mutable "current version" field.

---

## 4. Desired vs observed

Desired state comes from:
- Release Manifest;
- PromotionPlan;
- SystemUpdateManifest;
- RuntimeConfigurationSnapshot.

Observed state comes from:
- device report;
- attestation;
- updater;
- telemetry/twin provider.

Reconciliation compares both.

A successful command dispatch does not imply convergence.

---

## 5. Last-known state is freshness-bound

Digital twins commonly retain state while devices are offline.

Therefore every observed field/state needs:
- observed_at;
- source;
- freshness class/expiry;
- optional confidence.

Policy must distinguish:
- current enough for display;
- current enough for rollout decision;
- current enough for security-sensitive action.

---

## 6. Device-reported state is not fully trusted

A device may self-report:
- version;
- health;
- feature state.

Trust depends on:
- channel authentication;
- device identity;
- attestation;
- signed version metadata;
- updater/provider receipt.

For high-risk actions, self-report alone may be insufficient.

---

## 7. Twin write/commands

Do not let Digital Twin mutation directly become device authority.

Command path remains:

~~~text
User/Automation
 -> Control Plane policy
 -> Platform Action Gateway
 -> Device/Fleet provider
 -> observed actual state
 -> reconciliation
~~~

Twin platforms may transport desired properties/commands, but they remain an execution backend.

---

## 8. Twin divergence

Derived status:

~~~text
IN_SYNC
DRIFTED
STALE
UNKNOWN
UNREACHABLE
PARTIAL
~~~

Examples:
- expected Release differs from observed version;
- model/calibration differs;
- only Linux updated, MCU stale;
- device offline too long.

Divergence can trigger:
- Incident;
- Reconciliation Work;
- quarantine;
- rollout retry;
- diagnostic Procedure.

---

## 9. Multi-component twin

For robot/device products, twin should model a component graph:

~~~text
DeviceInstance
  Linux
  main MCU
  motor MCU
  dock relationship
  ML model
  calibration/config
  peripherals
~~~

Observed state can therefore express partial fleet/update states.

Do not reduce product state to one firmware version string.

---

## 10. Telemetry retention

Operational telemetry is not automatically long-term Evidence.

Policy selects:
- transient metrics;
- summarized observations;
- Evidence-worthy snapshots;
- Incident attachments.

High-volume raw telemetry stays outside core business DB.

---

## 11. Digital twin backend candidates

Potential integrations:
- Eclipse Ditto;
- ThingsBoard;
- Eclipse Kanto/device management;
- cloud/device-provider twins.

Strategy:
- consume observations and send governed actions through adapters;
- keep DeviceInstance/Release/Verification authority internal.

---

## 12. Manufacturing/RMA relationship

DeviceTwinProjection does not rewrite:
- ManufacturingResultReceipt;
- AS_BUILT;
- AS_RETURNED;
- RepairActionReceipt.

It provides field-observed context between those immutable lifecycle events.

---

## 13. M0/M1 impact

M0 reserves:
- ObservedDeviceState;
- DeviceTwinProjection read model;
- freshness/trust metadata.

M1 requires no IoT/twin platform.

---

## 14. M2/M3 impact

During fleet/device work:
- ingest actual versions/config;
- reconcile desired/observed state;
- detect drift/stale devices;
- select observations as Evidence when policy permits.

---

## 15. Invariants

1. Digital Twin is a read/projection model, not Release authority.
2. Desired and observed state are distinct.
3. Last-known state is freshness-bound.
4. Device self-report has explicit trust/provenance.
5. Command dispatch is not convergence.
6. Multi-component devices retain component-level observed state.
7. High-volume telemetry is not automatically business Evidence.
8. Immutable manufacturing/RMA history is never rewritten by twin state.
9. Drift triggers typed reconciliation/incident workflows.
10. Security-sensitive decisions may require attestation beyond ordinary twin telemetry.

## 16. Conclusion

The durable twin rule is:

> **A Digital Twin tells us what we currently believe about a device; immutable receipts, attestations and reconciled observations tell us what we can actually prove.**
