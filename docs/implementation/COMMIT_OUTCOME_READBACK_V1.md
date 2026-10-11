# PostgreSQL COMMIT outcome ambiguity and authoritative readback

## Safety boundary

After the database client has attempted `COMMIT`, a timeout, cancellation
or lost transport acknowledgement does **not** establish that the transaction
rolled back. The original operation may have committed in full, including
its audit event, Work/Run/Recovery transitions and Action idempotency key.

The canonical PostgreSQL authority labels such errors with
`ErrCommitOutcomeUnknown`. HTTP handlers return **503** with the fixed code
`commit_outcome_unknown` and the direction to **read back** authoritative
facts; driver errors, SQL and credentials are not reflected in the response.
This is not an HTTP 409/idempotency conflict, PASS receipt, or permission to
retry any external effect. Network loss before a response may prevent the
caller receiving this status; the same readback policy applies.

## Exact readback, not blind retry

- **Action**: read the exact operation ID and idempotency key/digest, Run and
  execution/recovery epochs, persisted state and audit history. A `PLANNED`,
  `DISPATCHED`, `UNKNOWN`, `RECONCILING`, `MANUAL` or historical
  `SAFE_TO_RETRY` cannot be interpreted as permission to redispatch.
- **Work/Task/Run/Checkpoint/Delivery/Evidence/Verification/Review/Closure**:
  read the same immutable ID, frozen subject/version/epoch and associated
  audit event. Reject conflicting duplicate identity. Do not generate a new
  subject or duplicate model turn merely because an acknowledgement is absent.
- **Recovery Begin/Complete/Proof**: read the current epoch/mode and retained
  Recovery Proof/audit state. Only the original two-principal protocol and
  recomputed facts authorize completion. Do not submit a new Recovery
  transition while the first COMMIT's outcome is ambiguous.
- **Pre-dispatch PLANNED abandonment**: use the exact authorized operation
  receipt readback. Any potentially effectful state remains blocked under
  P0 #228, irrespective of whether the previous POST response was lost.

Existing caller-bound PostgreSQL timeouts and separately bounded rollback
cleanup remain in place. The new error classification does not trigger any
automatic retry worker, schema migration or new authority. The tests exercise
classification and HTTP mapping; a real post-COMMIT lost-ACK fault and selected
host/provider long-run qualification are still tracked under P1 #229 and
external #103/#105/#106 and must not be claimed from CI fixtures.

## Worker and controlled maintenance transactions

The same typed unknown-COMMIT classification covers the original audited
transactions for Worker inbox claims/renewals/receipts, Offline and Codex
execution permits/renewals/results, Codex control queue/delivery/settlement,
Source Continuation, Checkpoint, Preparation, and execution reconciliation.
The relevant Worker HTTP error writer returns the same fixed 503 with no SQL
material. Callers must read back the *same* Run, execution/control token,
generation, Recovery epoch, retained receipt and audit before resuming; a
timeout must never mint a replacement permit or replay a model/Device/Git
operation.

Outbox retains its earlier separately specified
`outbox.ErrOutcomeUnknown` authority and independent settlement policy;
`schema_readiness`'s read-only transaction is not an effectful COMMIT.
No new ledger, schema change, external publisher access or automatic retry
is introduced.

The isolated PostgreSQL regression simulates loss of a successful ACK
*after* a real committed Recovery epoch/audit transaction, then demonstrates
exact readback and stale-epoch denial. It is **not** an actual TCP cut between
server COMMIT and client ACK and does not qualify production network behavior.
