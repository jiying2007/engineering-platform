# PostgreSQL authority operation budgets

Scope: the PostgreSQL Store API at base `93b9b8a209159692cb87b91e303388304722afc1`.
This is a caller-lifetime correction, not a new task service, database, schema,
retry policy or production qualification. The live readiness declaration remains
only in IMPLEMENTATION_STATUS.md; internal blockers [228,229] remain OPEN.

## Gap and policy

Server-side statement and lock timeouts start only after a pooled connection has
been obtained. A live HTTP/service caller may carry cancellation but no deadline.
Previously, several public reads and authority transactions could wait on an
exhausted pool for that caller's entire lifetime. The existing pool-wait test
supplied its own100ms deadline, so it could not detect this missing-default case.

`authorityContext` inherits the exact caller's cancellation and values. When the
caller has no deadline it creates one child budget using the existing20-second
statement safety ceiling. A supplied deadline is returned unchanged, including a
longer deliberately bounded administrative budget. Nested Store operations see
the already-bounded Context and cannot replenish the budget. Every owner defers
its cancel function; inherited Contexts use a no-op cancel owned by that wrapper,
not the parent's cancel function.

| Public entry policy | Count | Behavior |
| --- | ---: | --- |
| Missing-deadline fallback | 47 | Core/Action reads and mutations, recovery proof, Outbox single operations, continuation reads, Open and migration; preserve explicit caller deadlines |
| Existing caller-derived operation timeout | 36 | Worker/Offline/Codex/recovery5s and Outbox30s policies remain unchanged |
| Existing finite relay batch | 1 | At most64 steps, each under the original5s child and cancelled immediately; existing independently bounded rollback remains |

The fallback uses an existing safety constant rather than introducing a new
configuration flag or selecting a short timeout for every operation. Explicit
startup/migration, worker, recovery and post-Provider settlement budgets retain
their existing meaning. A single operation's deadline spans connection acquisition,
all SQL phases, audit and attempted COMMIT. The pool's background connection
maintenance lifetime is not replaced by that operation deadline.

## Outcomes and ownership

Before acquisition or admission, expiration must return the original deadline
error without SQL or external dispatch. After a COMMIT attempt it remains subject
to the existing unknown-outcome classifier and exact-ID/audit readback; expiration
is not a rollback certificate. Original post-Provider settlement retains its own
5s budget and never repeats the Provider call. Transaction rollback retains its
existing independently bounded cleanup. No idempotency reservation is released.

Context deadlines are cooperative. They do not promise that an arbitrary blocked
kernel call can be killed at an exact instant, nor that a callback which ignores
Context can be forcibly interrupted. Explicit operation policies must be reviewed
separately from selected-host throughput, storage failure and soak acceptance.

## Permanent regression contract

The existing go CI job, with PostgreSQL17, executes six named tests under race:

* The deadline helper preserves values/cancellation, pre-cancelled callers,
  explicit shorter/longer deadlines and nesting without resetting or cancelling
  its parent's timer.
* The AST guard enumerates every public Store method with Context (plus Open),
  requires a same-caller budget and matching deferred cancel before Context-taking
  calls, and pins the single reviewed finite-relay implementation. Its negative
  fixtures reject absent, detached, unreleased, unused-nested and late timers.
* A declaration-aware AST guard inventories every direct SQL Context argument;
  only the actual caller parameter or its derived/aliased child is accepted.
  SQL method-value aliases cannot evade it. The existing independent rollback
  is the sole explicitly reviewed exception; field-based and unreviewed origins
  are rejected. This is a SQL-boundary check, not dynamic points-to proof.
* The real saturated-pool test holds the only connection while seven independent
  no-deadline operations wait concurrently: Work read/write, Recovery read/begin,
  Action read/idempotency/admission. The actual20s fallback, not a test-only timer,
  must expire while the connection is still held. It records elapsed times,
  requires no new authority/audit/external rows or Provider call, and proves the
  pool can be reused afterward. The5s watchdog margin is not an SLO.
* The real mutation test observes one finite inherited deadline across SQL phases,
  cancels the caller before audit/COMMIT and verifies no audit residue.

The complete-event validator requires all six names, correct package start/end
and zero missing, skipped, failed or unfinished tests. Source/selector alignment
is separately tested. Existing caller-deadline, server-side deadline, lock, lost
COMMIT ACK, recovery and restore gates are retained, not replaced.

The source guard is a concrete entry-boundary check, not a universal call-graph
proof. Private transaction helpers receive their owning entry's Context; the
bounded relay owns its original per-step Context. Native fail/pass evidence and
exact-head/fresh-main CI qualify this code slice. Local PostgreSQL18 evidence
cannot substitute for PostgreSQL17 or selected-provider/host evidence.
