# Open Source Reference Review — Round 36: Tamper-Evident Audit and Disaster Recovery

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- Sigstore Rekor / Rekor Tiles
- Trillian / Tessera transparency logs
- codenotary immudb
- pgBackRest
- restic
- existing Domain Event / Audit Journal, External Operation Ledger and reconciliation semantics

## 1. Key conclusion

Append-only application logic is not enough for high-value audit.

The platform needs two distinct properties:

~~~text
Audit completeness
  did all authoritative events get durably recorded?

Audit integrity
  can we detect if historical records were altered/deleted/reordered?
~~~

Separately, backup availability is not enough:

~~~text
Backup exists
!=
system can be safely restored into a world where external side effects already happened.
~~~

---

## 2. AuditCheckpoint

Add immutable:

~~~text
AuditCheckpoint
~~~

Fields:
- aggregate/event stream scope;
- first/last sequence;
- event count;
- previous checkpoint ref;
- Merkle/root digest or chained digest;
- canonicalization/profile version;
- signer/trust-root ref;
- created_at;
- external anchor/transparency-log ref where used.

A checkpoint summarizes a closed event range without replacing individual events.

---

## 3. AuditIntegrityEvidence

Add:

~~~text
AuditIntegrityEvidence
~~~

Proves:
- event digests recompute;
- sequence/range is complete under defined scope;
- checkpoint chain is valid;
- external anchor/inclusion proof verifies where applicable;
- signer/trust root is valid.

Results:
- VALID;
- MISSING_EVENT;
- DIGEST_MISMATCH;
- ORDERING_ERROR;
- CHECKPOINT_MISMATCH;
- SIGNATURE_INVALID;
- INCONCLUSIVE.

---

## 4. Transparency log integration

Rekor/Trillian/Tessera demonstrate:
- append-only Merkle logs;
- inclusion proofs;
- consistency/integrity proofs;
- signed metadata anchoring.

### Strategy

Do not put all Run/PTY/domain events into a public transparency log.

Instead optionally anchor selected:
- AuditCheckpoint;
- Release Manifest;
- Trust-root change;
- high-risk Approval/Admission receipt;
- production provisioning summary;
- compliance package digest.

Potential backend:
- internal Tessera/compatible transparency log;
- Sigstore/Rekor-style external log where ecosystem/policy fits.

---

## 5. Sensitive audit separation

Audit Journal can contain sensitive IDs/metadata.

Transparency anchor should normally contain:
- digest;
- checkpoint metadata;
- timestamp;
- signer;
- minimal non-sensitive refs.

It should not expose:
- source code;
- prompts;
- customer/RMA data;
- secrets;
- embargoed ProductSecurityCase content.

---

## 6. Audit retention and legal/compliance hold

Audit retention is policy-driven.

Some event classes may require longer retention than raw logs.

Deletion/archival policy must preserve:
- checkpoint integrity;
- references needed to verify retained packages;
- legal/compliance holds.

If raw detail is expired, the platform must not claim it can still fully reconstruct content—only that retained checkpoints prove the integrity of the retained set according to policy.

---

## 7. BackupManifest

Add immutable:

~~~text
BackupManifest
~~~

Binds:
- authoritative PostgreSQL backup/PITR position;
- object/artifact store snapshot/version refs;
- configuration/schema versions;
- trust/key refs;
- workflow/orchestration state capture refs;
- created_at;
- backup tool/version;
- checksums;
- retention class.

One database dump alone is not a complete platform backup.

---

## 8. RecoveryPoint

Add:

~~~text
RecoveryPoint
~~~

Defines a coherent intended restore point across:
- database;
- event/outbox;
- ArtifactStore;
- audit checkpoints;
- secrets/trust metadata refs;
- Temporal/workflow compatibility metadata.

Not every storage backend can be restored to exactly the same instant.

The RecoveryPoint records known consistency boundaries.

---

## 9. RecoveryPlan

Add versioned:

~~~text
RecoveryPlan
~~~

Defines:
- failure scenario;
- backup source;
- restore order;
- key/secret recovery;
- service startup order;
- read-only/recovery mode;
- external-system reconciliation;
- exit criteria;
- RPO/RTO objectives where used.

A disaster recovery document with no exercised procedure is not sufficient evidence.

---

## 10. RecoveryExecutionReceipt

Every restore/drill produces:

~~~text
RecoveryExecutionReceipt
~~~

Records:
- RecoveryPlan version;
- RecoveryPoint;
- environment;
- actual restored components;
- timing;
- integrity checks;
- deviations;
- reconciliation results;
- failures;
- final status.

This is Verification Evidence for recoverability.

---

## 11. Recovery epoch

After authority-state restore, create a new:

~~~text
recovery_epoch
~~~

All pre-restore/pre-recovery Worker sessions, Action grants and stale callbacks are invalid for mutation.

This complements execution_epoch.

---

## 12. Recovery mode

Immediately after restoring an authoritative store:

~~~text
RECOVERY_RECONCILIATION
~~~

mode should:
- reject new irreversible external actions;
- reject production Release/Provision/Flash;
- suspend ambiguous workflows;
- allow read/audit/reconciliation actions;
- enumerate external operations after the restored point.

Exit only after required reconciliation completes.

---

## 13. Why restore is not rollback of the world

Example:

~~~text
10:00 database backup
10:15 production OTA executed successfully
10:20 database lost
10:30 restore 10:00 backup
~~~

The restored database may think OTA has not happened.

Therefore:
- do not re-run OTA blindly;
- query actual fleet/provider state;
- reconcile Release/DevicePromotionAttempt;
- append recovery reconciliation events;
- then resume.

This is one of the highest-risk distributed-state failure modes.

---

## 14. External-system reconciliation set

Recovery reconciliation should inspect as applicable:
- Git final merge/commit;
- CI external job/result;
- Artifact registry/object store;
- Release/OTA provider;
- Device observed state;
- manufacturing/provisioning station receipts;
- signing/KMS operations;
- issue/notification systems.

The exact set is defined by RecoveryPlan and External Operation Ledger.

---

## 15. Orchestrator restore

Temporal/workflow durable state should not become business authority.

Recovery strategy may:
- restore workflow storage where supported;
- restart/rebuild workflows from PostgreSQL authoritative aggregates/events;
- mark uncertain orchestration as reconciliation-required.

Domain state remains the basis for resumption.

---

## 16. Backup verification

pgBackRest/restic practices reinforce:
- checksums;
- integrity verification;
- encrypted/remote repositories;
- PITR/versioning;
- restore testing.

Add policy that backups are not considered qualified until restore has been tested on a defined cadence or after major schema/storage changes.

---

## 17. BackupCredentialBoundary

Backup systems require powerful read/write credentials.

Keep them separate from:
- Runtime;
- normal Worker;
- application credentials.

Backup deletion/retention/admin capability should use stronger authority and audit.

---

## 18. Schema/version compatibility

RecoveryPlan must specify:
- application version compatible with restored DB/schema;
- migration direction;
- rollback constraints;
- event schema reader compatibility;
- Artifact metadata compatibility.

A backup may be cryptographically valid but unusable by the current application build.

---

## 19. M0/M1 impact

M0 reserves:
- AuditCheckpoint;
- AuditIntegrityEvidence;
- BackupManifest;
- RecoveryPoint;
- RecoveryPlan;
- RecoveryExecutionReceipt;
- recovery_epoch / RECOVERY_RECONCILIATION semantics.

M1 should implement:
- signed/chained local AuditCheckpoint;
- one PostgreSQL + ArtifactStore backup/restore drill;
- one simulated stale external-side-effect reconciliation.

No transparency-log service is required.

---

## 20. M2/M3 impact

Add as risk/scale grows:
- external transparency anchor;
- stronger immutable/object-lock backup;
- multi-site backup;
- automated recovery drills;
- fleet/manufacturing reconciliation coverage.

---

## 21. Invariants

1. Append-only software logic and tamper-evident audit integrity are distinct.
2. Audit checkpoints never replace detailed authoritative events.
3. Sensitive data is not unnecessarily published to transparency logs.
4. Backup manifest spans all authority-bearing stores, not PostgreSQL alone.
5. Restore creates a new recovery epoch.
6. Restored database state never implies the external world rolled back.
7. Recovery mode blocks irreversible actions until reconciliation completes.
8. UNKNOWN external outcomes are reconciled before retry after restore.
9. Backup qualification requires tested restore, not only successful backup creation.
10. Workflow engine state remains subordinate to domain authority during recovery.

## 22. Conclusion

The durable recovery rule is:

> **Restore the records, then reconcile the world. Never assume recovering yesterday's database also reverted today's Git merge, device flash, production release or manufacturing action.**
