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

- no ExternalOperation outside CONFIRMED, SAFE_TO_RETRY or explicitly no-replay ABANDONED_RECONCILED;
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
migration before running binaries that require schema v13.

## Double check on completion

The authenticated API first requires a proof for the exact epoch and rejects the
same identity that created it. PostgreSQL `CompleteRecovery` then opens a
serializable transaction, locks `platform_state`, reloads and validates the
proof, recomputes current facts, requires the same clear facts digest, appends the
completion audit event, and only then changes mode to NORMAL.

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
