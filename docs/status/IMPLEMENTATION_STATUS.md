# Implementation Status

Stage: **Core persistence + authenticated API + prepared Runtime + durable Worker input admission; no real M1 pilot**
Reviewed base: `4c1a226b696221e5a3fa82b6a7e425fc301b21bb`.

## Scope and assembly

Core architecture, Embedded Domain Capability Model, Core M0/M1 plan and
ADR-001/002/003 remain canonical. This platform is independent of digital-worker.
No extension domain or service is introduced by the Worker/Runtime/Context work.

| Area | Implemented | Remaining acceptance boundary |
| --- | --- | --- |
| Core | Work/Task/frozen VerificationPlan and RunInput, epoch/CAS, Session/Steering/Checkpoint, Delivery/Evidence/Verification/Closure | Memory deep-copy/conformance, independent real evidence and Review |
| PostgreSQL | Core/action persistence, business + audit + outbox, recovery audit; additive inbox migration | Restore/migration operational drills and complete execution assembly |
| Outbox v2 | Live generation/expiry fencing, recovery-serialized dispatch, risk registry, bounded retry, UNKNOWN quarantine | External recipients remain responsible for idempotency/reconciliation |
| Authenticated HTTP | mTLS certificate URI principals, explicit platform capabilities, bound actors/actions/issuers, strict JSON | Single trust domain; no Work-level ACL or automated certificate lifecycle |
| Worker admission (this change) | Actual Control Plane loop atomically transfers run.started to durable inbox; profile/identity-bound claim, bounded renewal, generation/epoch fencing, pause deferral and retained INPUT_VALIDATED receipts | No code/provider execution or automatic advancement to completed Run/Evidence |
| Worker/CLI clients (this change) | Actual admission-only Worker and eng api with shared direct mTLS client; bounded I/O and no proxy/redirect/insecure fallback | Operator-issued credentials, lifecycle/restart policy and WorkBuddy connector |
| Recovery completion | Protected API requires independent exact-epoch reconciliation gate | Concrete verifier absent; completion remains unavailable in command |
| Action Gateway | Durable ledger and contract/epoch/recovery components with HTTP grants | Privileged provider/effect boundary is not injected into command |
| Workspace | Exact-base detached worktree, fresh HOME and cleanup | Git environment/config/hooks, shared metadata and parent-path hardening; OS isolation |
| Codex Runtime (#32) | Allowlisted environment, fixed stdio launch, bounded correlated JSONL, typed initialize/thread/turn/steer/interrupt, deny-only approvals | Qualified real binary/schema, Core execution authority and approval grants |
| Context (#32) | Structured refs/materializer/Verify, content-addressed LocalSource and exact frozen-input operator approval snapshot | Authenticated approval lifecycle, Worker execution consumption and durable prepared receipt |
| Embedded Skills | Six capabilities/eight defined Skills and material/routing rules | No retained Feature/Debug pilot; no PILOTED/PROVEN claim |

The old #31-base ZIP is superseded by the reviewed new-base Worker code in this
change. Do not apply that stale patch on top of current main. Input validation
always records context_bytes_verified=false and execution_started=false.

## Evidence

- #29 Context: PR `35998634850`; main `35998821289`.
- #30 outbox: PR `36003408959`; main `36003703669`.
- #31 authentication: PR `36007219869`; main `36007507189`.
- #32 prepared Runtime/Context: final PR head
  `c995062b477c45c8e0da311a179c0fb61efad431`, PR CI `36023841471`.
- This Worker change requires its exact final PR-head CI and fresh main CI.
  Older green component checks and partial local tests do not prove it.

CI preserves full Go 1.25 PostgreSQL 17 race tests, outbox repetitions, ten prepared
Runtime/Context repetitions, vet/build. It adds 20 Worker/client/loop/unit
repetitions, three shuffled Worker DB repetitions, and three command repetitions.
Command tests use the actual shared HTTP/relay lifecycle and compile/run real
Worker and eng binaries over mTLS against an isolated PostgreSQL schema. They
verify durable input receipts, not a model/provider task. The #32 subprocess
protocol test is still an offline helper, not installed Codex qualification.

The Worker suite additionally exercises pause-after-selection row-lock races,
expiry after audit-lock waits, same-identity concurrent claims, transfer/audit
rollback, stale generation/epoch, same-receipt replay and repeat migration.
No temporary credentials or destructive shared-public-schema reset are retained.

## Next delivery gates

1. Bind approved Context preparation and exact workspace to durable execution
   receipts with OS isolation, current Run/lease authority and trusted capability
   profiles. A validated input is not authorization to start an unsandboxed agent.
2. Qualify a pinned real Codex binary/schema; connect interactive approvals through
   Action Gateway and retain exact Git/CI/Artifact facts plus independent Review.
3. Implement retained reconciliation authority, memory-store conformance and
   operational restore/recovery drills without creating duplicate authorities.
4. Retain Feature/Debug pilots, then assess M1. No new extension domains yet.

Operator local approvals remain immutable restart snapshots, not an online
registry. Source merge does not deploy, provision credentials or run a production
migration. **M1 and production readiness remain unclaimed.**

Contracts:
- `docs/implementation/WORKER_ADMISSION_V1.md`
- `docs/implementation/PREPARED_RUNTIME_PROTOCOL_V1.md`
- `docs/implementation/AUTHENTICATED_CONTROL_API_V1.md`
- `docs/implementation/OUTBOX_DISPATCH_AUTHORITY_V2.md`
- `docs/implementation/CONTEXT_MATERIALIZATION_V1.md`
