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

- no ExternalOperation outside CONFIRMED or SAFE_TO_RETRY;
- no live non-OBSERVE outbox lease;
- no live Worker inbox lease;
- no offline execution in AUTHORIZED or UNKNOWN.

Expired offline AUTHORIZED is intentionally unresolved: the workload may have
started before its Worker disappeared.

The immutable proof binds the recovery epoch, reconciler identity, canonical facts
digest, audit-journal head and creation time. One proof exists per epoch.

## Double check on completion

The authenticated API first requires a proof for the exact epoch and rejects the
same identity that created it. PostgreSQL `CompleteRecovery` then opens a
serializable transaction, locks `platform_state`, reloads and validates the
proof, recomputes current facts, requires the same clear facts digest, appends the
completion audit event, and only then changes mode to NORMAL.

A stale proof therefore cannot authorize completion after new ambiguity appears.

## Migration v7

`0007_recovery_reconciliation.sql` adds only the immutable proof table and
schema-version marker. It does not fabricate historical proof rows or reclassify
operations.

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
NORMAL.

This proves logical authority restoration for the retained database state. It is
not a production backup policy, storage durability SLA, PITR test, encryption-key
recovery or cross-region disaster-recovery proof.

## CI provenance

The restore job is a required trusted-CI job. The final provenance envelope now
requires four upstream authority checks before it can be emitted:

- go;
- offline-container-integration;
- postgres-authority-restore-drill;
- codex-app-server-0.155.0-qualification.

A restore drill that is merely logged but absent from the trusted envelope is not
accepted as delivery evidence.

## Remaining boundary

A real external Action provider is still intentionally not wired into the
production entrypoint. UNKNOWN reconciliation semantics are durable, but a real
provider-specific observer/reconciler must be introduced together with that
provider. No generic stub is added here.

Real WIF model-turn proof, workspace-write Codex, interactive Action approvals and
retained Feature/Debug pilots remain separate gates.
