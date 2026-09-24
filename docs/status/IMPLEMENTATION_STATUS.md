# Implementation Status

Date: 2026-09-24
Stage: **M0 Core contracts/persistence; authenticated API assembly, no real M1 pilot**

## Canonical scope

Core architecture, Embedded Domain Capability Model, Core M0/M1 vertical slice
plan, Extension Catalog and ADR-001/002/003 remain canonical. This platform is
independent of digital-worker. Historical architecture profiles remain research
references, not an expanding implementation checklist.

## Implemented and bounded capabilities

| Area | Implemented | Remaining acceptance boundary |
| --- | --- | --- |
| Contracts | Work/Task/VerificationPlan, frozen RunInput, epoch/CAS, Session/Steering/Checkpoint, Delivery/Evidence/Verification/Closure | Memory deep-copy/conformance; real independent evidence and Review |
| Embedded domain | Six capabilities/eight defined Skills, explicit routing and Material Readiness | Retained Feature/Debug pilots; no PILOTED/PROVEN claim |
| PostgreSQL | Core/action persistence, business + audit + outbox transactions, recovery audit, exact lifecycle regression | Operational restore/migration drills and complete product assembly |
| Outbox v2 | Exact live generation/expiry fencing, recovery-serialized dispatch, risk registry, bounded cooperative delivery/backoff, UNKNOWN quarantine and tracked migration | Durable recipient/consumer and driving loop; no distributed exactly-once |
| Authenticated HTTP (this change) | Real command requires mTLS, certificate URI principal and explicit platform-scoped capability policy; actor/action/issuer/procedure/verifier checks; strict bounded wire JSON; startup/shutdown bounds | Single trust domain only; no row ACL, CA enrollment/automated revocation, hot reload or per-request atomic principal audit |
| Recovery completion (this change) | Protected API requires independent exact-epoch reconciliation gate beyond a caller boolean; missing gate is unavailable | Concrete trusted reconciliation receipt/verifier still required; completion disabled in command |
| Action Gateway | Contract/epoch/recovery components and durable ledger; authenticated HTTP checks exact principal action grants | Privileged provider is NOT injected in the command; actual effect boundary/reconciliation remains necessary |
| Workspace | Exact-base detached worktree, isolated HOME and cleanup tests | Parent-symlink/ownership, Git config/hooks/environment hardening, sandbox |
| Codex | Command builder and JSONL transport, generic supervisor | Bounded lifecycle and initialize/thread/turn/steer/approval; environment policy and actual Worker integration |
| Context | Structured refs, raw-byte hash, authorization/resolver ports, bounded materialization and restart/tamper verification | Concrete approval/resolver, Worker consumer, read-only sandbox mount and durable receipt |

Default startup is now fail-closed: mTLS/policy/DATABASE_URL must be complete.
`INSECURE_DEV=1` is explicitly memory-only and literal-loopback-only; it rejects
configured DB/migrations/TLS inputs rather than silently ignoring them.

## Retained verification checkpoints

- Context #29: main `3e1663a53064b2292ca36c415afa1469c381b3fb`;
  PR CI `35998634850`, fresh main CI `35998821289` passed.
- Outbox #30: main `3e3c320b7d3e6ceb09bbb4150035dc923b8781ea`;
  PR CI `36003408959`, fresh main CI `36003703669` passed. Includes full PostgreSQL
  race suite, 20 outbox repetitions and 3 shuffled database-authority repetitions.
- Authenticated API: exact new PR head CI and subsequent fresh-main CI are the
  authority for this change. Previous green checks are not substituted for it.
  New tests include real mTLS command assembly and in-memory/PostgreSQL HTTP
  contract lifecycles, using synthetic issuer results, not provider execution.

`CORE_REVIEW_2026-09-24.md` remains the historical review. The three outbox findings
are handled within the documented v2 contract. Unauthenticated command exposure
and self-asserted HTTP actors now have a guarded assembly path. Production
reconciliation authority, provider wiring and execution isolation are not implied.

## Next bounded implementation queue

P0: durable Worker inbox/recipient and actual driving loop with deduplication and
Run/epoch authority; approved Context consumption; Workspace/environment/sandbox
hardening before real Runtime; independently retained reconciliation authority.
Do not run raw irreversible effects inside outbox callbacks or authorize them
based only on the new HTTP capability gate.

P1: Codex bounded transport/interactive lifecycle; immutable memory Store
conformance; mTLS-capable CLI/front door and identity lifecycle; transactional
principal audit; actual Git/CI/Artifact-byte adapters and independent Review;
retained Feature/Debug pilots and recovery/restore drill. Temporal/OPA should
serve that bounded path instead of creating duplicate authorities.

See `docs/implementation/AUTHENTICATED_CONTROL_API_V1.md`,
`docs/implementation/OUTBOX_DISPATCH_AUTHORITY_V2.md` and
`docs/implementation/CONTEXT_MATERIALIZATION_V1.md` for exact component contracts
and limitations.

**M1 and production readiness are not claimed.** Device/HIL and new extension
domains remain deferred; no source merge deploys a service or provisions credentials.
