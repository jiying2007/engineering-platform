# Architecture Review — Round 3: Implementation Readiness

Date: 2026-09-23
Reviewed baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md`
Focus: protocol boundaries, trust, consistency, degraded operation and M0/M1 codability.

## Result

v1.1 is now structurally sound enough to keep as the target architecture. No additional top-level Plane is required.

However, implementation should **not** start from prose alone. The decisions below must be frozen as ADR/schema contracts during M0. Otherwise the team will re-litigate security and consistency semantics inside code.

## P0-1 — Worker identity, enrollment and ownership

An Ubuntu Worker is a privileged execution endpoint and needs its own machine identity.

Freeze:
- worker_id
- enrollment record
- mTLS/service identity
- project/environment scopes
- health/capability inventory
- lease/heartbeat
- revocation
- current software version

Workers should initiate outbound authenticated connections where possible so lab/desktop Ubuntu hosts do not need broadly exposed inbound ports.

A Worker must never be trusted merely because it can reach the Control Plane network.

## P0-2 — Split local tools from privileged platform actions

"Tool Gateway" is too broad unless execution classes are explicit.

### Local execution tools

Examples:
- compiler
- local unit tests
- grep/search
- repository read
- workspace-local scripts

Run inside the execution sandbox subject to filesystem/network/resource policy.

### Privileged platform actions

Examples:
- Git push / PR creation
- CI dispatch
- shared device reserve/flash
- secret-backed external API call
- artifact publication
- release promotion
- production operation

These execute outside the untrusted agent sandbox through an authenticated Platform Action Gateway.

The runtime requests an action; the platform evaluates actor/resource/capability policy and executes the action without exposing raw credentials to model-generated code.

## P0-3 — Freeze transport boundaries

Recommended reference implementation:

```text
WorkBuddy Connector
    -> HTTPS/MCP
Control API
    -> PostgreSQL + Temporal

eng CLI
    -> HTTPS for commands/queries
    -> WebSocket for interactive attach

Session Gateway
    <-> outbound Worker session stream

Worker
    -> mTLS gRPC/streaming control channel
    -> artifact object store using scoped upload/download grants
```

Exact libraries may vary, but control-plane commands and session streaming must remain separate concerns.

## P0-4 — One transactional business authority

PostgreSQL is the only mutable business-state authority.

For a state transition:

```text
DB transaction
  - validate expected aggregate version
  - mutate projection
  - append audit event
  - append outbox record
commit
```

Only after commit may outbox consumers:
- signal/start Temporal
- notify WorkBuddy
- dispatch external integration work

Do not dual-write PostgreSQL + Temporal + external service directly in one request path.

Per-aggregate sequence/version is authoritative for ordering; do not rely on worker clocks for ordering.

## P0-5 — Freeze a Run Input Manifest

Before a Formal Run starts, create an immutable `Run Input Manifest` Artifact.

It contains:
- Requirement Revision refs/digests
- Task Revision
- source input repositories / commits / tree digests
- selected context refs + trust classes
- runtime provider/profile
- policy bundle digest
- capability grant
- toolchain/environment profile
- parent Run / Checkpoint lineage if any

Store `run_input_digest` on the Run.

This makes "what exactly did this runtime start with?" answerable even after context sources evolve.

## P0-6 — Freeze a Run Receipt

At Run completion, create an immutable `Run Receipt` Artifact containing:
- Run / Attempt lineage
- Run Input Manifest digest
- final workspace/tree digest
- final commit(s) if created
- produced Artifact IDs/digests
- Evidence IDs
- human steering/takeover summary refs
- policy/approval decisions
- tool side-effect ledger summary
- terminal result
- runtime/provider identity/version
- usage/cost metadata where available

The Run Receipt is not Verification; it is the canonical execution provenance summary.

## P0-7 — Source schema must not hard-code one repository

M1 may support one repository operationally, but the domain schema should allow:

```yaml
source_inputs:
  - repo: ...
    commit: ...
    role: primary
  - repo: ...
    commit: ...
    role: dependency
```

Embedded products commonly span Linux/BSP, MCU, test tooling and fixtures.

Do not require a breaking schema migration when M2/M3 introduces multi-repo work.

## P0-8 — Formal result cannot remain an ambiguous dirty worktree

A completed implementation Run must freeze its result as one of:
- source commit(s), or
- immutable workspace/tree snapshot explicitly pending commit

Verification policy decides which is acceptable.

Release-quality source should normally resolve to immutable SCM commit(s).

Track tree digest independently of Git commit so human takeover/uncommitted checkpoints remain auditable.

## P0-9 — Policy decisions are versioned records

The policy engine is not only a function that returns allow/deny.

Every material evaluation records:
- policy_bundle_digest
- actor
- resource
- requested capability/action
- relevant environment/risk attributes
- result: allow / deny / require_approval
- required authority/quorum
- decision timestamp
- input digest

Default is deny for unrecognized privileged actions.

M1 may implement the evaluator in-process. The schema must allow later OPA/Cedar/other policy engines without changing domain semantics.

## P0-10 — Define a controlled degraded / break-glass path

A company-wide engineering platform cannot make all engineering impossible during a Control Plane outage.

Define two modes:

### Explore / local degraded mode

Local analysis and experiments may continue, but cannot claim Formal provenance or use privileged shared resources.

### Formal break-glass

For exceptional business-critical work:
- explicit authorized break-glass Decision
- bounded scope/time
- local signed evidence bundle
- no production operation without the required authority
- mandatory later reconciliation/import
- prominent non-normal-path audit marker

Break-glass is not a hidden bypass and should be rare.

## P1-1 — Artifact upload must be two-phase

Avoid database metadata pointing at missing/corrupt blobs.

Recommended:
1. request artifact upload
2. receive scoped upload grant
3. upload to object store
4. server verifies digest/size
5. finalize Artifact record

Abandoned uploads become garbage-collection candidates.

Immutable release artifacts should use retention/lock policy appropriate to the environment.

## P1-2 — Runtime/provider credentials stay outside agent-generated code

Provider credentials and third-party credentials should be separated.

Where possible:
- runtime connection credential is narrow and environment-specific
- third-party credentials remain in Platform Action Gateway / trusted proxy
- agent sandbox receives placeholders/scoped references, not reusable secrets
- redact secrets from event/transcript collection

This is aligned with current sandbox security guidance that long-lived third-party credentials should remain outside untrusted sandbox code.

## P1-3 — Transcript is not the audit log

Keep:
- structured Event Log for authority/audit
- runtime transcript for debugging/UX

Do not depend on replaying terminal text to reconstruct state.

Transcript retention can be shorter and must support redaction.

## P1-4 — Add resource scheduling semantics

Worker metadata should expose:
- CPU/memory capacity
- architecture
- SDK/toolchain capabilities
- attached local devices
- labels/pools
- occupancy

Runtime Scheduler selects an eligible Worker and receives a Worker execution lease.

M1 may use a single Worker but must retain the scheduling contract.

## P1-5 — Add environment dimension

Do not infer risk only from action type.

Every privileged action should include an environment/resource scope such as:
- local
- dev
- validation
- staging
- production

The same capability (for example `device.flash`) can have very different policy by environment.

## P1-6 — Verification procedure is versioned input

Device/HIL/test Verification must record exact:
- procedure ID/revision
- test software Artifact
- fixture revision/calibration
- thresholds/baseline revision
- environment
- sample/run count for non-deterministic tests

A PASS without the procedure/environment identity is weak evidence.

## P1-7 — Release promotion is a stateful history

Release should retain promotion history:
- candidate created
- verification complete
- approval
- staging promotion
- production promotion
- rollback/revocation where applicable

Every promotion binds the same immutable Release Manifest digest unless a new manifest/approval is created.

## P1-8 — Observability and cost are cross-cutting

Use OpenTelemetry-compatible trace/correlation identifiers across:
- Control API
- Temporal workflow/activity
- Worker
- Session Gateway
- Tool/Action Gateway
- CI/device integrations

Useful operational metrics:
- active Runs
- resume success/RTO
- Worker/session disconnects
- stale epoch rejections
- external reconciliation count
- artifact finalization failures
- approval wait time
- device lease utilization
- runtime token/API/compute cost

## M0 documents/specs that should exist before implementation

1. `docs/adr/0001-domain-authority.md`
2. `docs/adr/0002-temporal-orchestration.md`
3. `docs/adr/0003-worker-trust-and-session-channel.md`
4. `docs/adr/0004-tool-and-action-boundaries.md`
5. `docs/adr/0005-artifact-evidence-manifests.md`
6. `docs/adr/0006-policy-and-human-authority.md`
7. `docs/adr/0007-idempotency-and-reconciliation.md`
8. `docs/adr/0008-break-glass.md`
9. JSON Schema for Requirement/Task/Run/Attempt/Checkpoint/Artifact/Evidence/Verification/Review/Decision/Release
10. Event/Command envelope schema
11. State transition table with guards
12. Initial OpenAPI for Control API
13. Worker control protocol contract
14. Session Gateway attach protocol
15. M1 failure-injection acceptance matrix

## M1 go/no-go criteria

M1 should not be called complete because "Codex modified code".

Go requires:
- Run created from immutable Run Input Manifest
- one fenced active Attempt
- attach/steer/checkpoint/resume works after process/worker reconnect
- stale execution epoch is rejected
- final Run Receipt is immutable
- result Artifact/source tree is content-addressed
- CI Evidence binds exact subject
- Verification consumes currently applicable Evidence
- duplicate API/connector command does not duplicate side effects
- WorkBuddy receives closure from immutable closure data
- secrets are absent from agent-visible long-lived credentials
- audit reconstruction succeeds without parsing terminal transcript

## Conclusion

The reviewed architecture should proceed.

The most important implementation principle now is:

> **Make every authority-bearing boundary explicit and content-addressed; make every side effect fenced, idempotent or reconciled; keep streaming interaction separate from durable business state.**

After these contracts are frozen, the project is ready to move from architecture review into M0 implementation.
