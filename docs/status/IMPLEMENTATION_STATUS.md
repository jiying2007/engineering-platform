# Implementation Status

Reviewed base: `2ff3de8d0520a84573a59d72cd460bf0492534af` (#34).
Stage: **Authenticated Core + actual preparation + one-shot offline execution;
no qualified real Codex or Feature/Debug pilot**.

## Canonical scope

Embedded Core architecture, capability model, M0/M1 plan and ADR-001/002/003 remain
canonical. Independent of digital-worker; no service/domain catalogue expansion.

## Current assembled path

Work/Task/frozen Run -> atomic outbox/inbox relay -> mTLS preparation Worker ->
actual approved Context bytes and independent Git snapshot -> new exact-profile
Core authorization -> recheck current local approvals/source/context -> inspected
read-only, non-root, network-disabled container -> bounded output and confirmed
cleanup -> current-lease execution receipt -> actual eng CLI readback.

The offline task is explicitly opted into immutable Task/Run and existing exact
certificate ActionGrant. It is one reservation per Run, not a retryable generic
shell service or an online privileged Action provider. All receipt kinds retain
separate meanings: INPUT_VALIDATED, WORKER_ATTESTED_PREPARATION and
WORKER_ATTESTED_OFFLINE_EXECUTION. A process exit does not complete an engineering
Run or create Evidence/Verification/Delivery/Closure automatically.

## Capabilities and remaining limits

| Area | Implemented | Remaining boundary |
| --- | --- | --- |
| Core persistence | Immutable input/task identities, Run/Session epochs, receipt model, PostgreSQL business/audit/outbox | Memory-store deep-copy/conformance and independent Review |
| Worker admission/preparation | Real relay, mTLS clients, exact profile/identity claims and generation leases; actual approved source/context preparation | Approval snapshots require restart; no online enrollment/tenant ACL |
| Workspace | Independent exact-base Git objects, sanitized trusted Git, ownership-safe slots, actual source/config checks and process cancellation | Trusted source metadata/host parents; quota and retention |
| Offline execution (this change) | Fresh exact-profile authorization; one reservation per Run; current Run/recovery/lease renew/finish; image+guard pin; real constrained container; independent timeout/output limit; durable output receipt and CLI | Offline read-only only; bounded not instantaneous revocation; trusted local daemon/kernel/Worker; no VM-level or rootless qualification |
| Failure recovery | Historical identical finish replay, no automatic execution replay; UNKNOWN/expired-unreconciled status and local reconciliation records | Actual operator reconciliation/orphan sweep and production restore drills |
| Prepared Codex adapter | Bounded JSONL, typed initialize/thread/turn/steer/interrupt and deny-only approvals | Not invoked by offline lane; pinned real binary/schema and authorized online execution still needed |
| Action/recovery | Existing durable action ledger, authenticated grants and independent recovery-completion gate | External privileged providers and authoritative completion verifier not assembled |
| Engineering delivery | Frozen verification and delivery/evidence/closure contracts | Real Git/CI/Artifact facts, independent Review and Feature/Debug pilots |

## Evidence

- #33 admission: main `71a4877e2eb4973196cd612852fba19277919197`, PR
  `36031414023`, fresh main `36031860646` passed.
- #34 preparation: main `2ff3de8d0520a84573a59d72cd460bf0492534af`, PR
  `36075559599`, fresh main `36075797271` passed.
- This offline change requires both final PR checks and fresh-main checks. Local
  standard-library sandbox tests and formatting are only partial evidence.

CI preserves full Go 1.25/PostgreSQL 17/race and all earlier repeated regressions.
It adds shuffled offline authority DB tests and a separate mandatory real Docker
job. That job cannot silently skip on a missing Engine. It builds fixture images
locally, verifies actual runtime containment/timeouts/output bounds and executes
compiled Worker/eng through live mTLS/PG/relay with real prepared bytes. Probe
output is explicitly offline fixture evidence, not model output or a product pilot.

## Deployment and next acceptance gates

Version-5 migration is additive and operator-controlled. No code merge deploys,
provisions a daemon/image/guard/credentials or changes production data. Rootful
Engine access is privileged; isolate the Worker host and never mount its socket
or credentials into child execution. Failed/ambiguous Runs cannot be auto-retried.

Next: qualified Codex integration under explicit network/approval boundaries,
trusted artifact/Git/CI publication, independent Review, retained reconciliation
and restore drills, then real Feature/Debug pilots. No new extension domains.
**M1 and production readiness are not claimed.**

Contracts: docs/implementation/OFFLINE_EXECUTION_V1.md and the retained
WORKER_PREPARATION_V1 / WORKER_ADMISSION_V1 / PREPARED_RUNTIME_PROTOCOL_V1 /
AUTHENTICATED_CONTROL_API_V1 / OUTBOX_DISPATCH_AUTHORITY_V2.
