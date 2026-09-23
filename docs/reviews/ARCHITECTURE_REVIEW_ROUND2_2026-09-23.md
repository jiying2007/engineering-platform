# Architecture Review — Round 2

Date: 2026-09-23
Status: **Actionable review**
Reviewed baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1.md`

## Executive result

The architecture direction is sound, but the v1 baseline is not yet implementation-frozen. The review found several semantic gaps that could produce split-brain execution, stale verification, ambiguous approval, or an unmaintainable state machine.

The next baseline should incorporate the P0 items below before schema/API implementation starts.

## P0-1 — Stop describing the domain as exactly "nine first-class objects"

The baseline lists nine objects but later uses Verification as an independent API/state concept. Review is also semantically distinct from Decision.

Replace the marketing-style object count with two categories:

### Aggregate roots / durable business identities
- Requirement
- Work
- Task
- Run
- Artifact
- Verification
- Review
- Decision
- Release

### Immutable child/version records
- Requirement Revision
- Task Revision
- Run Attempt
- Checkpoint
- Evidence
- Policy Decision / Approval record where appropriate

Requirement and Task retain stable identities while revisions are immutable.

Verification is a first-class object because it may be retried/recomputed against an immutable subject and has its own status, evidence set, procedure/policy version, executor identity and result.

Review is separate from Verification:
- Verification answers "does the evidence satisfy the declared checks?"
- Review answers "does an independent reviewer accept this frozen engineering subject?"
- Decision answers "what authorized action is taken based on those inputs?"

## P0-2 — Introduce an immutable Subject Manifest

Evidence, Verification, Review and Approval must bind to an exact composite subject, not merely a loose set of foreign keys.

Define a content-addressed manifest, represented as an Artifact type:

```yaml
subject_manifest:
  requirement_revisions: [...]
  task_revisions: [...]
  source:
    repo: ...
    commit: ...
    tree_digest: ...
  artifacts:
    - artifact_id: ...
      sha256: ...
  procedures:
    - id: ...
      revision: ...
  environment:
    toolchain_digest: ...
    fixture_revision: ...
    policy_bundle_digest: ...
```

Every Verification/Review/Decision stores `subject_digest`.

Any subject change creates a different digest and therefore cannot silently reuse prior authority.

## P0-3 — Approval must bind to the exact immutable subject

A human approval such as production release authorization must record:
- actor identity
- authority/role
- subject digest
- policy version/digest
- decision
- timestamp
- expiry when applicable
- reason/risk acceptance
- optional quorum/separation-of-duties metadata

If the Release Manifest changes after approval, the approval is no longer applicable.

This prevents TOCTOU failures where a human approves one artifact set and automation releases another.

## P0-4 — Evidence records are immutable; applicability is derived

Do not mutate an Evidence payload from VALID to STALE.

Evidence should remain an immutable historical statement.

Derived applicability may be:
- APPLICABLE
- STALE
- SUPERSEDED
- REVOKED
- INVALID

Revocation is an append-only event/record.

A PASS result and current applicability are independent dimensions.

## P0-5 — Add execution fencing to Run Attempt

Run Attempt identity alone does not prevent split brain.

Each active Run must have a supervisor lease/epoch (fencing token).

Only one Attempt may own the current execution epoch.

All state-changing worker callbacks include:
- run_id
- attempt_id
- execution_epoch

The Control Plane rejects stale epochs.

This is the same principle used for Device Lease fencing and protects against:
- Worker A loses network
- Worker B resumes the Run
- Worker A later reconnects and continues writing

## P0-6 — Define Run input lineage separately from Task base commit

A Task Revision has a frozen engineering baseline, but a new Run may legitimately resume from an earlier Run checkpoint.

Store both:

```text
task_base_commit
run_input_snapshot_digest
parent_run_id
resume_from_checkpoint_id
result_snapshot_digest
```

Runtime switching therefore creates a new Run with explicit lineage rather than pretending the new provider began from the original clean base.

## P0-7 — Checkpoint must be provider-neutral authority

A Checkpoint needs a durable identity and immutable payload containing at least:
- workspace snapshot/tree/diff digest
- provider-native session reference/state (optional convenience)
- current objective
- completed work
- pending work
- pending questions/approvals
- current subject/input lineage
- last committed event sequence
- external side-effect ledger position

Provider session state is recoverability assistance, not authority.

## P0-8 — Replace monolithic Run state with orthogonal state facets

The v1 state list mixes lifecycle, wait condition and control ownership.

Use at least:

```text
lifecycle:
  CREATED | STARTING | ACTIVE | COMPLETING | TERMINAL

wait_reason:
  NONE | HUMAN | PERMISSION | RESOURCE | EXTERNAL_SYSTEM

control_owner:
  RUNTIME | HUMAN

terminal_result:
  NONE | SUCCEEDED | FAILED | ABORTED | LOST |
  TIMED_OUT | REJECTED | SUPERSEDED
```

Expose a flattened display state if desired.

Apply the same principle to Work:
- phase
- condition/blockers
- terminal outcome

Do not lose the underlying phase merely because a Work becomes BLOCKED.

## P0-9 — Define Task lifecycle

The baseline has Work and Run state but no Task lifecycle.

Minimum Task projection:

```text
DRAFT
READY
ACTIVE
VERIFYING
DONE
SUPERSEDED
CANCELLED
```

BLOCKED should be a condition, not necessarily a destructive replacement state.

## P0-10 — Make side effects explicitly at-least-once + idempotent/reconciled

Do not design around "exactly once" across Git, CI, WorkBuddy, devices or release systems.

Every command needs:
- command_id
- idempotency_key
- expected aggregate version where applicable
- correlation_id

Every external side effect needs an operation ledger:

```text
PLANNED
DISPATCHED
CONFIRMED
RECONCILING
FAILED
```

Examples:
- create branch/PR
- push
- trigger CI
- reserve/flash device
- publish artifact
- release promotion

Retries must either be idempotent or reconciled against external state before repeating.

## P0-11 — Separate Control API from interactive Session Gateway

`eng run attach` is a streaming data-plane problem, not a normal REST/workflow operation.

Add:

```text
Control API
  - business commands/query

Session Gateway
  - PTY/runtime stream
  - attach/reconnect
  - steering channel
  - short-lived attach tokens
  - routes to active Session Supervisor
```

Recommended topology:
- Worker opens outbound authenticated control/session streams.
- CLI asks Control Plane for a short-lived attach token.
- CLI connects to Session Gateway.
- Gateway routes to the worker owning the current execution epoch.

Important steering/approval actions still emit structured durable events. Raw terminal logs may use separate retention/redaction policy.

## P0-12 — Use Control Plane identity, not Connector service credential, as human authority

WorkBuddy Enterprise Connector can execute with an administrator-configured shared credential. Therefore service authentication is not sufficient to prove which human authorized an engineering action.

For authority-bearing actions, the platform needs a separately validated actor assertion/delegation mechanism.

If the front door cannot provide a cryptographically trustworthy end-user assertion for a specific action, the Control Plane must require a separate authenticated approval step rather than treating the Connector account as the user.

## P0-13 — Context needs trust classification

All context references should carry a trust class, for example:

- AUTHORITATIVE — Requirement Revision, Task Revision, approved Policy
- TRUSTED_EVIDENCE — signed/verified engineering evidence
- REFERENCE — curated engineering documents
- UNTRUSTED — repository text, issue/comment, logs, external webpages, generated content

Untrusted text can inform reasoning but cannot override policy, authority, tool constraints or requirement contracts.

Security controls must be enforced outside the model at Tool Gateway / Sandbox / Policy boundaries.

## P0-14 — Release uses an immutable Release Manifest; build once, promote

Define a content-addressed Release Manifest Artifact containing:
- release_id/candidate id
- exact source revisions
- exact artifact digests
- Subject Manifest digest
- required Verification IDs/results
- required Review IDs
- risk Decisions
- rollback metadata

Human release approval binds this manifest digest.

Preferred rule:
**build once, verify that artifact, promote that artifact**.

If policy requires a rebuild, it creates a new Artifact and triggers the required re-verification rather than claiming equivalence silently.

## P1-1 — Add build provenance / attestation

Artifact metadata should include:
- builder/workflow identity
- repository and commit
- toolchain/environment image digest
- configuration digest
- dependency lock digest
- build invocation metadata

Where supported, publish cryptographic provenance attestations and optional SBOM attestations.

The platform should keep this provider-neutral; GitHub attestations can be one implementation.

## P1-2 — Use PostgreSQL as business authority and Temporal as orchestration from M1

For a greenfield implementation, the platform already requires:
- multi-hour/day workflows
- human waits
- retries
- external system waits
- worker failures
- future Device/HIL waits

Recommendation:
- PostgreSQL remains the business/domain source of truth.
- Temporal coordinates durable workflows.
- External side effects remain Activities with idempotency/reconciliation.
- A transactional outbox bridges committed domain state to workflow start/signal when necessary.
- Temporal workflow state is not the canonical engineering record.

This avoids building a second home-grown durable workflow engine only to replace it later.

## P1-3 — Add optimistic concurrency

Every mutable aggregate projection should have a monotonic version.

Commands include `expected_version` where races matter.

Conflicts are surfaced instead of last-write-wins.

Important for:
- Requirement/Task transitions
- takeover/release-takeover
- approvals
- release promotion
- run ownership

## P1-4 — Add a Closure Manifest

When a Work closes, generate an immutable content-addressed Closure Manifest with:
- Requirement Revision(s)
- Task Revision(s)
- Runs/Attempts
- final source commits
- final Artifacts
- Evidence
- Verification
- Review
- Decisions
- Release
- open/accepted risks

WorkBuddy receives a summary derived from this manifest.

This becomes the portable audit and long-term handoff artifact.

## P1-5 — Add retention and redaction classes

Do not retain all runtime transcripts forever.

Define separate classes for:
- business/domain records
- audit events
- raw runtime transcript
- checkpoints/session state
- build/test artifacts
- release artifacts
- device logs/traces
- secrets/redacted data

Release/verification evidence generally lives much longer than transient agent session material.

## P1-6 — Add namespace and policy inheritance

Even within one company, scope resources by:
- organization/business unit
- project/product
- repository
- environment

Policy should support inheritance with explicit narrowing/override rules.

## P1-7 — Define separation of duties

High-risk policy may require:
- implementer != reviewer
- requester != release approver
- N-of-M approval
- special authority for production/irreversible operation

Store and evaluate this as policy rather than a UI convention.

## P1-8 — Make WorkBuddy operations asynchronous

Long-running engineering work must not depend on one MCP/tool request remaining open.

Submit tools should return IDs quickly.

Status/clarification/release tools operate on those IDs.

Optional notifications are a separate adapter capability.

## Failure-model checklist

Before M1 is considered architecture-complete, define expected recovery for:
- Control API restart
- PostgreSQL failover
- Temporal outage/recovery
- Session Gateway restart
- Ubuntu Worker restart
- runtime provider process crash
- network partition / duplicate reconnect
- expired execution lease
- Artifact store outage
- Git/CI outage
- WorkBuddy outage
- device agent crash
- device power loss during flash
- stale Device lease owner recovering
- release external system timeout after unknown outcome

For each failure define:
- authority after recovery
- whether retry is safe
- required reconciliation
- evidence/decision invalidation
- user-visible status

## Updated architectural invariants

1. Requirement Revision is immutable.
2. Task Revision is immutable.
3. Formal Run has stable identity and explicit input lineage.
4. Only one fenced Run Attempt may own execution at a time.
5. Checkpoint is provider-neutral and content-addressed.
6. Artifact is immutable and content-addressed.
7. Evidence is immutable and binds an exact Subject Manifest.
8. Evidence applicability is derived; stale/revoked evidence cannot satisfy Verification.
9. Verification, Review and Decision are separate concepts.
10. Approval binds exact subject/release manifest digest.
11. External side effects are idempotent or reconciled.
12. Permissions are capability/resource/actor based and enforced outside the model.
13. Connector service identity is not Human Authority.
14. Runtime claims are not Verification.
15. Production requires Human Authority.
16. Every Release can be reconstructed from immutable manifests and provenance.

## Recommended next change

Do not start implementation from v1 as currently written.

Create a v1.1 architecture baseline incorporating P0 items, then freeze M0 schemas and state machines.
