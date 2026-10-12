# Positive publication recovery with original completion evidence

Base: `a9a73d36ff04811d26e0960f02058a4189c63b85` (#267). This is a new
repository implementation, not live GitHub, selected-host, or production
qualification. Internal readiness remains BLOCKED; #228 and #229 are not closed.

## One supported automatic disposition

The original independent Publisher has finished its synchronous Git/HTTP
publication successfully, validated the complete frozen publication receipt,
and durably recorded completion, but Core did not durably settle that original
Action. During a later Recovery epoch an authorized reconciliation principal can
ask Core to retain a fresh positive readback and settle that *same* Action as
CONFIRMED. Core never calls Publish from this recovery route.

The completion witness means the **original successful invocation returned past
all mutating Git/HTTP calls**. It is not a guessed PID-death observation, an
operating-system process-tree stop proof, or proof of remote absence. An exception,
timeout, cancelled or crashed invocation cannot write the successful completion
record. A later positive observation alone cannot synthesize the missing record.

## Independent Publisher evidence

Service configuration v2 requires a canonical pre-created private 0700
`witness_root`, disjoint from Worker-visible `artifact_root`. The canonical
systemd template creates this state directory with mode 0700 and grants writes
only to that directory (artifacts remain read-only). Direct in-process pilot
configuration is not accepted as independent recovery evidence.

Each repository + original Codex execution has one exclusively allocated witness
slot. `started.json` is written and synced before invoking Publish.
`completed.json` is written and synced only after a valid successful original
return. Both records are exclusive-create, size-bounded regular files with mode
0600. Exact canonical bytes, original root/directory identities and start digest
are checked on readback. Root/directory symlinks and incomplete records fail
closed. Readback re-establishes file/directory durability rather than assuming a
previous failed fsync succeeded. Slots have no delete/retry/reset endpoint.

Duplicate requests, changed Action IDs or idempotency keys, and process restarts
cannot reuse an occupied slot to publish. Partial allocation/writes remain
occupied. The witness store is not another Core ledger, task queue or scheduler:
it can deny duplicate mutation and supply evidence, never grant Core execution.

`POST /v1/dispatch-evidence` accepts only the frozen plan and original Action
identity from the authenticated Core publisher client. It first reads the
original successful completion, then performs Observe, and requires the exact
same positive PR number, URL, branch, base and result. ABSENT, PARTIAL, CONFLICT,
replacement/closed PRs, malformed responses and unavailable upstream all fail
closed. The endpoint never calls Publish or accepts caller-supplied stop flags.

## Core transaction and capabilities

`POST /api/v1/recovery/actions/confirm-publication` requires the existing
`recovery:reconcile` capability. Input is only version, exact original Action
identity and current Recovery epoch. Caller-uploaded evidence/receipts are
rejected by strict JSON decoding. Core re-derives the plan from its frozen
engineering result and fetches evidence itself through the configured mTLS
independent Publisher client. Missing local bundles do not prevent this read.

The serializable PostgreSQL transaction locks platform state then the original
external operation. It requires current RECOVERY_RECONCILIATION, exact original
reservation/action/risk/capability, an eligible original state and empty existing
receipt/disposition slots. It stores the full typed evidence and its digest in
the existing `external_operations.receipt_json` column, writes CONFIRMED plus
the actual observed publication receipt, and appends exactly one audit event in
the same transaction. No schema migration or new normal Action transition is
added. Planned-only ABANDONED_RECONCILED and its schema-v13 reconciliation_json
constraint are unchanged. The original epochs and idempotency key never change.

Eligible potential-effect states are DISPATCHED, UNKNOWN, RECONCILING, MANUAL
and historical SAFE_TO_RETRY, **only with this exact positive completion**.
PLANNED, already abandoned, unrelated/modified identities, missing completion,
unavailable frozen results, and stale Recovery requests do not enter this path.

Lost COMMIT acknowledgement retains the existing typed unknown-COMMIT error.
`GET /api/v1/recovery/actions/{id}/publication` returns the exact stored result
and verifies it against the original row, observed receipt, recorded time and
exactly one matching original audit payload digest. A valid self-hashed receipt
is insufficient when it disagrees with that journal event. Exact-repeat SQL and
Recovery facts use the same validator, including before completion of a previously
created aggregate proof. Missing rows/receipts/events, inconsistent observations,
unknown receipt fields and altered self-consistent receipts stay blocked.
This is mismatched-restore/corruption detection, not cryptographic protection
against an administrator replacing the entire database and journal. A repeated HTTP
request returns the saved historical receipt without another observation/write;
it does not assert the remote PR is still open at the time of the repeat.
Conflicting request/principal is denied. The principal who reconciled a
publication cannot complete that same Recovery epoch, even if another principal
created its aggregate proof. Original aggregate Recovery proof and completion
checks remain mandatory; this transaction does not automatically complete them.

## Explicit remaining boundary

Historical operations without these witnesses and crashes before completion
remain unresolved. Negative/partial external evidence cannot be turned into
success or replay authorization. Extending recovery to those cases still needs
provider-specific original-dispatch/process evidence and independent policy
acceptance; this implementation does **not** relabel that work as human sign-off.
No live account/model/provider call, target deployment, destructive witness GC,
second execution, or independent human approval is performed by these tests.

Filesystem cancellation is cooperative between calls. A Context deadline cannot
forcibly interrupt a blocked kernel filesystem syscall. No goroutine is detached
to manufacture a bounded return. Storage failures leave a non-reusable slot and
an unresolved original operation. Witness backup/retention is part of selected
host acceptance; do not put this root under Worker artifact cleanup.

## Acceptance

Native tests cover actual filesystem records, restart, partial/null/symlink
records, root replacement, in-flight blocking, wrong identities, negative or
replacement observations and real mTLS wrong-peer denial. Provider tests bind the
original Core request digest to the independent Publisher over real local TLS,
without treating the synthetic upstream as live GitHub. API tests require a
separate reconciler, reject uploaded evidence and preserve unknown-COMMIT errors.

The required PostgreSQL suite covers five actual ledger-state paths (the legacy SAFE_TO_RETRY row is seeded
explicitly only in an isolated fixture), exact
repeats, immutable audit/key/epoch preservation, independent completion, proof
clearing, post-completion readback and insufficient-evidence rejection. Running
without PostgreSQL skips those tests and is NOT acceptance; the existing CI is
extended to require the database and zero skipped targeted tests. Exact-head CI,
protected merge and fresh-main delivered-byte readback are still required.


## Transaction and restore qualification additions

Targeted database tests also exercise a real TCP cut after PostgreSQL emits the
COMMIT CommandComplete, concurrent identical settlements, caller cancellation
while waiting on platform/operation/audit locks, and failure to append audit.
The client returns no success receipt on unknown COMMIT; direct authoritative
readback retains one receipt and one audit without calling the Provider again.

Core HTTP with real mTLS and isolated PostgreSQL checks capability denial,
identity conflict, independent aggregate-proof/completion roles, and historical
readback after the observer is unavailable. Its external witness fixture is
explicitly synthetic; independent Publisher generation is tested separately.

The restore gate uses genuine pg_dump/pg_restore into a separate allocated test
database, removes the original fixture schema's tables, then checks saved receipt,
proof, journal head, unique reservation and independent completion against only
the restored database. A separate private-filesystem test restores exact witness
bytes into another root and rejects replay and missing completion. These fixtures
do not establish production backup provenance, retention or disaster-recovery SLOs.

Canonical CI retains PostgreSQL 17 Docker tools and requires both existing Core
restore and publication restore tests. An explicitly selected native same-major
test-tool mode supports additional local validation; a PostgreSQL 18 run never
counts as PostgreSQL 17 or five-job canonical CI. Strict JSON-log gates reject
missing/failed/skipped top-level or nested tests, including a PASS parent with a
SKIP child. The DB gate names all nine required recovery tests, not just two
historical tests that would allow later coverage to disappear silently.


The transient systemd regression maps the canonical StateDirectory and writable
witness path to its own random ep-sysd namespace, retaining mode 0700 and the
separate read-only artifact path. Its configuration is v2, and teardown can
remove only that new test namespace after stopping its units. No production-named
state directory is created or removed by the fixture. Offline mapping contracts
are not a substitute for the required real systemd hosted-runner test.
