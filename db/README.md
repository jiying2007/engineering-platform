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
