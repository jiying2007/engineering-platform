# AI Native Engineering Platform v1.1

Status: **Architecture Baseline — Round-2 reviewed**
Date: 2026-09-23
Repository: `jiying2007/engineering-platform`

## 1. Purpose

This platform turns product requirements into a formal, auditable engineering chain:

```text
Requirement
  -> Work / Task
  -> Human + AI execution
  -> Git / Build / CI
  -> Immutable Artifact
  -> Device / HIL
  -> Evidence
  -> Verification
  -> Independent Review
  -> Human Authority
  -> Release
```

It is not a WorkBuddy-to-agent proxy, prompt portal, or multi-agent demo. It is the engineering control plane for AI-native development.

## 2. Five-plane architecture

### 2.1 Experience Plane

Windows / WorkBuddy is the corporate R&D front door.

Responsibilities:
- submit and revise requirements
- view work status and blockers
- answer clarifications
- accept product risk
- request/approve release
- inspect delivery results

It must not expose raw SSH, shell, Git, device flashing, long-lived credentials, or unrestricted runtime prompts.

### 2.2 Control Plane

The Engineering Control Plane is the workflow and state authority.

Responsibilities:
- identity and authorization
- requirement/work/task registries
- context resolution
- planning and durable workflow
- policy evaluation
- run and run-attempt registry
- artifact registry
- evidence registry
- verification, decision, review and release state

It answers:
- which requirement revision is active?
- which task revision is being executed?
- what base commit did the run start from?
- who/what changed the workspace?
- which artifact was tested?
- what evidence proves each acceptance criterion?
- why is a release allowed?

### 2.3 Execution Plane

Ubuntu Engineering Workers host the actual engineering environment.

A worker contains:
- execution sandbox
- workspace manager
- session supervisor
- runtime gateway
- tool gateway
- credential broker client
- artifact collector
- optional device client

Runtime providers include Codex, Claude and future providers.

### 2.4 Evidence Plane

Engineering facts are represented as immutable artifacts and evidence with provenance.

Sources include:
- Git commits and diffs
- build outputs
- CI results
- unit/integration tests
- device/HIL measurements
- raw logs/traces
- independent review outputs

Runtime prose is not evidence by itself.

### 2.5 Assurance Plane

Assurance performs:
- verification
- independent review
- risk decisions
- release authorization
- production promotion

Production and irreversible actions require Human Authority, but may still be executed by trusted automation.

## 3. Authority boundaries

Canonical authority is separated as follows:

| Concern | Authority |
|---|---|
| Human-facing collaboration | WorkBuddy |
| Formal workflow/state | Engineering Control Plane |
| Engineering execution | Ubuntu Worker |
| Runtime reasoning/coding | Runtime Provider |
| Source/build/device facts | Git / CI / Device / HIL |
| Deliverable identity | Artifact Registry |
| Proof | Evidence Registry |
| Production decision | Human Authority |

A runtime may propose a result. It cannot self-declare verified delivery.

## 4. Core domain model

Do not reduce the domain to a fixed marketing count of "objects". Separate stable aggregate identities from immutable version/child records.

### 4.1 Aggregate roots / durable identities

- Requirement
- Work
- Task
- Run
- Artifact
- Verification
- Review
- Decision
- Release

### 4.2 Immutable version / child records

- Requirement Revision
- Task Revision
- Run Attempt
- Checkpoint
- Evidence
- Policy/Approval records where required

Relationship:

```text
Requirement
  └─ Requirement Revision
          |
          v
         Work
          |
          v
Task ── Task Revision DAG
          |
          v
         Run
      ├─ Run Attempt
      ├─ Checkpoint
      ├─ Artifact
      └─ Evidence
          |
          v
     Verification
          |
          v
        Review
          |
          v
       Decision
          |
          v
        Release
```

### 4.3 Requirement / Requirement Revision

Requirement is the stable identity. A Requirement Revision is immutable once accepted for formal development.

Conversation and source documents are context, not authority.

Requirement changes create a new revision and trigger impact analysis.

Minimum revision fields:
- requirement_id
- revision
- title / goal
- scope / non_goals
- constraints
- acceptance criteria
- source snapshot reference/digest

### 4.4 Work

A Requirement Revision may create one or more engineering Works.

Work is the primary delivery object visible to engineering management.

### 4.5 Task / Task Revision

Task is the stable engineering identity. Task Revision is the immutable engineering contract.

Changes to repository, modules, technical approach, verification or outputs create a new Task Revision even when the product requirement itself is unchanged.

Minimum revision fields:
- task_id / revision
- work_id
- type
- repository / frozen task_base_commit
- inputs
- required outputs
- required verification
- policy profile

### 4.6 Run

A Run is one logical engineering execution with fixed:
- Task Revision
- runtime identity/provider
- policy profile
- task_base_commit
- run_input_snapshot_digest
- optional parent_run_id
- optional resume_from_checkpoint_id

A Run is not a PID and is not equivalent to a provider-native CLI session.

A runtime switch creates a new Run with explicit lineage.

### 4.7 Run Attempt and execution fencing

A Run Attempt is one concrete execution epoch.

Only one Attempt may own active execution for a Run at a time.

Every active execution has an `execution_epoch` / fencing token. State-changing worker callbacks carry:
- run_id
- attempt_id
- execution_epoch

Callbacks from stale epochs are rejected.

Worker restart, process crash or network loss may end ATTEMPT-01 and resume the same logical Run as ATTEMPT-02.

### 4.8 Checkpoint

Checkpoint is provider-neutral authority for resumability.

An immutable Checkpoint records at least:
- workspace snapshot/tree/diff digest
- optional provider-native session reference/state
- current objective
- completed and pending work
- pending questions/approvals
- input lineage / subject references
- last durable event sequence
- external side-effect ledger position

Provider-native session state is a convenience; the platform Checkpoint is authoritative.

### 4.9 Artifact

Artifact is a first-class immutable, content-addressed deliverable.

Examples:
- firmware/bin/elf
- OTA package
- executable/container
- model
- test package/report
- SBOM
- Subject Manifest
- Release Manifest
- Closure Manifest

Artifact identity binds:
- producer Run/Attempt
- source repository and commit
- build/toolchain/configuration digests
- content digest
- provenance/attestation references where available

Preferred release rule: build once, verify that artifact, promote that exact artifact. A rebuild creates a new Artifact and triggers required re-verification.

### 4.10 Subject Manifest

Verification, Review and authority-bearing Decision bind to an immutable content-addressed Subject Manifest.

It identifies the exact composite subject:
- Requirement Revision(s)
- Task Revision(s)
- source commit/tree digest
- Artifact digests
- Procedure revisions
- fixture/environment/toolchain digests
- policy bundle digest where relevant

Every Verification, Review and Approval stores `subject_digest`.

### 4.11 Evidence

Evidence is an immutable historical statement bound to an exact subject.

Evidence payload is never rewritten from VALID to STALE.

Keep two independent dimensions:
- test/result: PASS / FAIL / INCONCLUSIVE / ...
- applicability: APPLICABLE / STALE / SUPERSEDED / REVOKED / INVALID

Applicability is derived from the current Subject Manifest and append-only revocation/supersession facts.

### 4.12 Verification

Verification is a first-class assessment of whether the required checks and currently applicable Evidence satisfy the declared acceptance/policy for one immutable subject.

It records:
- verification_id
- subject_digest
- evidence set
- procedure/policy version
- executor identity
- result
- timestamps

Verification may be recomputed without mutating the evidence it consumes.

### 4.13 Review

Review is distinct from Verification.

Verification asks whether declared checks are satisfied.

Review asks whether an independent reviewer accepts the frozen engineering subject, risks and change set.

### 4.14 Decision

A Decision records:
- actor / authority
- exact subject or release-manifest digest
- policy version/digest
- inputs/evidence/review
- chosen action
- rationale
- risk acceptance
- expiry/quorum/separation-of-duties metadata when required

### 4.15 Release

Release binds an immutable Release Manifest containing:
- Requirement Revision(s)
- exact source revision(s)
- exact Artifact digest(s)
- Subject Manifest digest
- required Verification(s)
- required Review(s)
- Decisions / accepted risks
- rollback metadata

Human release authorization binds the exact Release Manifest digest. A manifest change invalidates the prior approval.

### 4.16 Closure Manifest

When a Work closes, generate a content-addressed Closure Manifest that records the complete closure graph:
- requirements/tasks
- runs/attempts
- final source revisions
- final artifacts
- evidence
- verification
- review
- decisions
- release
- accepted/open risks

WorkBuddy delivery summaries are projections of this immutable closure record.

## 5. Acceptance to Evidence

Acceptance is mapped to evidence from the moment a requirement becomes formal.

Example:

| AC | Verification | Evidence | Status |
|---|---|---|---|
| AC-01 | Device | EV-101 | PASS / VALID |
| AC-02 | HIL | EV-108 | PASS / VALID |
| AC-03 | Thermal | - | MISSING |

Verification passes only when every required acceptance criterion has acceptable Evidence whose applicability is APPLICABLE to the exact current Subject Manifest. A historical PASS whose subject no longer matches is insufficient.

## 6. Interactive Runtime Session

Formal AI engineering is interactive, not a batch job.

A Run must support:
- execute
- observe
- attach
- steer
- add context
- ask human
- request permission
- pause/resume
- checkpoint
- human takeover
- abort
- switch runtime via a new Run

### 6.1 Session Supervisor

The Session Supervisor is more important than any individual runtime adapter.

It owns:
- process supervision
- runtime I/O channel
- event capture
- checkpoints
- attempts
- pause/resume
- terminal attach
- human steering
- takeover
- failure recovery

Runtime adapters only translate provider-specific launch, resume, permission and metadata behavior.

### 6.2 Session Gateway

Interactive attach is a streaming data-plane concern, separate from ordinary business REST/workflow operations.

Add a Session Gateway responsible for:
- PTY/runtime streaming
- attach/reconnect
- short-lived attach tokens
- routing to the active Session Supervisor
- enforcing current Run execution epoch
- controlled transcript capture/redaction

Recommended path:

```text
eng CLI
  -> Control API: request short-lived attach token
  -> Session Gateway
  -> Worker outbound session channel
  -> active Session Supervisor
```

Important steering/approval actions still emit structured durable events. Raw terminal I/O may use a separate retention policy.

## 7. Change classification

Every intervention must be classified.

### Run Steering

Goal, scope, acceptance and task contract remain unchanged.

The current Run continues.

### Task Replan

Requirement remains unchanged, but engineering contract changes materially.

Action:
```text
pause
-> Task Revision
-> replan
-> new Run
```

### Requirement Revision

Goal, scope, acceptance or constraints change.

Action:
```text
REQ rN
-> REQ rN+1
-> impact analysis
```

## 8. Human Takeover

Human takeover is a first-class runtime state.

A human may directly modify the current formal workspace.

The platform records:
- actor
- timestamps
- diff/tree digest
- resulting Artifact impact
- Evidence invalidation impact

After takeover the runtime may resume from a checkpoint.

No evidence from a pre-change subject may silently remain authoritative after material human changes.

## 9. Workspace and execution isolation

Git worktree provides source workspace isolation only.

Formal execution also requires a security/resource sandbox.

Recommended Run sandbox:
- isolated worktree mount
- temporary HOME
- rootless container and/or Linux namespaces
- cgroup resource limits
- network policy
- process isolation
- controlled SDK/toolchain mounts
- short-lived injected credentials

Kubernetes is not required for the first implementation.

## 10. Identity and authorization

Connector authentication must not be treated as end-user authorization.

A WorkBuddy Connector may operate using an administrator-configured shared third-party credential. That service identity is therefore not sufficient proof of which human authorized an engineering action.

Authority-bearing operations require a separately validated actor assertion/delegation or a separate authenticated Control Plane approval step.

Identity chain:

```text
WorkBuddy user
  -> enterprise identity
  -> engineering actor
  -> role / authority / delegation
  -> capability policy evaluation
```

Minimum identity fields:
- actor_id
- tenant_id
- groups
- roles
- authority
- delegation
- authentication_source

## 11. Capability-based policy

Risk levels may exist for UI/reporting, but enforcement is capability-based rather than a single linear level.

Examples:
- filesystem.workspace_read
- filesystem.workspace_write
- git.read
- git.feature_branch_push
- git.merge
- network.allowed_domains
- device.reserve
- device.dev_flash
- device.production_flash
- secrets.github_ephemeral_token
- secrets.ota_signing_key
- release.propose
- release.production_approve

## 12. Credentials

Runtime providers do not receive long-lived credentials directly.

Flow:

```text
Runtime
  -> Tool Gateway
  -> Credential Broker
  -> short-lived, least-privilege, run-bound credential
```

Credentials must be:
- short lived
- revocable
- least privilege
- run/actor scoped
- audited

## 13. Device and HIL

Device/HIL is a formal subsystem, not a pile of scripts.

Components:
- Device Registry
- Lease Manager
- Device Agent
- Fixture Registry
- Procedure Registry

Device operations require a lease with:
- lease_id
- owner Run
- expiry
- fencing token

Every operation carries the current fencing token so a recovered stale worker cannot operate a device after ownership has moved.

Standard flow:

```text
reserve
-> lease
-> verify device identity
-> flash exact Artifact
-> reset
-> execute exact Procedure Revision
-> collect raw evidence
-> calculate result
-> publish Evidence
-> release
```

## 14. Review

The review subject is frozen:
- Requirement Revision
- Task Revision
- base/result SHA
- diff
- Artifact
- Evidence
- open risks

Reviewer must not mutate the reviewed subject.

Independence levels:
- R0 same-run self-check
- R1 independent session
- R2 independent Run/context
- R3 different runtime/provider
- R4 human specialist

Policy selects the required level based on risk. Different vendor is not always required; independent execution identity is the core property.

## 15. Human Authority

For production and irreversible changes:

```text
AI proposes
-> policy validates
-> human authorizes
-> trusted automation executes
```

Human Authority does not require manual Human Execution.

Examples:
- production OTA
- fuse/OTP
- mass flashing
- release promotion
- signing-key use

## 16. Explore and Formal modes

### Explore

Engineers may use Codex/Claude directly for:
- code reading
- learning
- experiments
- temporary RCA
- prototypes

Explore does not require a formal Work.

### Formal

Formal commit/PR, shared devices, product changes, HIL or Release must enter the formal object model.

### Promotion boundary

Explore output is untrusted context, not verified engineering output.

Promotion path:

```text
Explore result
-> import as context
-> Formal Task
-> Formal Run
-> reproduce/implement
-> Artifact
-> Verification
```

## 17. State model

Do not encode lifecycle, waiting reason, human takeover and terminal outcome into one flat enum.

### Run projection

Use orthogonal facets:

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

A flattened display state may be derived for UI.

### Task projection

Minimum phase:
- DRAFT
- READY
- ACTIVE
- VERIFYING
- DONE
- SUPERSEDED
- CANCELLED

BLOCKED is preferably a condition/blocker projection rather than a destructive replacement for the current phase.

### Work projection

Phase:
```text
DRAFT
-> READY
-> PLANNED
-> EXECUTING
-> VERIFYING
-> REVIEWING
-> RELEASE_READY
-> CLOSED
```

Track blocker/condition and terminal outcome separately so entering BLOCKED does not erase the underlying phase.

## 18. Event and command contract

Formal operations emit append-only events.

Every mutating command has:
- command_id
- idempotency_key
- correlation_id
- actor
- expected aggregate version where races matter

Minimum event envelope:
- event_id
- sequence
- aggregate_type / aggregate_id
- event_type
- actor
- source
- timestamp
- correlation_id
- causation_id
- schema_version
- idempotency_key where applicable
- payload
- payload_digest

Typical events:
- RUN_CREATED
- ATTEMPT_STARTED
- CONTEXT_LOADED
- TOOL_CALLED
- FILE_MODIFIED
- HUMAN_STEERED
- RUNTIME_QUESTIONED
- HUMAN_ANSWERED
- PERMISSION_REQUESTED
- PERMISSION_GRANTED
- CHECKPOINT_CREATED
- RUN_PAUSED
- TAKEOVER_STARTED
- TAKEOVER_FINISHED
- ARTIFACT_CREATED
- EVIDENCE_PUBLISHED
- RESULT_PROPOSED
- RUN_COMPLETED

Events are audit facts, not automatically knowledge.

Do not assume exactly-once side effects across Git, CI, WorkBuddy, devices or release systems.

Maintain an external-operation ledger:

```text
PLANNED
-> DISPATCHED
-> CONFIRMED

or

PLANNED
-> DISPATCHED
-> RECONCILING
-> CONFIRMED / FAILED
```

Retries must be idempotent or reconciled against external state before repeating.

## 19. Workflow implementation

For the greenfield reference implementation:

- PostgreSQL is the canonical business/domain source of truth.
- Temporal is the durable orchestration engine from M1.
- Temporal Workflow state is not the canonical engineering record.
- External calls and side effects execute as Activities with idempotency/reconciliation rules.
- A transactional outbox bridges committed domain changes to workflow start/signal where atomic cross-system behavior is needed.
- Mutable aggregate projections use optimistic concurrency/version fields.

This platform already requires human waits, retries, reconnects, worker crashes and future Device/HIL waits. Building a second home-grown durable workflow engine first would create avoidable migration work.

## 20. WorkBuddy Connector

Expose high-level business tools only, for example:
- submit_requirement
- revise_requirement
- get_requirement
- get_work_status
- get_blockers
- answer_clarification
- approve_plan
- accept_risk
- get_verification_status
- request_release
- approve_release
- get_release_status
- get_delivery_result

Never expose raw shell, SSH, Git, unrestricted runtime prompt, production secret or direct device flashing.

Connector operations are asynchronous by default: submit/start commands return stable IDs quickly. Long-running engineering execution must never depend on one MCP/tool request remaining open.

All connector requests include correlation/idempotency identifiers.

### Context trust

Context references carry a trust class such as:
- AUTHORITATIVE
- TRUSTED_EVIDENCE
- REFERENCE
- UNTRUSTED

Repository text, issue comments, logs, webpages and generated content cannot override Requirement, Policy, identity, authorization or Tool Gateway enforcement.

## 21. Ubuntu CLI

Canonical CLI: `eng`

Examples:

```bash
eng work list
eng work show WORK-421
eng work open WORK-421

eng task list WORK-421

eng run codex TASK-005
eng run attach RUN-812
eng run status RUN-812
eng run steer RUN-812 "先检查积分清零"
eng run pause RUN-812
eng run resume RUN-812
eng run takeover RUN-812
eng run checkpoint RUN-812

eng artifact show ART-812
eng evidence list WORK-421
```

The platform must preserve the native interactive runtime experience rather than replacing it with a weak custom coding UI.

## 22. Recommended implementation shape

Initial monorepo:

```text
engineering-platform/
├── cmd/
│   ├── eng/
│   ├── control-plane/
│   ├── session-gateway/
│   └── worker/
├── internal/
│   ├── identity/
│   ├── requirement/
│   ├── work/
│   ├── task/
│   ├── run/
│   ├── artifact/
│   ├── evidence/
│   ├── verification/
│   ├── review/
│   ├── decision/
│   ├── release/
│   ├── policy/
│   └── workflow/
├── runtimes/
│   ├── contract/
│   ├── codex/
│   └── claude/
├── worker/
│   ├── sandbox/
│   ├── workspace/
│   ├── supervisor/
│   ├── tool-gateway/
│   └── credentials/
├── connectors/
│   └── workbuddy/
├── device/
│   ├── registry/
│   ├── lease/
│   ├── agent/
│   └── procedures/
├── schemas/
│   ├── domain/
│   ├── events/
│   └── policy/
├── docs/
│   ├── architecture/
│   ├── adr/
│   └── reviews/
├── deploy/
└── tests/
```

Do not split into many microservices before the domain model stabilizes.

## 23. Delivery roadmap

### M0 — Domain Contract

Freeze:
- aggregate roots and immutable revision/child records
- Subject/Release/Closure Manifest schemas
- ID conventions
- Run/Task/Work state projections
- event + command envelope
- capability/ABAC policy schema
- evidence applicability/revocation rules
- execution lease/fencing rules
- idempotency/reconciliation rules
- invariant tests

No full AI platform is required yet.

### M1 — Minimal Vertical Slice

Support only:
- one WorkBuddy Connector
- one Control Plane
- one PostgreSQL
- one Temporal deployment/namespace
- one Session Gateway
- one Ubuntu Worker
- one repository path
- Codex as first runtime
- one CI provider

Mandatory semantics:
- Requirement/Task revision
- Run identity
- frozen Base SHA
- Run Attempt + execution fencing
- immutable Checkpoint/input lineage
- interactive attach through Session Gateway
- steering
- checkpoint/resume
- human takeover
- Artifact + Subject Manifest identity
- basic immutable Evidence + Verification
- Release/Closure Manifest skeleton
- closure back to WorkBuddy

Do not include automatic planner, knowledge ingestion, skill routing, multi-agent orchestration, Claude or HIL farm merely for breadth.

Principle: **cut breadth, not semantics**.

### M2 — Embedded Trusted Loop

Add:
- Device Registry
- leases/fencing
- Device Agent
- flash/test procedures
- raw evidence
- HIL
- Acceptance-to-Evidence
- independent verification

### M3 — Multi Runtime

Add:
- Claude adapter
- provider-neutral runtime contract
- runtime switching
- fallback/comparison

### M4 — Planner / Skill

Only after real Work history exists:
- Capability Registry
- Skill Registry
- Context Resolver
- Task DAG proposals
- runtime routing

### M5 — Engineering Knowledge

Curate proven:
- known issues
- design rules
- compatibility rules
- recovery runbooks
- test methods
- decision patterns

Do not auto-ingest every chat into canonical engineering knowledge.

## 24. Pilot set

Use three representative pilots:

1. ordinary MCU feature: Requirement -> Code -> Build/CI
2. Linux/BSP debug: RCA -> Steering -> Takeover -> Resume
3. Motor/device change: firmware -> flash -> HIL -> Evidence -> Verification

These expose architecture weaknesses much faster than a simple demo.

## 25. Platform acceptance metrics

Core integrity metrics:
- 100% Formal Work has Requirement Revision
- 100% Formal Run has Run ID and Base SHA
- 100% Run restarts are represented by Attempt identity
- 100% formal Artifact traces to producer/source
- 100% required AC traces to valid Evidence
- 100% release Artifact matches verified Artifact or approved equivalence rule
- stale Evidence never satisfies current verification
- 100% Human Steering is auditable
- zero unauthorized capability use
- 100% production releases have Human Authority

AI effectiveness metrics come later:
- cycle time
- first-pass verification
- human correction rate
- retry rate
- runtime switching rate
- rework
- defect escape
- developer waiting time

## 26. Non-negotiable invariants

1. Requirement Revision is immutable.
2. Task Revision is immutable.
3. Every Formal Run has stable identity and explicit input lineage.
4. Only one fenced Run Attempt may own execution at a time.
5. Checkpoint is provider-neutral and immutable/content-addressed.
6. Artifact is immutable and content-addressed.
7. Evidence is immutable and binds an exact Subject Manifest.
8. Evidence applicability is derived; stale/revoked Evidence cannot satisfy Verification.
9. Verification, Review and Decision are separate concepts.
10. Approval binds an exact Subject/Release Manifest digest.
11. External side effects are idempotent or reconciled.
12. Permission is actor/resource/capability based and enforced outside the model.
13. Connector service identity is not Human Authority.
14. Runtime claims are not Verification.
15. Production requires Human Authority.
16. Every Release is reconstructable from immutable manifests and provenance.

## 27. Supply-chain provenance

For release artifacts record:
- builder/workflow identity
- repository and commit
- toolchain/environment image digest
- configuration digest
- dependency lock digest
- content digest
- optional SBOM
- cryptographic build provenance/attestation where supported

Keep the abstraction provider-neutral. GitHub Artifact Attestations are one possible implementation, not the domain model itself.

## 28. Failure model required before M1 exit

Define explicit recovery behavior for:
- Control API restart
- PostgreSQL failover
- Temporal outage/recovery
- Session Gateway restart
- Worker restart
- runtime process crash
- network partition and duplicate reconnect
- stale execution epoch reconnect
- artifact store outage
- Git/CI outage
- WorkBuddy outage
- device agent crash/power loss during flash
- stale device owner recovery
- release timeout with unknown external outcome

For each case define authority after recovery, retry safety, reconciliation, evidence/decision impact and user-visible state.

## 29. Reference implementation notes

Current external capabilities supporting this architecture:
- WorkBuddy Enterprise Connector supports tool filtering and may use administrator-configured unified third-party credentials; the Control Plane must therefore preserve its own actor/authority boundary.
- Codex supports configurable sandbox and approval policies; external platform policy must still be enforced at Tool Gateway / sandbox boundaries.
- Stateful agent/sandbox APIs distinguish harness state, sandbox session state and snapshots; this reinforces the platform rule that provider-native session state is not the sole authority.
- Temporal provides durable workflow execution across crashes/network/infrastructure failure and is used here for orchestration, not as the engineering system of record.
- GitHub Artifact Attestations can establish signed build provenance; use them where plan/repository constraints allow.

References:
- https://cloud.tencent.com/document/product/1831/134453
- https://developers.openai.com/zh-Hans/docs/config-file/config-basic
- https://developers.openai.com/api/docs/guides/agents/sandboxes
- https://developers.openai.com/api/docs/guides/agents-api/environments/security
- https://docs.temporal.io/
- https://docs.github.com/en/actions/concepts/security/artifact-attestations

## 30. One-sentence definition

**WorkBuddy is the R&D front door; the Control Plane owns engineering state; Ubuntu owns execution; coding agents are replaceable interactive runtimes; deliverables are immutable Artifacts; engineering confidence is Evidence-driven; production remains under Human Authority.**
