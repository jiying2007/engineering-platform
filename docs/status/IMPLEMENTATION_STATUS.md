# Implementation Status

Date: 2026-09-24
Stage: **M0 Core implemented in parts; M1 execution components, not an assembled pilot**

## Canonical scope

- `docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md`
- `docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md`
- `docs/roadmap/CORE_M0_M1_VERTICAL_SLICE_PLAN_V1.md`
- `docs/extensions/EXTENSION_CATALOG_V1.md`
- `docs/adr/ADR-001-clean-slate-embedded-platform-scope.md`
- `docs/adr/ADR-002-postgres-transaction-audit-outbox.md`
- `docs/adr/ADR-003-go-postgres-baseline.md`

This platform is independent of digital-worker. Historical architecture profiles
are research evidence, not an expanding Core implementation checklist.

## Implemented components

| Area | Implemented | Boundary still to close |
| --- | --- | --- |
| Contracts | Work, immutable Task revisions and frozen VerificationPlan; RunInput digest; epoch/CAS; Session/Steering/Checkpoint; Delivery/Evidence/Verification/Closure | In-memory alias protection/conformance and authenticated authority |
| Embedded domain | Six capabilities, eight defined Skills, explicit routing, fail-closed Material Readiness, evidence-backed debugging contract | Retained Feature/Debug pilot evidence; Skills are not yet PILOTED/PROVEN |
| PostgreSQL | Core Store, lifecycle integration, state + audit + outbox transaction, action repository/audit, recovery transition audit, DATABASE_URL backend switch | Operational migration/recovery drills and whole-product runtime wiring |
| Outbox v2 (this change) | Generation + DB-expiry fencing for all receipt writes; epoch-bound SKIP LOCKED claims; locked dispatch/recovery serialization and atomic settlement/audit; explicit risk registry; cooperative deadlines; bounded backoff; UNKNOWN quarantine; tracked legacy migration | Actual driving loop and trusted idempotent recipient integration; no distributed exactly-once or forced cancellation claim |
| Action Gateway | Policy/epoch/recovery guard components, idempotency/external operation ledger, UNKNOWN/reconciliation and API tests | Gateway is NOT injected by the current control-plane executable; real privileged adapters/authentication remain absent |
| Workspace | Exact full-SHA detached worktree, new-worktree clean check, separate HOME and cleanup tests | Parent-symlink/ownership and Git environment/config/hook hardening; OS sandbox |
| Runtime | Provider-neutral process supervisor; Codex app-server command builder and JSONL transport | Initialized thread/turn/steering/approval lifecycle, bounded transport shutdown, environment policy and real Worker integration |
| Context | Structured refs, raw-byte hash, independent authorization port, bounded resolver reads, staged read-only bundle, deterministic manifest, Verify/revocation/tamper checks | Concrete trusted Resolver/Authorizer, Worker consumption, sandbox read-only mount and durable receipt |
| HTTP | Bootstrap API defaults to 127.0.0.1; explicit LISTEN_HOST/PORT overrides | Authenticated principal/capabilities; trusted recovery-completion/evidence issuers; remote API must not be exposed as production-ready |

## Verification evidence

Context PR #29 merged as `3e1663a53064b2292ca36c415afa1469c381b3fb`.
PR CI `35998634850` and fresh main CI `35998821289` passed module hygiene,
gofmt, full race tests with PostgreSQL 17, vet and all three binary builds.

Outbox v2 preserves those checks and adds 20 repeated unit-suite runs and 3
repeated focused PostgreSQL authority-suite runs. Its proof must be the exact
new PR head and fresh main CI; the prior Context result cannot prove this change.
No marker-only PR is needed to observe push/main CI.

## Review finding disposition

`CORE_REVIEW_2026-09-24.md` remains the historical baseline review. Its first
three outbox P0 findings now have an implementation and adversarial DB tests:
exact live-lease receipt fencing, dispatch-time recovery serialization/epoch, and
fail-closed risk classification. Receiver-default mutation and fixed-delay retry
are also replaced. See `docs/implementation/OUTBOX_DISPATCH_AUTHORITY_V2.md` for
acceptance conditions, explicit callback assumptions and crash/DB-outage limits.
Do not extrapolate cooperative dispatch proof to arbitrary external side effects.

## Immediate queue

P0: authenticated HTTP/action and command-level Worker assembly; independent
recovery-completion/evidence authority; approved Context consumption; workspace,
environment and sandbox safety. Only bounded idempotent recipients may be wired
to outbox; irreversible operations still require the reconciled Action ledger.

P1: bounded Codex protocol lifecycle; immutable memory-store conformance; concrete
Git/CI/artifact adapters; retained Feature pilot and Debug pilot; restore/drill
proof. Temporal/OPA integration should serve that bounded path, not introduce
another independent authority or expand the domain catalogue.

See `CORE_REVIEW_2026-09-24.md` for the retained review and
`docs/implementation/CONTEXT_MATERIALIZATION_V1.md` for Context semantics.

## Readiness

Core PostgreSQL persistence and tested execution building blocks are real, not
just tables/dependencies. Library/contract CI is not command-level product
execution. There is no retained real Feature pilot or Debug pilot. WorkBuddy
front-door integration, authenticated policy/credentials, independent Review,
trusted evidence issuers and Artifact-byte authority remain incomplete.

**M1 and production readiness are not claimed.** Device/HIL and other extension
domains remain deferred until the two Core pilots have retained exact evidence.
