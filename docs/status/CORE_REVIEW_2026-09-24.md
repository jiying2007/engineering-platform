# Core review — 2026-09-24

Reviewed baseline: `c4c93c1880e9f7dd648b1e6e736e0612e96a384a`.
Its exact-main CI run `35993962820`, job `107614384742`, completed successfully.
PRs #26/#27/#28 are merged. No repeated PostgreSQL/workspace scaffolding is needed.
This review covers the Core model/API, transaction/outbox boundaries, workspace,
Codex transport/provider, executable assembly, CI and implementation status. It
is not a line-by-line certification of every archived research document.

## Architecture assessment

The clean-slate embedded-software scope and separation of engineering, external
action authority, evidence and verification remain appropriate. Core PostgreSQL
transactions already put business mutation, audit and outbox insertion in one
commit (`internal/store/postgres/mutation.go`). Further extension domains are not
needed. The main risk is mistaking individually tested components for an assembled,
authenticated and recoverable product.

## Findings and acceptance gates

| Priority | Finding at reviewed baseline | Required next proof |
| --- | --- | --- |
| P0 | `outbox_store.go` gates recovery only during claim; `dispatcher.go` invokes handlers without checking current recovery/lease authority. | Claim, enter recovery, then dispatch: mutation handler must not execute. Fence the actual side-effect boundary; a second unlocked read alone cannot make external effects transactional. Preserve UNKNOWN/reconciliation semantics. |
| P0 | MarkDispatched/Retry/DeadLetter match only ID/state/worker ID; no expiry or lease generation. | Same worker ID reclaims an expired lease: the old owner cannot ACK/retry/dead-letter the new attempt. Use an exact lease token/generation and DB-time expiry checks. |
| P0 | `insertOutbox` defaults an omitted risk class to OBSERVE. | Unclassified/unknown topics fail closed or have explicit trusted classification. Recovery tests must include missing classification. |
| P0 | `cmd/control-plane/main.go` constructs `api.NewServer`, leaving Action Gateway unbound; Worker/dispatcher/runtime orchestration is not assembled. | A command-level vertical test proves dispatch, authenticated policy, pause/steer/approval and durable receipts through real wiring. Keep absent privileged adapters disabled, never substitute permissive stubs. |
| P0 | HTTP handler returns its mux without authentication; recovery completion and evidence issuer facts are caller supplied. Original listener binds all interfaces. | This change defaults to loopback. Before remote use, bind authenticated principal/capabilities, restricted recovery authority and validated reconciliation/evidence issuers. Loopback is not authentication. |
| P0 before real Runtime | Codex provider inherits the full parent environment and arbitrary launch args; isolated HOME is only a lexical check. | Allowlisted environment/config/launch policy, credential separation, rootless sandbox, resource/network limits and a test that host secrets/config cannot reach the Runtime. |
| P0 before real Runtime | Workspace root/cleanup checks are lexical; parent symlink and inherited Git environment/hooks/config are not hardened. | Anchored path operations and exact ownership; adversarial symlink/cleanup/hook/config tests. A clean worktree is not a sandbox. |
| P1 | Codex JSONL client is transport only; no initialized thread/turn/approval product adapter. Buffered events can block forever; there is no explicit close/drain lifecycle. | Version-matched schema adapter, request correlation, cancellation, bounded backpressure, server approvals, short-write/EOF handling and controlled shutdown tests. |
| P1 | Dispatcher mutates defaults on a shared receiver, uses fixed retry delay, lacks per-message deadline/lease renewal. | Concurrent DispatchBatch under race detector; no handler can outlive authority, bounded retry policy and actual worker driving loop. |
| P1 | In-memory Store retains/returns slice-containing structs by shallow copy. | Mutation of submitted or returned contracts/Run inputs must not alter stored digest-addressed records; add deep-copy and conformance tests. PostgreSQL serialization does not prove memory semantics. |
| P1 | Implementation status incorrectly says PostgreSQL/outbox/workspace do not exist. | Replace stale component inventory and explicitly distinguish library/API coverage, executable wiring and real pilot evidence. Updated in this change. |

These are code-path findings, not claims that an exploitation or failure occurred
in a deployed environment. Negative tests for unresolved findings remain required.

## This change

- Structured ContextRef identity and strict JSON validation, not legacy strings.
- Separate raw-byte digest and explicit independent authorization/resolution ports.
- Bounded streaming, staged atomic publication, traversal-resistant root operations,
  deterministic manifest and restart Verify/revocation/tamper checks.
- Default HTTP bind to 127.0.0.1; `LISTEN_HOST` is an explicit operator override.
- CI runs `go test -race -timeout 5m ./...`, retaining PostgreSQL 17 integration,
  module hygiene, gofmt, vet and all three binary builds.
- Real code PR, not a new marker-only validation file.

Context filesystem modes are NOT protection against a hostile same-UID process.
Production resolver/approval registry, sandbox read-only mounting and Worker
consumption are not implemented by this change. See
`docs/implementation/CONTEXT_MATERIALIZATION_V1.md` for the exact contract.

## Next sequence — no new domains

1. Close outbox lease-generation/recovery/risk-classification gaps with real DB
   negative tests before wiring any irreversible handler.
2. Bind authenticated command-level Worker/Action/Context execution, with context
   approval and workspace/environment/sandbox boundaries tested as one path.
3. Add versioned Codex initialize/thread/turn/steering/approval lifecycle and bounded
   transport shutdown; retain a controlled offline integration test.
4. Connect actual Git/CI/artifact facts, then retain one Feature and one Debug pilot.

No module count, schema count, architectural research count or green component CI
is authority to claim M1 or production readiness.
