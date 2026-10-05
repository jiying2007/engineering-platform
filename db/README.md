# Core database

This directory contains the first PostgreSQL persistence contract for engineering-platform Core.

## Scope

The initial migration intentionally includes only M0/M1 Core data:

- platform recovery state;
- WorkItem;
- immutable TaskContract revisions;
- Run / RunAttempt;
- interactive Session / Steering;
- Checkpoint;
- External Operation Ledger;
- Artifact metadata;
- Evidence;
- Verification report;
- append-only Audit events;
- transactional Outbox;
- Worker registration.

It intentionally excludes Extension Catalog domains such as:
- PLM/MES/RMA;
- PSIRT;
- DPP/GS1;
- manufacturing/fleet;
- privacy/compliance;
- ML;
- certification.

Those schemas are introduced only by real extension milestones.

## Authority boundary

PostgreSQL is intended to own mutable business state and authoritative projections.

Temporal will own durable orchestration/waits/retries, not business truth.

Object storage owns immutable bytes.

Git/CI/Device/HIL remain external fact producers.

## Concurrency

The `runs.version` field is the optimistic concurrency token.

A PostgreSQL adapter should update a Run aggregate using a pattern equivalent to:

~~~sql
UPDATE runs
SET ...,
    version = version + 1
WHERE run_id = $1
  AND version = $expected_version;
~~~

Zero affected rows means optimistic-concurrency conflict.

## Recovery

`platform_state.recovery_epoch` and `recovery_mode` support the Core recovery invariant:

> after an authority restore, irreversible external actions remain blocked until real external state has been reconciled.

## Migration policy

- migrations are forward-only during M0 bootstrap;
- destructive migration requires explicit ADR once real retained data exists;
- no Extension table is added "for future use";
- every new table must name its real M1/M2 consumer.


## Current binary startup fence

The existing `Store.CheckWorkerSchema` pre-listen check requires the current
binary's full recorded migration set (0002 through 0011), not only the inbox
migration. Migration 0001 predates the ledger; do not invent a version-1 row.
Missing, duplicate and future entries reject startup. This deliberately refuses
an unqualified binary downgrade against a newer database.

A five-second, read-only repeatable snapshot also checks every required Core
relation is a permanent ordinary table in the current schema. Search-path
fallback, temporary shadows and views do not satisfy that contract. Columns
introduced on existing tables by later migrations must be present. The source
regression binds the table/ledger inventory to the embedded migrations, so a
new migration cannot silently omit its startup contract update.

This does not migrate, backfill receipts, repair data or change the Recovery
epoch. Production still requires `AUTO_MIGRATE=0`; an authorized operator must
quiesce all services, retain and verify a native backup, and explicitly migrate
before using a new binary. A failed check leaves the existing database unchanged.
Do not automatically downgrade, delete newer ledger entries or manufacture old
schema compatibility. Historical evidence remains unchanged.

Passing this minimum compatibility check is not proof of every column type,
constraint/index, role permission, data invariant or service readiness. It is a
point-in-time observation, not a fence against privileged DDL after startup.
Full upgrade/rollback and production restore qualification remain separate work.
