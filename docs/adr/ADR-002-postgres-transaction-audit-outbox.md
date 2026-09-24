# ADR-002 — PostgreSQL Transaction Boundary, Audit and Outbox

Date: 2026-09-24
Status: **ACCEPTED**

## Context

Core mutations must eventually update several durable concerns:

- authoritative aggregate/projection state;
- append-only audit facts;
- integration/orchestration outbox.

Writing these independently would create a dual-write failure mode:

~~~text
business state committed
audit/outbox failed
~~~

or:

~~~text
audit/outbox committed
business state failed
~~~

Either result makes recovery and external side-effect reconciliation less trustworthy.

The in-memory bootstrap Store is deliberately simple, but the PostgreSQL implementation must freeze the production transaction boundary before more APIs are added.

## Decision

### 1. PostgreSQL owns the authoritative mutation transaction

Every authoritative Control Plane mutation executes one PostgreSQL transaction that, as applicable:

1. checks expected aggregate version;
2. checks execution/recovery epoch and state guards;
3. updates the authoritative projection;
4. appends one or more audit-event rows;
5. inserts outbox rows for post-commit work;
6. commits.

No network call or external side effect occurs inside this database transaction.

### 2. Optimistic concurrency is mandatory

Mutable aggregates use an explicit version.

Example:

~~~sql
UPDATE runs
SET state = $new_state,
    version = version + 1,
    updated_at = now()
WHERE run_id = $run_id
  AND version = $expected_version;
~~~

Zero affected rows means conflict/stale writer.

### 3. Audit event is part of the same transaction

A successful authoritative mutation without its audit event is invalid.

The event stores:
- event type;
- aggregate identity;
- payload digest;
- correlation/causation;
- hash-chain metadata as defined by the current audit profile.

Audit transport/export may be asynchronous.
Audit fact creation is not.

### 4. Outbox is part of the same transaction

If committed business state requires:
- Temporal signal/activity;
- Git/CI dispatch;
- notification;
- provider reconciliation;
- other asynchronous work,

the transaction inserts an outbox record.

Post-commit workers dispatch the outbox item.

### 5. External side effects are never part of the DB transaction

For a privileged external action:

~~~text
DB tx:
  validate + create PLANNED operation + audit + outbox
commit

outbox dispatcher:
  mark/claim dispatch
  call provider
  persist CONFIRMED or UNKNOWN in new DB tx
~~~

A timeout after provider invocation becomes UNKNOWN.

It is reconciled before retry.

### 6. Temporal is not business truth

Temporal owns:
- durable waits;
- timers;
- retry scheduling;
- activity orchestration;
- human/worker wait coordination.

PostgreSQL owns:
- Work/Task/Run state;
- versions;
- decisions;
- action ledger;
- authoritative audit/outbox.

Workflow replay cannot overwrite PostgreSQL authority.

### 7. In-memory bootstrap does not pretend to provide production atomicity

The current memory Store exists for:
- contract tests;
- API bootstrap;
- fast M0 development.

Its API should stay compatible with the PostgreSQL semantics, but it is not proof of crash-safe atomicity.

Production maturity requires integration tests against PostgreSQL.

## Consequences

Positive:
- no hidden business/audit dual-write;
- clear recovery model;
- outbox is replayable;
- Temporal remains replaceable orchestration;
- external effects stay reconciled.

Negative:
- repository/service methods need explicit transactional mutation boundaries;
- PostgreSQL adapter is more than CRUD;
- some current bootstrap APIs will be refactored behind application services.

## Implementation requirement

Before the first real M1 pilot, PostgreSQL integration tests must demonstrate:

1. optimistic conflict rejects stale mutation;
2. business state and audit commit atomically;
3. business state and outbox commit atomically;
4. transaction rollback leaves neither state nor audit/outbox partial;
5. outbox duplicate delivery is idempotent;
6. lost provider response creates UNKNOWN and reconciliation path;
7. recovery mode blocks irreversible outbox dispatch.

## Non-decision

This ADR does not require:
- event sourcing;
- Kafka;
- a separate audit database;
- a separate message broker;
- two-phase commit with external providers.

The design intentionally uses ordinary PostgreSQL transactions plus an outbox and reconciliation.
