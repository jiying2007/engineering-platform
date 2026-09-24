# Outbox dispatch authority v2

Date: 2026-09-24. Baseline: `3e1663a53064b2292ca36c415afa1469c381b3fb`.
This implements the three outbox P0 findings in the retained Core review. It is
an in-process Core infrastructure component, not a new service or domain.

## Exact lease and time authority

A lease receipt contains `(outbox_id, lease_owner, attempt_count)`. The attempt
counter is a monotonically increasing generation on every claim, including an
expired lease reclaimed by the SAME worker ID. ACK, retry and dead-letter match
all three values, LEASED state and `lease_until > clock_timestamp()` in SQL.
Expired owners cannot mutate a message even before another worker claims it.
Counters are not reset by retry, settlement or migration.

The database is time authority. PostgreSQL `now()` is transaction-start time and
must not validate a lease after waiting for a row lock. Dispatch reads wall time
after acquiring locks and converts remaining duration conservatively into a
local context timeout, subtracting the entire timing-query round trip. Retry
eligibility is calculated from database wall time, not a host-supplied timestamp.
Claim batches are bounded to 1..1000, leases to 1 ms..24 h.

## Recovery and dispatch linearization

Claim remains `FOR UPDATE SKIP LOCKED` and now records `lease_recovery_epoch`.
The only dispatcher handler path is `Repository.Dispatch`:

1. Lock `platform_state` FOR SHARE and load current epoch/mode.
2. Lock and reload the exact outbox row FOR UPDATE. Match live owner/generation.
3. Compare its persisted risk/topic to the trusted handler registration. Missing
   state, unknown mode, mismatched risk and missing lease epoch fail closed.
4. For CONTROLLED_MUTATION/HIGH_RISK require NORMAL and the claim's exact recovery
   epoch. OBSERVE remains available during a valid recovery mode.
5. Invoke a cooperative bounded handler synchronously while holding authority.
6. Check cancellation, validate its result and settle the message together with
   `outbox.settled` audit in the same transaction, before releasing either lock.

If recovery commits first, a previously claimed mutation never reaches its
handler. Completing recovery does not revive an old-epoch lease. A fresh claim
is required after expiry. If dispatch acquires authority first, recovery's UPDATE
waits for that already admitted delivery/settlement. Concurrent Dispatch calls
for the same lease cannot both invoke the callback after a committed settlement.
No unlocked second-read TOCTOU gate or separate post-unlock ACK is used.

This is cooperative dispatch serialization, NOT atomicity between PostgreSQL
and arbitrary external systems. A dropped DB connection, process crash or
ambiguous COMMIT can leave a delivered message without a durable ACK. Transport
is at-least-once. A recipient MUST deduplicate by the stable outbox_key and use
its Action Gateway ledger/epoch authority for irreversible effects. A database
lock cannot recall an external operation already accepted by its provider.

## Handler contract and failure behavior

Registration requires an explicit RiskClass and Handler. Configure the registry
before concurrent use; no mutable receiver defaults are written by DispatchBatch.
The default lease is 30 seconds; the default per-message handler context is half
the configured lease. Store.Dispatch independently caps context at 30 seconds
and the remaining database lease; it applies that context through settlement,
audit and COMMIT. Backoff is exponential and capped; attempts include reclaims.

Handlers must honor cancellation, finish synchronously, emit sanitized errors,
and perform only bounded idempotent delivery. They must NOT detach goroutines,
run a long build/agent turn, mutate recovery/outbox rows, or re-enter transactions
which contend for this authority. Reserve connection capacity for downstream
work, or use a separate transport/pool; never exhaust a pool with callbacks that
wait for that same pool. Long work is submitted as a durable recipient intent,
not performed under this transaction.

Go cancellation is cooperative. The code does not claim it can kill an arbitrary
non-cooperative callback: it keeps ownership until that callback returns rather
than abandoning a live goroutine. Such handlers are not valid registrations.
Process isolation/termination and real Worker integration remain separate gates.

- Success commits DISPATCHED plus audit.
- Safe delivery failure schedules a database-timed retry; exhausted budget goes
  to DEAD_LETTER. Missing handlers are quarantined without calling anything.
- A recipient explicitly returning ErrOutcomeUnknown is quarantined with a
  reconciliation-required reason, never put back in the automatic retry queue.
- A transaction/cancellation failure after callback invocation returns
  ErrOutcomeUnknown and never reports success or immediately retries. The lease
  may later be reclaimed: recipient deduplication/reconciliation remains essential
  for crash/DB-outage ambiguity. This is not a substitute for the Action ledger.
- Authority denial is Deferred, not success or handler failure; no stale ACK is
  attempted. An explicit operator must resolve persistent registration errors.

The receipt-only Mark methods require the same live lease but do not invoke a
handler or authorize an external action. They are not evidence that a side effect
occurred; the supported dispatch path commits its own outcome and audit.

## Fail-closed classification and upgrade

`Classify` owns the small trusted Core topic registry: run.started is
CONTROLLED_MUTATION, work.created is OBSERVE. Known topics cannot be downgraded;
unknown topics must have a valid explicit class. Missing/invalid classification
rolls back business state, audit and outbox insertion together. A DB constraint
also rejects Core-topic downgrades, and the SQL OBSERVE default is removed.

Migration 0002 is additive and tracked under a transaction/advisory lock; 0001 is
not rewritten. Before upgrading an existing deployment, stop all legacy
owner-only dispatchers. Mixed-version dispatch and rollback to legacy executors
are unsupported. The migration quarantines every old LEASED message and unknown
queued topic, preserving its attempt count. Known PENDING Core topics are
reclassified; terminal historical evidence is not rewritten. The NOT VALID topic
constraint retains old evidence while enforcing all future inserts/updates.
Repeated migration does not reset leases, counters, terminal states or history.
Quarantined entries require risk assessment and external reconciliation before
any explicit redrive. No automatic redrive or destructive data deletion occurs.

No production database is modified merely by merging this source change.
Deployment/migration requires the operator-controlled application path.

## Retained regression proof

Unit tests cover concurrent default configuration, recovery-after-claim, risk
mismatch, timeout/no ACK, budget/backoff, missing handlers and explicit UNKNOWN.
PostgreSQL tests cover expiry before reclaim; same-worker old-generation ACK,
retry and dead-letter rejection; recovery-after-claim/old-epoch rejection; shared
lock conflict with recovery UPDATE; duplicate concurrent dispatch; delayed ACK
wall-clock expiry after row-lock wait; audit-failure rollback; risk rollback;
legacy schema upgrade/quarantine/history and idempotent migration replay.

CI retains full Go race tests plus PostgreSQL 17 and runs the outbox unit suite
20 times and focused real-DB authority tests 3 times. Merge evidence is the exact
PR head CI followed by fresh main CI, not an old marker or this document.

Official locking/time references:
- https://www.postgresql.org/docs/17/explicit-locking.html
- https://www.postgresql.org/docs/17/functions-datetime.html

## Remaining product gates

This does not wire an authenticated Worker, production handler, credential broker,
Context consumer, sandbox, Git/CI/artifact adapter or initialized Codex session.
The next bounded slice is trusted command-level assembly, with independent
recovery completion/evidence authority. Real Feature/Debug pilots remain required
for M1. Do not enable raw irreversible handlers on the strength of these tests.
