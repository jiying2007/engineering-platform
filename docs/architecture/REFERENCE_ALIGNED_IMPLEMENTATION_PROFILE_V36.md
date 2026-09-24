# Reference-Aligned Implementation Profile v36

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V35.md

Research basis:
- Open Source Reference Review Rounds 1–36
- Architecture v1.2

## 1. Goal

Profile v36 retains all v35 semantics and adds tamper-evident audit and disaster-recovery/reconciliation contracts.

No new mandatory M1 service is introduced.

---

## 2. AuditCheckpoint

Add immutable:

~~~text
AuditCheckpoint
~~~

Fields:
- event/audit scope;
- first/last sequence;
- event count;
- previous checkpoint;
- root/chained digest;
- canonicalization profile;
- signer/trust-root;
- created_at;
- optional transparency anchor ref.

Checkpoint closes a range but never replaces underlying events.

---

## 3. AuditIntegrityEvidence

Add:

~~~text
AuditIntegrityEvidence
~~~

Verifies:
- event digest integrity;
- ordering/sequence;
- checkpoint chain;
- signature;
- optional transparency-log inclusion/consistency.

Results:
- VALID;
- MISSING_EVENT;
- DIGEST_MISMATCH;
- ORDERING_ERROR;
- CHECKPOINT_MISMATCH;
- SIGNATURE_INVALID;
- INCONCLUSIVE.

---

## 4. Transparency anchoring

Optional future backends:
- Rekor/Rekor Tiles;
- Tessera-compatible transparency log;
- enterprise/internal append-only log.

Anchor only compact non-sensitive checkpoint/release digests where useful.

Never publish raw prompts/source/customer/security-embargo content merely for transparency.

---

## 5. BackupManifest

Add immutable:

~~~text
BackupManifest
~~~

Covers:
- PostgreSQL/PITR position;
- Artifact/Object-store snapshot/version refs;
- event/outbox checkpoint;
- trust/config/schema versions;
- workflow compatibility metadata;
- backup tool/version;
- checksums;
- retention.

One DB backup is not a complete authority backup.

---

## 6. RecoveryPoint

Add:

~~~text
RecoveryPoint
~~~

Defines intended coherent restore boundary across authority-bearing stores and records known consistency limits.

RecoveryPoint may be approximate across systems, but ambiguity is explicit.

---

## 7. RecoveryPlan

Versioned:

~~~text
RecoveryPlan
~~~

Defines:
- failure scenario;
- restore source/order;
- application/schema compatibility;
- trust/key recovery;
- service startup sequence;
- recovery mode;
- required external reconciliation;
- exit criteria;
- RPO/RTO if used.

---

## 8. RecoveryExecutionReceipt

Every drill/restore creates:

~~~text
RecoveryExecutionReceipt
~~~

Records:
- RecoveryPlan;
- RecoveryPoint;
- environment;
- actual components restored;
- integrity checks;
- timing;
- deviations;
- reconciliation;
- final status.

Recoverability becomes testable Evidence.

---

## 9. recovery_epoch

Every authority restore/recovery generates a new:

~~~text
recovery_epoch
~~~

All old:
- Session Grants;
- stale Worker mutation authority;
- pending privileged callbacks;
- execution leases

must fail mutation checks unless explicitly reconciled/reissued.

This complements execution_epoch.

---

## 10. RECOVERY_RECONCILIATION mode

After restore, Control Plane enters:

~~~text
RECOVERY_RECONCILIATION
~~~

Allowed:
- read;
- audit;
- reconcile;
- inspect;
- rebuild projections.

Blocked by default:
- production Release;
- provisioning;
- irreversible device flash;
- signing/promotion;
- ambiguous external retry.

Exit only after required reconciliation completes.

---

## 11. External world never rolls back automatically

Recovery reconciliation includes as relevant:
- Git merged state;
- CI state;
- Artifact registry;
- Release/OTA provider;
- actual Device state;
- manufacturing/provisioning receipts;
- KMS/signing operation receipts;
- notifications/external actions.

Restored domain history is amended with reconciliation events rather than pretending external actions never happened.

---

## 12. Workflow/orchestrator recovery

Temporal remains durable orchestration, not business authority.

After restore:
- workflows may be restored;
- or re-created/re-driven from authoritative aggregate/event state;
- ambiguous activities are reconciled before retry.

Domain state decides what should resume.

---

## 13. Backup qualification

Backup creation success is not enough.

Policy may require periodic:
- restore drill;
- checksum/integrity verification;
- application/schema boot;
- selected domain invariants;
- external reconciliation simulation.

Backup tools such as pgBackRest/restic are implementation candidates, not domain authority.

---

## 14. Credential boundary

Backup/restore credentials are highly privileged and separate from:
- Runtime;
- Worker;
- ordinary application role;
- ToolProfile.

Backup deletion, retention and restore require stronger authorization/audit.

---

## 15. Relationship to TLA+ / failure injection

Use v15 formal model and Toxiproxy/fakes to test:
- backup-era stale callbacks;
- lost external responses;
- restore after irreversible side effect;
- duplicate promotion prevention;
- old recovery epoch rejection.

Round 36 turns those failure semantics into explicit recovery contracts.

---

## 16. M0/M1 strategy

### M0
Freeze:
- AuditCheckpoint;
- AuditIntegrityEvidence;
- BackupManifest;
- RecoveryPoint;
- RecoveryPlan;
- RecoveryExecutionReceipt;
- recovery_epoch;
- RECOVERY_RECONCILIATION guards.

### M1
Implement:
- local signed/chained checkpoint;
- PostgreSQL + ArtifactStore backup/restore drill;
- stale external-side-effect recovery simulation.

No Rekor/Tessera/EventStore service required.

---

## 17. Final rules added by v36

1. Append-only and tamper-evident are different guarantees.
2. Audit checkpoint integrity never replaces detailed audit events.
3. Transparency anchors minimize sensitive content.
4. Backup scope covers all authority-bearing stores.
5. Restore generates a new recovery epoch.
6. Restoring storage never means external systems rolled back.
7. Recovery mode blocks irreversible actions until reconciliation.
8. Unknown external outcomes are reconciled before retry.
9. Backup is qualified by successful restore/drill.
10. Orchestration state remains subordinate to domain authority.

Architecture v1.2 remains canonical.
Profile v36 is the current implementation companion.
