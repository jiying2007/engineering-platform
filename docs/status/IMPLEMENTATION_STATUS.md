# Implementation Status

Date: 2026-09-24
Stage: **M0 core implementation**

## 1. Canonical scope

Current:
- `docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md`
- `docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md`
- `docs/roadmap/CORE_M0_M1_VERTICAL_SLICE_PLAN_V1.md`
- `docs/extensions/EXTENSION_CATALOG_V1.md`
- `docs/adr/ADR-001-clean-slate-embedded-platform-scope.md`
- `docs/adr/ADR-002-postgres-transaction-audit-outbox.md`

Historical v1.2 / profile-v43 / M0-v43 documents remain research/design evidence only.

## 2. Implemented and CI-verified

### Repository / build
- Go module.
- `cmd/control-plane`.
- `cmd/eng`.
- `cmd/worker`.
- Makefile.
- GitHub Actions:
  - gofmt;
  - `go test ./...`;
  - `go vet ./...`;
  - build all three binaries.
- current Actions use `actions/checkout@v7` and `actions/setup-go@v7`.

### Clean-slate embedded domain
- 6 initial embedded capabilities.
- 8 initial engineering Skills.
- explicit M1 task routing.
- fail-closed Material Readiness.
- DEGRADED material path requires explicit approver + reason.
- exact source/device identity remains a non-degradable hard gate.

### Task / verification contract
- immutable monotonic TaskContract revisions.
- historical revisions remain addressable by digest.
- VerificationPlan is frozen before execution.
- TaskContract binds exact VerificationPlan digest.
- frozen plan expresses evidence requirements by procedure/issuer rather than future Evidence IDs.
- verification criteria must cover the Task acceptance criteria.

### Run identity / interactive execution
- RunAttempt.
- `execution_epoch`.
- stale-epoch rejection.
- Human Takeover revokes prior Runtime epoch.
- structured Steering sequence.
- Checkpoint contract.
- optimistic Run aggregate version / CAS semantics.
- immutable RunInputManifest.
- Run binds exact RunInputManifest digest.
- Runtime / Tool / Worker / Policy profiles are frozen before Run start.

### Session Supervisor
- local provider-neutral process supervisor.
- stdout/stderr capture.
- stdin input.
- epoch guard.
- Linux SIGSTOP/SIGCONT pause/resume.
- abort/wait/PID.
- tested on GitHub Ubuntu runner.

### Action Gateway
- ActionRequest / Receipt.
- capability authorization boundary.
- epoch guard.
- External Operation Ledger.
- `DISPATCHED -> UNKNOWN -> RECONCILING`.
- no blind retry after ambiguous provider outcome.
- idempotency key + immutable request digest.
- duplicate identical request does not dispatch twice.
- same idempotency key with different request fails closed.

### Delivery / Evidence / Verification / Closure
- Run completion is explicit and terminal.
- DeliveryReceipt requires a completed Run.
- Delivery subject digest is calculated by the platform from:
  - TaskContract digest;
  - Run;
  - Target;
  - base/result commit;
  - sorted Artifact identities/digests.
- Evidence registration binds to an existing DeliveryReceipt; caller cannot self-declare subject.
- Verification consumes only registered Evidence.
- frozen VerificationPlan is loaded from the TaskContract; caller cannot replace the plan after implementation.
- stale/different-subject Evidence cannot pass.
- wrong procedure/issuer cannot satisfy a frozen EvidenceRequirement.
- FAIL Verification cannot close Work.
- ClosureReceipt requires PASS Verification for the exact Delivery subject.
- Work lifecycle is fail-closed through DRAFT -> READY -> EXECUTING -> VERIFYING -> CLOSED/REVIEWING.

### Debug discipline
- HypothesisRegistry.
- Observed/Inferred separation in contract.
- root-cause confirmation requires Evidence.

### Audit / recovery
- tamper-evident audit hash chain.
- audit integrity verification.
- audit checkpoint.
- recovery_epoch.
- RECOVERY_RECONCILIATION mode.
- irreversible actions blocked until reconciliation completes.

### Database contract
- focused PostgreSQL schema exists for Core only.
- Work / immutable Task + frozen VerificationPlan.
- immutable RunInputManifest.
- Run/Attempt/Session/Steering/Checkpoint.
- Action ledger.
- Artifact/Delivery/Evidence/Verification/Closure.
- Audit / Outbox / Worker.
- no Extension Catalog tables.

## 3. Real GitHub CI evidence

Validated with fresh PRs cut from current main during implementation.

Successful examples:

- PR #2 / run `35949487711`
  - gofmt PASS
  - test PASS
  - vet PASS
  - binary builds PASS

- PR #3 / run `35949673286`
  - Session Supervisor tests PASS on Ubuntu
  - gofmt/test/vet/build all PASS

- PR #6 / run `35950353698`
  - frozen VerificationPlan / EvidenceRequirement model PASS
  - gofmt/test/vet/build all PASS

- PR #8 / run `35950752163`
  - frozen RunInputManifest model PASS
  - gofmt/test/vet/build all PASS

Marker-only validation PRs are used because the current connector exposes pull-request workflow runs reliably.

## 4. Implemented bootstrap API

Current in-memory bootstrap API covers:

~~~text
WorkItem
 -> TaskContract + frozen VerificationPlan
 -> Run + frozen RunInputManifest
 -> Steering / Pause / Resume / Takeover
 -> Run Complete
 -> DeliveryReceipt
 -> Evidence
 -> Verification
 -> Closure
~~~

This is a contract/invariant vertical slice.

It is not yet durable production execution.

## 5. Not implemented yet

### Durable persistence
- PostgreSQL Store adapter is not implemented yet.
- business-state + audit + outbox atomic transaction is defined by ADR-002 but not yet executed against PostgreSQL.
- migration exists but has not yet been validated by a real PostgreSQL integration test.

### Orchestration
- Temporal workflow implementation.
- outbox dispatcher.
- durable waits/retries.

### Runtime/product execution
- Codex-specific Runtime adapter/profile.
- Workspace/worktree manager.
- rootless sandbox/resource/network policy.
- Context attachment/materialization.
- Session Gateway/WebSocket attach.

### Policy / identity
- OPA integration.
- worker enrollment/mTLS.
- credential broker.
- production capability grants.

### External engineering facts
- Git provider adapter.
- CI provider adapter.
- Artifact store integration.
- Device/HIL adapter.

### Assurance
- independent Review workflow.
- Evidence issuer/trust registry.
- Artifact-byte verification/store.

### WorkBuddy
- WorkBuddy connector/front-door integration.

### Real product evidence
- no retained real Feature pilot yet.
- no retained real Debug pilot yet.
- Skills remain DEFINED, not PILOTED/PROVEN.

## 6. Immediate implementation queue

P0:
1. PostgreSQL Store adapter.
2. PostgreSQL integration test for CAS + state/audit/outbox atomicity.
3. outbox dispatcher contract.
4. durable audit writes/checkpoints.
5. workspace/worktree manager.
6. Codex Runtime adapter on top of Session Supervisor.
7. Control API persistence switch.

P1:
8. Temporal Run lifecycle.
9. OPA Action Gateway authorizer.
10. Git/CI provider adapter.
11. Artifact store.
12. CLI commands for full work/run lifecycle.
13. first retained Feature pilot.

P2:
14. Debug pilot with HypothesisRegistry.
15. sandbox/resource policy.
16. recovery/reconciliation drill against real PostgreSQL.
17. M2 Device/HIL only after Feature + Debug pilots are retained.

## 7. Readiness statement

Current claimable maturity:

> **Core architecture: clean-slate and implementation-focused.**
>
> **Core invariants: implemented with unit/API tests.**
>
> **Current Go main: repeatedly verified by real GitHub PR CI.**
>
> **M0: materially underway, not complete.**
>
> **M1: not yet achieved because persistence, real Runtime/workspace, external facts and real pilots are missing.**
>
> **Production readiness: not claimed.**

No research-round count, schema count or green bootstrap CI may be used to claim M1/production maturity.
