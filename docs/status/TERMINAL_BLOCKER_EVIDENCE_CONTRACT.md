# Engineering Platform — terminal blocker evidence contract

**Scope:** Internal blockers [#228](https://github.com/jiying2007/engineering-platform/issues/228) and [#229](https://github.com/jiying2007/engineering-platform/issues/229). This is an acceptance contract, **not** a claim that either issue is resolved. Baseline: protected main `46e9a2b3512bc7dca059166b22a65019f130ccca` (PR #284 merged). The platform remains `BLOCKED [228,229]`; production #105 and credential alternatives #103/#106 are separate.

## Authority and evidence semantics

A successful CI run proves only the exact source and test environment covered by that run. It does **not** prove that a previously issued Provider effect has ceased, that a remote publication happened, that an absent remote ref cannot appear later, that a database commit was rolled back, or that a selected host meets a service-level objective.

No positive remote observation alone may convert an effectful Action to `CONFIRMED`. No negative observation alone may permit replay. A stop flag is not a dispatcher-quiescence certificate. A `MANUAL`, `UNKNOWN`, `DISPATCHED`, `RECONCILING`, historically bare `CONFIRMED`, or legacy unqualified `SAFE_TO_RETRY` record cannot become healthy through operator assertion, bulk SQL repair, an audit event count or a timeout. Preserve original idempotency keys, request digests, epochs and audits.

## #228 — exact, independent reconciliation

| Case | Acceptable result | Mandatory evidence |
|---|---|---|
| Proven pre-dispatch `PLANNED` | Recovery-only non-success abandonment, never `CONFIRMED` | Immutable reservation ordering, no original dispatch, exact run/epoch/key and audit/readback |
| `DISPATCHED` without original completion | Remains blocked | Original dispatcher identity and process-tree state, surviving operation/plan binding; **no automatic replay** |
| `UNKNOWN`, `RECONCILING`, `MANUAL` | Remains blocked absent independent outcome disposition | Independently retained provider-specific readback, frozen publication subject and target, principal separation, full audit |
| Original successful dispatch, persisted confirmation | May be `CONFIRMED` only through the trusted original settlement path | Exact current persisted Operation digest plus unique causal audit, consistent timestamp and immutable receipt |
| Historical bare `CONFIRMED` | Readiness blocked until independently authorized full-evidence Recovery enrichment | Original completion/reap evidence; exact identity, target and independent current observation; no event-count inference |
| Positive recovery receipt missing but surviving recovery audit exists | Corrupt; block | Forensic preservation, no overwriting old receipt or pretending first enrichment |
| External absent, partial, retargeted or transiently unreadable | Remains blocked | One absence probe / closed PR / vanished branch alone is insufficient proof of no delayed remote effect |

A qualified terminal disposition must bind operation ID, idempotency key and request digest; Run and execution/recovery epochs; frozen plan, repository, target branch/ref and expected subject digest; the **original** publisher process identity and independently established quiescence; observation time/window, remote status and exact immutable evidence hash; distinct `recovery:reconcile` and `recovery:complete` authorities; append-only audit linkage and a post-restore readback. An exact duplicate request returns the original record without new observation, audit or external mutation. Conflicting identity, principal, target, epoch, readback or receipt fails closed.

**Fault matrix (not waiveable):** crashes before dispatch, after effect before acknowledgment, after observation before local commit, and while recovery is reconciling; live/delayed publisher, stale process identifier, missing bundle, changed target, deleted branch with closed PR, delayed materialization, database COMMIT acknowledgment loss, concurrent Recovery Begin/Takeover and a failed mTLS authorization. Preserve retained failure evidence as failures. Execute on the selected Provider/host before claiming live qualification.

## #229 — selected host qualification

Before the selected-host result can be `QUALIFIED`, retain:

1. **Identity:** exact protected main SHA/tree, binary manifest and actual binary hashes, toolchain/OS/kernel, PostgreSQL major, configuration digests, service identities and active provider; never reuse another host's receipt.
2. **Lifecycle:** cold/warm start, graceful termination, forced kill, restart and upgrade/rollback; no orphan workers, late external dispatch or unbounded detached authority work.
3. **Storage:** disk-full, quota, permission, WAL/backup restore, crash/reboot and durable readback under the host's real fsync/storage policy. A test DB with fsync disabled does not prove power-loss durability.
4. **Latency/capacity:** measured p50/p95/p99 and worst-case queue, connection-acquisition, lock, statement, COMMIT and complete request durations; concurrency, workload, duration, CPU/RAM, peak disk, throughput and error denominators. CI characterization is not a production SLO.
5. **Cancellation/ambiguous outcomes:** caller deadline, row-lock contention, actual transport interruption during COMMIT and complete readback of the exact resource/epoch/key/audit; never infer rollback, refresh deadlines or replay model/publication.
6. **Long soak:** declared host/workload/duration, steady-state memory/fd/goroutine/thread/connection counts, recovery after faults, retention/cleanup and trend thresholds. Stop/abort/failure logs must be retained.

Acceptance criteria and numeric budgets must be specified **before** running tests by the responsible host owner; no fabricated default production SLO. A bound caller context and independently bounded authorized maintenance lifetime must cover the whole database operation including pool acquisition and transaction rollback/settlement. `context.WithoutCancel` must not become an unbounded authority path.

## Machine-readable receipt expectations

For each acceptance case, retain one immutable record with fields:

```yaml
schema: engineering-platform-terminal-acceptance-v1
repo: jiying2007/engineering-platform
main_sha: <40-hex-from-live-protected-main>
main_tree: <40-hex>
host_identity_digest: sha256:<64-hex>
provider_identity_digest: sha256:<64-hex-or-explicit-not-applicable>
case_id: <stable-case-id>
operation_id: <exact-operation-id-or-explicit-not-applicable>
run_id: <exact-run-id-or-explicit-not-applicable>
execution_epoch: <integer-or-explicit-not-applicable>
recovery_epoch: <integer-or-explicit-not-applicable>
subject_digest: sha256:<64-hex-or-explicit-not-applicable>
original_dispatcher_quiescence_evidence: <immutable-reference-or-explicit-not-applicable>
external_readback_evidence: <immutable-reference-or-explicit-not-applicable>
raw_logs_sha256: sha256:<64-hex>
result: PASS|FAIL|BLOCKED
performed_by: <principal>
independent_verified_by: <different-principal>
time_utc: <RFC3339>
```

`PASS` is permitted only when all case-specific required evidence is independently verified, complete and internally consistent. An unavailable, partial, self-signed or contradictory artifact must be `BLOCKED` or `FAIL`. This proposed receipt shape is documentation, not a deployed parser, schema migration or a new authority ledger.

## Closure procedure

1. Freeze a candidate and its exact source/binary identities. Inventory historical bare `CONFIRMED`, unsafe retry and unresolved effectful rows **before** deployment. Do not silently migrate or auto-fix them.
2. Run existing repository tests and five canonical required jobs on the exact PR HEAD. Require protected merge, fresh-main five gates, actual artifact/binary/status bytes and own-branch retirement. This proves repository-side delivery only.
3. On the selected host/provider, execute the #228 fault matrix with retained independent evidence; unresolved effects remain blocked. Obtain separate reconcile/complete principals and independent review where applicable.
4. Execute #229 lifecycle, storage, transport-cut, resource, latency and soak acceptance using predeclared thresholds; collect negative/failure controls.
5. Independently review receipts, audit chains, readback, identity binding, failures and scope. Only then consider issue-state transitions; keep #105, #103 and #106 separate and retain original historical evidence.

**Exit state:** Until every applicable #228 and #229 case has evidence and independent verification, report `BLOCKED [228,229]`. A documentary checklist, fresh-main green run, mock provider, historical local PostgreSQL18.6 test or synthetic runner must not change that state.
