# Publication pre-invocation recovery — v1 and armed-cancellation v2

## Meaning and ordering

An Action's durable DISPATCHED row means the Core admitted a call, not that the
independent Publisher invoked Git/HTTP. This slice resolves only an original
**STARTED-v2 whose original handler never entered Remote.Publish**. Evidence v1
is inferred under the original lock before arming; evidence v2 is an explicit
original-writer cancellation certificate after arming, not a later inference. It reuses
the existing Action ledger, Recovery epoch and ABANDONED_RECONCILED non-success
state. There is no second queue, recovery worker, schema migration or retry.

The independent Publisher acquires an exclusive nonblocking flock on the
original slot directory inode **before** writing STARTED-v2. The start binds the
original identity and frozen plan plus a digest of kernel boot ID, device and
inode. Before Remote.Publish, it exclusively creates and fsyncs invoked.json and
its directory. The inode lock remains owned until the original handler finishes.
A cancelled caller or crashed process cannot authorize a later invocation from a
released lease. The original slot remains permanently occupied.

The Recovery endpoint acquires the **same inode lock**, without waiting or
killing the original process. It requires a valid v2 start, this same boot/device/
inode, and absence of both invocation and completion files. It exclusively writes
and syncs not-invoked.json. Any original retry/new Action ID/new idempotency key
for that execution still encounters the occupied slot and is refused.

The inference is not based on a remote ABSENT response. No remote Git/HTTP method
is called to manufacture no-effect evidence. A real SIGKILL test verifies that a
process dying before arming releases the lock; another live process is rejected.

## Boundaries that must not be weakened

- Version-1 starts remain readable only under the existing positive completion
  contract. They do not assert the newer lock/barrier order and cannot be sealed.
- Missing starts, copied/restored directories, another boot/device/inode and
  unsupported/network filesystems cannot mint this negative/pre-effect proof.
  Current local filesystem allowlist is ext-family, XFS, Btrfs, overlay and tmpfs;
  successful fsync is still required. tmpfs does not claim reboot persistence.
- An invoked.json without a valid original armed-cancellation certificate stays
  unresolved, including partial/malformed bytes and death before the actual call.
  A failed original Publish or timed-out HTTP mutation is NOT this certificate;
  no later observer can turn a stopped process or remote absence into one.
- A completed original publication uses the existing positive path, not this one.
- A whole live-VM/filesystem rollback that preserves identities, privileged record
  rewriting or compromised Publisher credentials is outside the local locking
  guarantee. Selected-host backup/quiescence policy is required; this mechanism
  is not independent attestation against an administrator.

## Cancellation after durable arming, before function entry

The original newly-created lease records in memory that its own invoked.json
write and both syncs succeeded. If that original caller then observes cancellation
before authorizing entry into Remote.Publish, it appends not-invoked.json with
kind PUBLISH_NOT_INVOKED_AFTER_ARM_V2 under the still-held inode lock. The record
binds the exact start and invocation digests and ordered timestamps. The caller
still receives an error, never a publication success receipt.

This append uses one independent five-second cooperative lifecycle. Failed writes,
partial bytes or failed syncs keep the slot occupied; no deletion, retry or detached
writer exists. Once entry is authorized, the in-memory flag is irreversible and
cannot certify non-invocation, even if the remote later returns an error or times
out. No certificate is reconstructed from an older marker after a crash.

Recovery only READS this original v2 certificate with its original invocation.
It cannot generate it. It still acquires the same inode lock and checks the same
boot/device/inode, strict private files and absence of a completion. Evidence v1
with an invocation attached, v2 without invocation, contradictory kinds/digests,
invalid time ordering, copied records or a live original holder are rejected.
A crash after intent but before certificate leaves an unresolved operation.

The existing API and non-success transaction are reused; evidence is explicitly
versioned rather than pretending the intent was absent. Earlier v1 JSON bytes,
receipt kinds and audit digests remain unchanged. A database-committed v2 receipt
can be backed up/restored like v1; a copied Publisher directory still cannot mint
fresh negative authority. This is not an implementation of error-after-entry or
ambiguous external-effect disposition.

## API and durable receipt

The independent Publisher accepts POST /v1/dispatch-no-invocation only from its
configured mTLS Core identity. Core derives the frozen plan and original Action;
operator requests cannot supply a witness, receipt, stopped PID or success claim.

Core endpoints:

- POST /api/v1/recovery/actions/abandon-uninvoked (recovery:reconcile)
- GET /api/v1/recovery/actions/{id}/uninvoked (read)

The POST uses the existing PublicationRecoveryRequest identity/epoch shape. A
serializable transaction locks platform_state and the original operation, then
rechecks RecoveryReconciliation mode/epoch, original identity, eligible unresolved
state, absent effect receipt and absent external success reference. It saves
RECOVERY_GITHUB_PUBLICATION_NOT_INVOKED_V1 in reconciliation_json and one audit
event recovery.github.publication.not_invoked. The old observed diagnostic is
retained in both row and receipt; original epochs, request digest and idempotency
key never change. receipt_json stays NULL; CONFIRMED is never manufactured.

GET, exact repeated POST, Recovery proof and final completion check strict receipt,
identity, timestamp, diagnostic and exactly matching audit digest. Exact repeat
needs no Publisher call, works after Recovery completion, and adds no audit.
COMMIT acknowledgement loss stays unknown until exact readback. The publication
reconciler cannot complete that Recovery epoch through another person's proof.

The settlement transaction performs this retained row/audit check before
deciding that an operation has no existing disposition. A partially restored
operation row cannot erase the meaning of a retained non-invocation audit event:
an unresolved state with a NULL reconciliation receipt and an existing audit is
a conflict. The transaction leaves both the row and audit count unchanged;
it does not append a second disposition and report success before later
readback discovers the contradiction.

The existing PLANNED-only abandonment endpoint remains narrower and unchanged;
it does not gain permission to abandon a DISPATCHED operation without this proof.

## Tests and acceptance

Required Publisher tests cover same-process and other-process locking, real
SIGKILL before intent, cancellation, replay/key substitution, v1/partial/armed/
foreign-slot rejection, copied snapshot rejection and real mTLS peer separation.
Database tests cover five unresolved states, original identity/no success/no
replay, insufficient evidence, corruption, audit rollback, three held locks,
concurrent settlement, real TCP COMMIT-ACK loss, final proof recheck and real mTLS.
A separate pg_dump/pg_restore drill retains the sealed Core receipt, audit and
independent completion rule. It does not claim a restored STARTED directory is a
fresh pre-effect proof.

Canonical CI requires nine database tests x3, each for both evidence versions,
fifteen required Publisher tests x3, and all
three explicit restore drills, with no missing/failing/skipped targeted cases.
The synthetic DB witness fixture is not a live GitHub operation. This slice does
not close all #228 historical/post-intent cases or #229 selected-host qualification.

## Implementation references

Linux flock(2) defines independent-open-file-description locks and release at the
last close; LOCK_NB rejects an active owner. Kernel sysctl documentation defines
random/boot_id as stable for the running boot. These are local lifecycle facts,
not remote request cancellation or proof of what a stale backup omitted.
