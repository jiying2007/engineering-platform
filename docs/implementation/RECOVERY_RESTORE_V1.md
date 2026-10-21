# Recovery reconciliation and PostgreSQL restore drill v1

Base: `747114948bfa35cf07cafcebdf1e32b7e0341047` (#46).

This increment makes recovery completion depend on retained database facts rather
than a caller-supplied boolean, and makes PostgreSQL restore correctness a
mandatory CI fact.

## Recovery authority

The external `reconciled` request member is removed. Production completion now
requires two authenticated roles:

- a dedicated `recovery:reconcile` principal that may hold only that capability
  and optional `core:read`;
- a distinct `recovery:complete` principal.

`POST /api/v1/recovery/proofs` does not accept reconciliation facts from the
caller. PostgreSQL computes them while the exact recovery epoch is locked.

A proof is created only when all machine-observable ambiguity is clear:

- no ExternalOperation outside CONFIRMED or explicitly no-replay ABANDONED_RECONCILED;
- no live non-OBSERVE outbox lease;
- no live Worker inbox lease;
- no offline execution in AUTHORIZED or UNKNOWN.

Expired offline/Codex AUTHORIZED and explicit UNKNOWN executions are
intentionally unresolved: the workload or model turn may have started before its
Worker disappeared. Migration v12 adds an explicit operator reconciliation path:
a dedicated recovery reconciler may, only while the exact Recovery epoch is
active and after the execution lease has expired, retain a
`RECOVERY_EXECUTION_ABANDONMENT_V1` receipt bound to exact Run/execution identity
and an externally retained observation digest. The execution becomes
`ABANDONED_RECONCILED`, never FINISHED; no result, Delivery, replay or execution
authority is created. Exact retry is idempotent, while changed observation bytes
or a different reconciler are rejected. The durable execution row remains, so
the one-Run reservation still prevents replay.

The immutable proof binds the recovery epoch, reconciler identity, canonical facts
digest, audit-journal head and creation time. One proof exists per epoch.

### Retained Recovery Proof integrity

Proof readback, completion authorization and the serializable completion
transaction use the same strict projection. The requested epoch, complete proof
JSON and independent SQL columns must agree on epoch, reconciler, facts digest,
captured audit head and creation time. Every proof member and all four facts
counters must be present and non-null; a missing counter or `facts: null` cannot
be decoded into clear zero-valued facts. Unknown JSON members are rejected.

The captured audit sequence locates the original
`recovery.reconciliation_proved` event at the following sequence. Its type,
aggregate, canonical proof payload digest, previous digest, event digest and
creation time must match. When the captured sequence is nonzero, the head event
at that sequence must still exist with the bound digest. Only that predecessor's
sequence and digest are read; the proof does not introduce a new size limit on
unrelated historical aggregate IDs or a full audit-chain verification contract.
One SQL statement reads the proof and at most two audit rows by their sequence
primary keys, so a
standalone GET also observes a single snapshot. Later audit entries or Recovery
epochs do not replace the original binding or prevent valid historical reads.

Missing or inconsistent retained material blocks all three paths; completion
keeps Recovery active and writes no completion event. The implementation does
not regenerate proof JSON, rewrite audit history or synthesize missing evidence.
These checks detect inconsistent restores and one-sided corruption. They do
not establish an independent trust root against an administrator able to rewrite
all authoritative records and audit history consistently.

## Migration v13 — Recovery of pre-dispatch PLANNED Actions only

An Action may crash with a durable PLANNED reservation but before any
DISPATCHED transition. The Core's only permitted call to an external Provider
occurs **after** a successfully persisted DISPATCHED transition. The existing
#226 PostgreSQL guard locks Recovery/Run/Session when granting that transition.

Migration `0013_action_planned_reconciliation.sql` therefore adds a
Recovery-only, non-success `ABANDONED_RECONCILED` Action state and immutable
`reconciliation_json`. The old idempotency key, Run/execution/recovery epochs,
source request digest and audit history remain intact. The state is not
accessible through normal Action Transition or generic Update.

A separate mTLS `recovery:reconcile` identity may invoke:

- `POST /api/v1/recovery/actions/abandon-planned`
- `GET /api/v1/recovery/actions/{id}/abandon-planned` (`core:read`)

The POST accepts version 1, exact operation/Run/execution/original-Recovery
identity, the **current** Recovery epoch, original idempotency key/request
digest, an independently retained operator observation digest, and the fixed
`ABANDON_NO_REPLAY` disposition. Under a single bounded serializable
transaction, PostgreSQL locks the current Recovery row before the exact
Action row. Only an effect-free `PLANNED` row without external/receipt
material may transition, accompanied by its audit event. A duplicate with
exact same bytes and principal returns the same stored receipt; conflicting
identity, observation or actor is rejected. The read-only GET works after
Recovery completion, including when the response to the original POST was
lost after commit.

**PLANNED is the only state for which the database can establish that this
Core-mediated Provider path was never dispatched.** A caller-supplied
observation digest alone is NOT independent proof of external absence.
`DISPATCHED`, `UNKNOWN`, `RECONCILING` and `MANUAL` cannot use this path,
even with a matching observation digest. A prior nonterminal in-flight
publication may still have produced a remote effect; only provider-specific
external readback and an independent quiescence/settlement procedure can
resolve it. Those cases remain internal P0 #228.

The non-success receipt explicitly forbids effect-confirmation, execution,
replay and production-readiness claims. Recovery Proof counts it as closed
only for the **specific pre-dispatch reservation**; every other unresolved
Action continues to block. Recovery completion still requires a separate
authenticated principal and retained proof of the exact epoch. A successful
local fixture/CI is not proof of production, publisher, provider or operator
qualification. Existing installations require explicit quiescence and
migration before running binaries that require the current schema v14.

### Retained PLANNED receipt integrity

Readback, exact repeat, Recovery facts, operational status, completion
authorization and completion validate the same retained projection. The Action
row, strict receipt and unique original abandonment audit event must agree on
the original operation, Run and epochs, idempotency key, request digest,
observation, disposition, actor and timestamps. The audit payload digest is
recomputed. Missing, duplicate, orphaned or mismatched original evidence blocks
the read/proof; the implementation does not reconstruct missing history.

Validation includes retained dispositions from earlier Recovery epochs. A new
aggregate proof does not replace their original actors: the completion identity
must remain distinct from each original PLANNED reconciler for the same Recovery
epoch even if roles have subsequently changed. The database remains the pre-dispatch authority; this
check does not establish absence of effects for a previously dispatched Action.

## Migration v14 — bounded provenance lookups

`0014_external_operation_provenance_index.sql` adds the partial
`audit_external_operation_provenance_idx` index for ExternalOperation provenance
lookups. It changes neither Action states nor retained receipt/audit bytes and
does not backfill, repair or reclassify historical records. Apply migrations
with services quiesced under the existing bounded migration procedure before
starting the v14 binary. Index construction needs the maintenance window; this
is not an online concurrent-index rollout.

Provenance counts share a single aggregate scan and correlated audit lookups use
the index. All relevant historical operations are still validated; no newest-N
window, evidence cache or history truncation is introduced. The growth regression
records actual query plans, checks index validity, and runs
500/2,000/5,000-operation histories under the existing five-second store budget,
then corrupts the oldest retained evidence and requires
rejection. Those sizes are measured regression fixtures, not a production
capacity guarantee.

## P0 remote absence does not prove Publisher quiescence

An instantaneous GitHub `ABSENT` ref/PR observation is not a retained proof
that a timed-out original Publisher cannot finish its remote effect later.
Even a byte-exact local Git bundle proves only source integrity. Accordingly,
Publisher reconciliation returns `MANUAL / ABSENT_DISPATCH_NOT_QUIESCED`,
not `SAFE_TO_RETRY`; it does not execute another Git mutation.

Historical `SAFE_TO_RETRY` rows remain unresolved in Recovery Proof, and
operational status v5 explicitly counts and degrades on those rows.
The normal Action state machine and PostgreSQL ledger refuse new transitions
into `SAFE_TO_RETRY` or from historical `SAFE_TO_RETRY` to `DISPATCHED`.
Tests seed such a historical state only with isolated PostgreSQL fixture SQL;
this is not production state-transition authority. All
idempotency keys, original Action epochs and audit history are retained; there
is no migration, fabricated confirmation, or auto-retry. Only a separately
qualified provider-specific original-dispatcher quiescence and immutable
external observation could support an eventual disposition. Such real
effectful reconciliation is still P0 #228 and is not proven by these fixtures.

## Double check on completion

The authenticated API first requires a proof for the exact epoch and rejects the
same identity that created it. PostgreSQL `CompleteRecovery` then opens a
serializable transaction, locks `platform_state`, reloads and validates the
proof, recomputes current facts, requires the same clear facts digest, appends the
completion audit event and changes mode to NORMAL atomically.

A stale proof therefore cannot authorize completion after new ambiguity appears.

## Migrations v7 and v12

`0007_recovery_reconciliation.sql` adds only the immutable proof table and
schema-version marker. It does not fabricate historical proof rows or reclassify
operations.

`0012_execution_reconciliation.sql` extends only the existing offline/Codex
execution ledgers with the non-success `ABANDONED_RECONCILED` state and an
immutable reconciliation receipt. It does not delete the original reservation,
invent a result, authorize retry, or reclassify historical rows.

## Restore drill

The mandatory `postgres-authority-restore-drill` CI job uses PostgreSQL 17 for
both server and client tooling. It creates an isolated source database, applies
all migrations, then seeds a real authoritative chain:

Work -> Task -> Run -> Delivery -> Evidence -> Verification -> Review -> Closure,
plus a CONFIRMED external operation, audit/outbox state, active recovery epoch and
a reconciliation proof.

The job performs a PostgreSQL 17 custom-format `pg_dump`, restores it into a new
database with `pg_restore --exit-on-error`, and compares a canonical authority
snapshot covering Core records, Action, Review/Closure, recovery proof,
audit-journal head, migrations and outbox state.

Finally it uses the restored proof with a different completion identity and
actually transitions the restored database from RECOVERY_RECONCILIATION to
NORMAL. The original reconciler remains unable to authorize that completion.
The drill then begins a later Recovery epoch and, only in its isolated restored
database, transplants the genuine older proof JSON into the new proof row while
retaining the new row's columns and all original audit events. Readback,
authorization and completion must reject that cross-epoch mismatch without
leaving Recovery or appending a completion event. The original restored proof
must remain readable with its original digest.

The required PLANNED restore scenario separately compares the original receipt,
proof and audit head after a real dump/restore and deletion of the source schema.
It completes the restored epoch, begins a later Recovery epoch, and verifies
that corrupting the old operation binding blocks readback, status, authorization
and completion. The event-stream verifier requires this scenario to execute
and pass; a missing or skipped test cannot satisfy the restore gate.

This proves logical authority restoration for the retained database state. It is
not a production backup policy, storage durability SLA, PITR test, encryption-key
recovery or cross-region disaster-recovery proof.

## CI provenance

The restore job is a required trusted-CI job. The final provenance envelope now
requires four upstream authority checks before it can be emitted:

- go;
- offline-container-integration;
- postgres-authority-restore-drill;
- codex-app-server-qualification.

A restore drill that is merely logged but absent from the trusted envelope is not
accepted as delivery evidence.

## Remaining boundary

The Publisher-specific paths accept only their exact retained completion or
non-invocation proofs. Other unresolved external Action effects still require a
qualified provider-specific observer/reconciler and original-dispatcher
quiescence evidence. No generic terminal override is introduced here.

Real WIF model-turn proof, workspace-write Codex, interactive Action approvals and
retained Feature/Debug pilots remain separate gates.

Retained-proof consistency and successful local/CI restore fixtures do not close
internal blockers #228/#229 or the real unattended qualification gate #105.
