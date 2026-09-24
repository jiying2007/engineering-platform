# Implementation Status

Date: 2026-09-24
Stage: **Core contracts/persistence + authenticated API + prepared Runtime protocol; no real M1 pilot**

## Scope and assembly

Core architecture, Embedded Domain Capability Model, Core M0/M1 plan and
ADR-001/002/003 remain canonical. This platform is independent of digital-worker.
No extension domain or service is introduced by the Runtime/Context work.

| Area | Implemented | Remaining acceptance boundary |
| --- | --- | --- |
| Core | Work/Task/frozen VerificationPlan and RunInput, epoch/CAS, Session/Steering/Checkpoint, Delivery/Evidence/Verification/Closure | Memory deep-copy/conformance, independent real evidence and Review |
| PostgreSQL | Core/action persistence, business + audit + outbox transactions, recovery audit and lifecycle tests | Restore/migration drills and actual product assembly |
| Outbox v2 | Live generation/expiry fencing, recovery-serialized dispatch, trusted risk registry, bounded cooperative retry, UNKNOWN quarantine | Actual durable recipient/consumer; no distributed exactly-once |
| Authenticated HTTP | Real command requires mTLS, explicit certificate URI principal and platform capabilities; bound actors/actions/issuers; strict JSON | Single trust domain, no Work-level ACL or automated certificate lifecycle |
| Recovery completion | Protected API requires independent exact-epoch reconciliation gate | Concrete verifier absent; completion remains unavailable in command |
| Action Gateway | Durable ledger and contract/epoch/recovery components with HTTP grants | Privileged provider/effect boundary is not injected into command |
| Workspace | Exact-base detached worktree, fresh HOME and cleanup | Git environment/config/hooks, shared metadata and parent-path hardening; OS isolation |
| Codex Runtime (#32) | Constructed allowlisted environment, fixed stdio launch, closable bounded correlated JSONL, typed initialize/thread/turn/steer/interrupt, scoped deny-only approvals, EOF/early-completion regression | Real version-pinned Codex qualification, Core authority binding, supervised execution and approval-grant integration |
| Context (#32) | Structured refs/materializer/Verify plus concrete content-addressed LocalSource and exact frozen-input operator approval snapshot | Authenticated approval lifecycle, Worker mounting/consumption and durable execution receipt |
| Worker admission candidate | Prior local ZIP contains proposed inbox/relay/worker/client/CLI code | NOT merged or covered by Runtime PR CI. Do not silently apply the old-base package to new main |
| Embedded Skills | Six capabilities/eight defined Skills and explicit material/routing rules | No retained real Feature/Debug pilot; no PILOTED/PROVEN claim |

## Evidence

- #29 Context: PR `35998634850`; main `35998821289`.
- #30 outbox: PR `36003408959`; main `36003703669`.
- #31 authentication: PR `36007219869`; main `36007507189`.
- #32 Runtime/Context: authoritative evidence is its exact final PR check and
  subsequent fresh-main check, not the preceding results or this document.

The new cross-component test uses real local Git, an exact-base worktree,
operator-approved source bytes, materialization and Verify, a newly launched
isolated subprocess, and thread/turn/steer/interrupt/close with source clean-state
checks and cleanup. The subprocess is an offline test helper, NOT real Codex,
model output, provider evidence or proof of actual model Context consumption.
CI retains full Go race tests with PostgreSQL 17, outbox repetitions, vet/build
and adds ten Runtime/Context repetitions. Scoped local Go 1.23 tests are partial;
full os.Root/cross-component proof requires the repository's Go baseline CI.

## Next delivery gates

1. Resolve and verify the separate Worker admission change through an authorized
   code review/CI path; bind the real recipient to Run/lease/recovery authority.
2. Harden Workspace Git/OS isolation and bind approved Context preparation to an
   execution receipt. Do not substitute a read-only protocol request for an OS
   sandbox or a boolean INPUT_VALIDATED for execution authorization.
3. Qualify a pinned real Codex binary/schema; connect interactive approvals to
   Action Gateway and retain exact Git/CI/Artifact facts plus independent Review.
4. Retain Feature/Debug pilots and recovery/restore proof, then assess M1.

Operator local approvals are immutable restart snapshots, not an online registry.
No production credentials, model calls, migrations or deployment occur merely
by merging source. **M1 and production readiness remain unclaimed.**

Contracts and explicit limits:
- `docs/implementation/PREPARED_RUNTIME_PROTOCOL_V1.md`
- `docs/implementation/AUTHENTICATED_CONTROL_API_V1.md`
- `docs/implementation/OUTBOX_DISPATCH_AUTHORITY_V2.md`
- `docs/implementation/CONTEXT_MATERIALIZATION_V1.md`
