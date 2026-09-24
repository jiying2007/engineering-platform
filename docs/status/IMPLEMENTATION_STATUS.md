# Implementation Status

Date: 2026-09-24
Stage: **implementation-bootstrap**

## 1. Canonical scope

Current:
- `docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md`
- `docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md`
- `docs/roadmap/CORE_M0_M1_VERTICAL_SLICE_PLAN_V1.md`
- `docs/extensions/EXTENSION_CATALOG_V1.md`

Historical v1.2 / profile-v43 / M0-v43 documents remain research/design evidence only.

## 2. Implemented

### Repository / build
- Go module initialized.
- `cmd/control-plane` entrypoint.
- `cmd/eng` CLI entrypoint.
- `cmd/worker` entrypoint.
- GitHub Actions CI for gofmt/test/vet/build.
- developer Makefile.

### Core contracts
- WorkItem.
- TaskContract + digest.
- RunInputManifest.
- ArtifactRef.
- EvidenceRef.
- DeliveryReceipt.

### Embedded domain
- 6 initial capabilities.
- 8 initial Skills.
- explicit M1 task routing.
- fail-closed Material Readiness.

### Execution safety
- RunAttempt.
- execution_epoch.
- stale-epoch rejection.
- Human Takeover revokes old Runtime epoch.
- structured Session and Steering sequence.
- Checkpoint contract.

### External actions
- External Operation Ledger.
- UNKNOWN cannot be blindly retried.
- reconciliation is required before safe retry.

### Debug
- HypothesisRegistry.
- hypothesis confirmation requires Evidence.

### Verification
- acceptance criteria require concrete Evidence.
- stale/different-subject Evidence cannot PASS current Verification.
- non-PASS Evidence blocks Verification.

## 3. Locally validated

The current Go bootstrap was independently reconstructed from repository contents and passed:

~~~text
gofmt
go test ./...
go vet ./...
go build ./cmd/control-plane ./cmd/eng ./cmd/worker
~~~

This is implementation validation of the bootstrap only.

It is not M0 completion, production readiness, or a real pilot.

## 4. Not implemented yet

### Persistence / orchestration
- PostgreSQL repositories.
- Temporal workflows.
- outbox.
- durable audit journal/checkpoints.
- recovery_epoch.

### Runtime execution
- actual Session Supervisor process management.
- PTY/stream attach.
- Codex Runtime adapter.
- pause/resume at OS process level.
- workspace/worktree/sandbox manager.
- context attachment.

### Action Gateway
- OPA integration.
- Git provider adapter.
- CI adapter.
- device/HIL adapter.
- credential broker.
- action receipts persisted durably.

### Assurance
- persisted Verification plans/reports.
- independent Review workflow.
- Evidence issuer registry.
- Artifact store integration.

### WorkBuddy
- WorkBuddy connector/front-door integration.

### Real engineering evidence
- no real Feature pilot yet.
- no real Debug pilot yet.
- Skills are currently DEFINED, not PILOTED/PROVEN.

## 5. Immediate implementation queue

P0:
1. persistence interfaces + PostgreSQL schema;
2. Control API for WorkItem/TaskContract/Run;
3. durable Run/Attempt state and optimistic concurrency;
4. Session Supervisor skeleton;
5. Codex local Runtime adapter;
6. structured event/audit journal;
7. Action Gateway contract + fake provider;
8. Verification persistence.

P1:
9. Temporal workflow around Run lifecycle;
10. Git/CI provider adapter;
11. Artifact store;
12. CLI commands for run/steer/checkpoint/takeover/evidence/verify;
13. first Feature pilot.

P2:
14. Debug pilot with HypothesisRegistry;
15. Worker sandbox/worktree;
16. recovery/reconciliation drill.

## 6. Readiness statement

Current repository claimable maturity:

> **Core architecture: frozen enough to implement.**
>
> **Bootstrap code: compiling and unit-tested.**
>
> **M0: in progress.**
>
> **M1: not yet achieved.**
>
> **Production readiness: not claimed.**

No document count, research round count or green bootstrap CI may be used to claim otherwise.
