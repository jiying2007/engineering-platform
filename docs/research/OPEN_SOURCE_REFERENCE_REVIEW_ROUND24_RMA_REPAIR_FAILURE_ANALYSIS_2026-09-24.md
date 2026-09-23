# Open Source Reference Review — Round 24: RMA, Repair and Failure Analysis

Date: 2026-09-24
Status: **Archived research / implementation input**

References reviewed:
- ERPNext Warranty Claim / serial-number support
- Odoo Repairs / returns / after-sales flows
- existing Incident, DeviceInstance, ManufacturingResultReceipt and Knowledge lifecycle

## 1. Key conclusion

RMA/repair is neither manufacturing nor Incident alone.

The platform must preserve the physical unit's state across:

~~~text
AS_BUILT
 -> AS_SHIPPED
 -> AS_FIELD_OBSERVED
 -> AS_RETURNED
 -> AS_REPAIRED / AS_REPLACED
~~~

while keeping CRM/warranty/commercial workflows external.

---

## 2. ExternalServiceCaseRef

Add:

~~~text
ExternalServiceCaseRef
~~~

Fields:
- provider/system;
- warranty/repair/RMA/ticket ID;
- customer/account locator;
- lot/serial;
- warranty status;
- external status;
- external timestamps.

ERP/CRM/Helpdesk owns commercial/service workflow.

engineering-platform owns technical engineering evidence and state reconstruction.

---

## 3. ReturnReceipt

When a physical unit returns:

~~~text
ReturnReceipt
~~~

Binds:
- DeviceInstance;
- ExternalServiceCaseRef;
- return time/location;
- reported symptom;
- customer/field context refs;
- physical condition;
- tamper/seal state where relevant;
- initial firmware/config identity if readable;
- attachments/photos/logs;
- custody/handler.

It creates the start of the technical RMA investigation.

---

## 4. DeviceStateSnapshot

Add immutable:

~~~text
DeviceStateSnapshot
~~~

Kinds:
- AS_BUILT;
- AS_SHIPPED;
- FIELD_OBSERVED;
- AS_RETURNED;
- PRE_REPAIR;
- POST_REPAIR;
- AS_REPLACED;
- CUSTOM.

Snapshot may include:
- installed component Artifact versions;
- bootloader;
- configuration/calibration;
- hardware revision/substitutions;
- serial/lot subcomponents;
- trust state;
- fault counters;
- environment/service data refs.

A repair must never erase the AS_RETURNED state.

---

## 5. FailureAnalysisRecord

Add:

~~~text
FailureAnalysisRecord
~~~

Includes:
- DeviceInstance;
- ReturnReceipt;
- observed symptoms;
- reproduction attempts;
- measurements;
- teardown observations;
- suspected causes;
- replaced parts;
- destructive-analysis flags;
- hypotheses;
- root-cause Decision ref;
- raw Evidence.

Use existing InvestigationHypothesis / RootCauseDecision semantics.

---

## 6. RepairActionReceipt

Every repair action creates immutable:

~~~text
RepairActionReceipt
~~~

Records:
- DeviceInstance;
- replaced/reworked parts;
- old/new serialized component refs;
- firmware/config/calibration changes;
- Procedure;
- station/technician/service identity;
- tools/fixture;
- timestamps;
- outcome;
- generated Evidence.

This creates a full technical service history.

---

## 7. Part replacement lineage

For serialized or controlled parts:

~~~text
old component
 -> removed from DeviceInstance
new component
 -> installed in DeviceInstance
~~~

Record:
- part revision;
- supplier/lot/serial where available;
- reason;
- disposition of removed part;
- compatibility approval.

After repair, the device may no longer match its original ManufacturingResultReceipt exactly.

That difference is intentional and traceable.

---

## 8. Repair configuration identity

Repair may alter:
- firmware;
- bootloader;
- calibration;
- feature/config;
- hardware part revision;
- security credential.

Therefore POST_REPAIR DeviceStateSnapshot must define the exact resulting technical state.

"Repaired" is not enough.

---

## 9. Requalification after repair

Policy may require:

~~~text
Repair
 -> Post-repair Procedure
 -> Evidence
 -> RepairVerification
 -> Service disposition
~~~

Depending on changed parts:
- quick functional test;
- calibration;
- full production test;
- security/provisioning check;
- HIL regression.

Repair completion in ERP does not imply engineering requalification PASS.

---

## 10. RMA disposition

Technical disposition examples:

~~~text
NO_FAULT_FOUND
REPAIRED
REPLACED
SCRAP
RETURN_TO_VENDOR
ENGINEERING_ANALYSIS
QUARANTINE
~~~

Commercial/warranty disposition remains external.

---

## 11. No-fault-found is a real result

NO_FAULT_FOUND should retain:
- attempted reproduction;
- test scope;
- environment;
- observed device state;
- Evidence.

It is not equivalent to "customer report invalid".

Repeated NFF cases can become an Incident/quality signal.

---

## 12. Fleet / population correlation

Aggregate RMA cases by:
- TargetRevision;
- Release;
- hardware revision;
- supplier lot;
- component part;
- factory/station;
- calibration range;
- geography/environment;
- symptom/failure signature.

Derived analysis may reveal:
- supplier lot issue;
- design weakness;
- firmware regression;
- manufacturing station issue.

Correlation creates InvestigationHypothesis, not automatic Root Cause.

---

## 13. Corrective engineering feedback

Confirmed failure may trigger:
- Incident;
- ProductSecurityCase if security-relevant;
- EngineeringChangePackage;
- software Work;
- supplier action;
- production recipe change;
- new TestDefinition;
- KnowledgeCandidate.

RMA is therefore an important field-feedback source.

---

## 14. Repairability and supportability

Future product policy may define:

~~~text
ServiceabilityPolicy
~~~

including:
- repairable assemblies;
- replace-only parts;
- calibration requirements;
- secure credential handling;
- supported software versions;
- post-repair test level;
- warranty constraints;
- data/privacy handling.

This is optional and product-specific.

---

## 15. Privacy/data handling

Returned devices may contain:
- user data;
- Wi-Fi credentials;
- logs;
- recordings/images;
- account tokens.

RMA Procedure must define:
- data access scope;
- consent/legal basis as applicable;
- sanitization;
- retention;
- redaction;
- secure wipe.

Engineering debugging must not silently copy unrestricted user data into ordinary Artifact/Evidence stores.

---

## 16. M0/M1 impact

M0 reserves:
- ExternalServiceCaseRef;
- ReturnReceipt;
- DeviceStateSnapshot;
- FailureAnalysisRecord;
- RepairActionReceipt;
- optional ServiceabilityPolicy.

M1 requires no repair/RMA integration.

---

## 17. M2/M3 impact

When field returns begin:
- integrate service/RMA system by typed refs;
- capture AS_RETURNED state before modification;
- connect repair/requalification to Device history;
- mine repeated failures into Incident/Engineering Change/Knowledge.

---

## 18. Invariants

1. External warranty/RMA ticket is not technical engineering authority.
2. AS_RETURNED state is immutable and preserved before repair.
3. Repair actions are append-only technical receipts.
4. Component replacements preserve old/new part lineage.
5. Post-repair Device state is explicitly re-established.
6. Repair closure may require engineering requalification.
7. NO_FAULT_FOUND retains reproduction/test evidence.
8. RMA correlations propose hypotheses; they do not decide root cause.
9. Repair can trigger Engineering Change/Incident/Knowledge through typed workflows.
10. Returned-device user data follows explicit privacy/retention policy.

## 19. Conclusion

The durable RMA rule is:

> **Never repair away the evidence. Preserve the returned state first, record every technical change, then prove the resulting device state before service closure.**
