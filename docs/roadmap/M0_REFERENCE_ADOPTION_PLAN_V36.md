# M0 Reference Adoption Plan v36

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V35.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V36.md

## 1. Principle

Round 36 adds auditable backup/recovery semantics without adding a new M1 service.

M0 must prove that:
- audit history is tamper-evident;
- backup is coherent enough to restore;
- restored authority cannot blindly replay real-world side effects.

---

# Track A — Audit Integrity

## 2. AuditCheckpoint fixture

Create a small event stream and:
- compute canonical event digests;
- build a chained/Merkle-style AuditCheckpoint;
- sign checkpoint;
- verify signature/root.

Prove:
- event mutation is detected;
- event deletion/reordering is detected;
- checkpoint does not replace event storage.

---

## 3. Audit-integrity failure fixture

Create failures:
- changed payload;
- missing sequence;
- duplicate sequence;
- invalid signature;
- broken previous-checkpoint chain.

Generate AuditIntegrityEvidence for each.

---

# Track B — Backup

## 4. BackupManifest fixture

Capture:
- PostgreSQL backup/PITR position;
- MinIO/S3 object snapshot/version refs;
- schema/app version;
- audit checkpoint;
- outbox/event boundary;
- trust/config refs.

Prove:
- DB-only backup is marked incomplete for full-platform recovery.

---

## 5. RecoveryPoint fixture

Define one coherent RecoveryPoint.

Include:
- known timing gaps;
- object-store version;
- DB/WAL boundary;
- checkpoint.

Prove unknown consistency boundaries are explicit rather than guessed.

---

# Track C — Restore / Recovery Mode

## 6. RecoveryPlan fixture

Create one scenario:
- DB corruption/loss after an external Release side effect.

Plan includes:
- restore;
- startup in RECOVERY_RECONCILIATION;
- enumerate external operations after RecoveryPoint;
- compare actual external state;
- append reconciliation events;
- exit recovery mode.

---

## 7. recovery_epoch fixture

Create:
- pre-failure Worker session;
- restore;
- new recovery_epoch.

Prove:
- old Session Grant rejected;
- stale callback rejected;
- old privileged lease cannot mutate;
- fresh post-recovery command can execute only after authorization/reconciliation.

---

## 8. External-world reconciliation fixture

Simulate:

~~~text
backup at 10:00
external OTA succeeds at 10:15
DB lost at 10:20
restore 10:00
~~~

Prove:
- OTA is not blindly executed again;
- actual provider/device state is queried;
- DevicePromotionAttempt/Release state is reconciled;
- UNKNOWN remains explicit until confirmed.

---

# Track D — Restore Qualification

## 9. RecoveryExecutionReceipt fixture

Run one integration restore drill and capture:
- plan/version;
- point;
- restore duration;
- integrity checks;
- schema/app startup;
- reconciliation;
- result.

Use Testcontainers/dev infrastructure where possible.

---

## 10. Backup tool spike

Preferred M1 implementation candidates:
- pgBackRest for PostgreSQL;
- S3/MinIO versioning/object locking according to environment;
- restic only where file-level backup is needed.

Decision target:
- choose minimum set that supports tested restore.

No generic backup platform is required.

---

# Track E — Existing M0

## 11. Retain all v35 requirements

All previous M0 work remains:
- certification/privacy/prognostics/compatibility/threat-model fixtures;
- supplier/compliance/twin/FOSS;
- PLM/PSIRT/RMA/variants;
- AI governance/review;
- Device trust;
- ML lineage;
- Interface compatibility;
- ResolvedConfiguration;
- TraceLink;
- Incident/Knowledge;
- Run/Build/Integration/Release;
- OTel/CDEvents;
- formal model checking;
- failure injection.

---

## 12. M0 exit additions

M0 additionally requires:
- signed/chained AuditCheckpoint fixture;
- tamper-detection tests;
- complete BackupManifest;
- one tested RecoveryPoint;
- RECOVERY_RECONCILIATION guard behavior;
- stale pre-recovery authority rejected by recovery_epoch;
- one lost-response/external-side-effect restore scenario reconciled;
- one RecoveryExecutionReceipt from actual integration restore.

No Rekor/Tessera/EventStore production dependency is required.

---

## 13. M1 target

M1 remains the same trusted engineering loop, with the added requirement that its authoritative state and audit trail are demonstrably recoverable without duplicating real-world side effects.
