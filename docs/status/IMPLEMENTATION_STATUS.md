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
| Outbox | PostgreSQL SKIP LOCKED claim, lease/reclaim, retry/dead-letter; recovery filter at claim; dispatcher library | Lease generation/expiry fencing, actual-dispatch recovery guard, safe risk defaults, bounded handler lifecycle and driving loop |
| Action Gateway | Policy/epoch/recovery guard components, idempotency/external operation ledger, UNKNOWN/reconciliation and API tests | Gateway is NOT injected by the current control-plane executable; real privileged adapters/authentication remain absent |
| Workspace | Exact full-SHA detached worktree, new-worktree clean check, separate HOME and cleanup tests | Parent-symlink/ownership and Git environment/config/hook hardening; OS sandbox |
| Runtime | Provider-neutral process supervisor; Codex app-server command builder and JSONL transport | Initialized thread/turn/steering/approval lifecycle, bounded transport shutdown, environment policy and real Worker integration |
| Context (this change) | Structured refs, raw-byte hash, independent authorization port, bounded resolver reads, staged read-only bundle, deterministic manifest, Verify/revocation/tamper checks | Concrete trusted Resolver/Authorizer, Worker consumption, sandbox read-only mount and durable receipt |
| HTTP (this change) | Bootstrap API defaults to 127.0.0.1; explicit LISTEN_HOST/PORT overrides | Authenticated principal/capabilities; trusted recovery-completion/evidence issuers; remote API must not be exposed as production-ready |

## Verification evidence

The reviewed baseline is `c4c93c1880e9f7dd648b1e6e736e0612e96a384a`.
Its main CI run `35993962820` / job `107614384742` passed module hygiene, gofmt,
all tests with PostgreSQL 17, vet and binary builds. PRs #26, #27 and #28 were
merged component-validation checkpoints.

This change upgrades CI to `go test -race -timeout 5m ./...`, while retaining the
Go version from go.mod, PostgreSQL 17, module hygiene, gofmt, vet and builds.
Its authoritative result is the exact PR head check, followed by fresh main CI
following merge; the baseline result is NOT substituted for new-change evidence.
No marker-only PR is required to query push/main CI.

## Immediate queue

P0: outbox lease/recovery/risk semantics and negative DB tests; trusted HTTP/action
and Worker assembly; approved Context consumption; workspace/env/sandbox safety.
Do not drive irreversible handlers before these gates are closed.

P1: bounded Codex protocol lifecycle; immutable memory-store conformance; concrete
Git/CI/artifact adapters; retained Feature pilot and Debug pilot; restore/drill
proof. Temporal/OPA integration should serve that bounded path, not introduce
another independent authority or expand the domain catalogue.

See `CORE_REVIEW_2026-09-24.md` for exact findings and acceptance tests, and
`docs/implementation/CONTEXT_MATERIALIZATION_V1.md` for Context semantics.

## Readiness

Core PostgreSQL persistence and tested execution building blocks are real, not
just tables/dependencies. However library/contract CI is not command-level product
execution. There is no retained real Feature pilot or Debug pilot. WorkBuddy
front-door integration, authenticated policy/credentials, independent Review,
trusted evidence issuers and Artifact-byte authority remain incomplete.

**M1 and production readiness are not claimed.** Device/HIL and other extension
domains remain deferred until the two Core pilots have retained exact evidence.
