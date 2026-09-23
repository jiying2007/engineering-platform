# AI Native Engineering Platform v1

Status: **Architecture Baseline**
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

The platform has nine first-class objects:

1. Requirement Revision
2. Work
3. Task Revision
4. Run
5. Run Attempt
6. Artifact
7. Evidence
8. Decision
9. Release

Relationship:

```text
Requirement Revision
        |
        v
       Work
        |
        v
 Task Revision DAG
        |
        v
       Run
        |
        +---- Run Attempt
        |
        +---- Artifact
        |
        +---- Evidence
                |
                v
             Decision
                |
                v
             Release
```

### 4.1 Requirement Revision

A formal requirement is immutable once accepted for formal development.

Conversation and source documents are context, not authority.

Requirement changes create a new revision and trigger impact analysis.

Minimum fields:
- requirement_id
- revision
- title / goal
- scope / non_goals
- constraints
- acceptance criteria
- source snapshot reference/digest

### 4.2 Work

A Requirement Revision may create one or more engineering Works.

Work is the primary delivery object visible to engineering management.

### 4.3 Task Revision

A Task is a concrete engineering contract. Changes to repository, modules, technical approach, verification or outputs create a new Task Revision even when the product requirement itself is unchanged.

Minimum fields:
- task_id / revision
- work_id
- type
- repository / frozen base commit
- inputs
- required outputs
- required verification
- policy profile

### 4.4 Run

A Run is one logical engineering execution with fixed:
- Task Revision
- runtime identity/provider
- base commit
- policy profile

A Run is not a process PID and is not equivalent to a provider-native CLI session.

### 4.5 Run Attempt

A Run Attempt represents one concrete execution epoch.

Worker restart, process crash or network loss may terminate ATTEMPT-01 and resume the same logical Run as ATTEMPT-02.

Changing runtime execution identity, for example Codex to Claude, creates a new Run rather than pretending to continue the same one.

### 4.6 Artifact

Artifact is a first-class immutable deliverable.

Examples:
- firmware/bin/elf
- OTA package
- executable/container
- model
- test package/report
- SBOM

Artifact identity must bind:
- producer Run/Attempt
- source repository and commit
- build/toolchain/configuration digests
- content digest

The artifact released must be the artifact that was actually verified, unless an explicit promotion/rebuild policy proves equivalence.

### 4.7 Evidence

Evidence is a verifiable engineering fact tied to an exact subject.

Lifecycle:
- VALID
- STALE
- SUPERSEDED
- REVOKED
- INVALID

PASS/FAIL is the test result, not evidence validity.

Any material change to the verified subject invalidates affected evidence.

### 4.8 Decision

A Decision records:
- actor / authority
- inputs and evidence
- chosen action
- rationale
- risk acceptance when applicable

### 4.9 Release

A Release binds:
- Requirement Revision(s)
- source commit(s)
- exact Artifact(s)
- required Evidence
- verification result
- review decision
- rollback information
- Human Authority

## 5. Acceptance to Evidence

Acceptance is mapped to evidence from the moment a requirement becomes formal.

Example:

| AC | Verification | Evidence | Status |
|---|---|---|---|
| AC-01 | Device | EV-101 | PASS / VALID |
| AC-02 | HIL | EV-108 | PASS / VALID |
| AC-03 | Thermal | - | MISSING |

Verification passes only when every required acceptance criterion has acceptable, currently VALID evidence bound to the current subject.

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

## 17. State machines

### Run

```text
CREATED
 -> STARTING
 -> RUNNING
      |- WAITING_HUMAN
      |- WAITING_PERMISSION
      |- WAITING_RESOURCE
      |- PAUSED
      |- HUMAN_TAKEOVER
 -> COMPLETING
 -> COMPLETED
```

Terminal/exception states:
- FAILED
- ABORTED
- LOST
- TIMED_OUT
- REJECTED
- SUPERSEDED

### Work

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

Side states:
- BLOCKED
- CANCELLED
- SUPERSEDED

## 18. Event contract

Formal operations emit append-only events.

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

## 19. Workflow implementation

Domain contracts must not depend on one workflow engine.

M1 may use:
- PostgreSQL durable state
- transactional outbox
- idempotency
- worker lease/heartbeat

Temporal can be introduced when long-lived workflows, multi-worker recovery, human waits, shared devices and complex retries justify it.

The domain model must be designed for durable execution from day one even if Temporal is not an M1 prerequisite.

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
│   └── worker/
├── internal/
│   ├── identity/
│   ├── requirement/
│   ├── work/
│   ├── task/
│   ├── run/
│   ├── artifact/
│   ├── evidence/
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
├── deploy/
└── tests/
```

Do not split into many microservices before the domain model stabilizes.

## 23. Delivery roadmap

### M0 — Domain Contract

Freeze:
- nine core objects
- ID conventions
- state machines
- event envelope
- capability policy schema
- evidence invalidation rules
- invariant tests

No full AI platform is required yet.

### M1 — Minimal Vertical Slice

Support only:
- one WorkBuddy Connector
- one Control Plane
- one PostgreSQL
- one Ubuntu Worker
- one repository path
- Codex as first runtime
- one CI provider

Mandatory semantics:
- Requirement/Task revision
- Run identity
- frozen Base SHA
- Run Attempt
- interactive attach
- steering
- checkpoint/resume
- human takeover
- Artifact identity
- basic Evidence
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

1. Requirement is versioned.
2. Task Contract is versioned.
3. Every Formal Run has identity.
4. Every Runtime restart has Attempt identity.
5. Every Formal Run has a frozen Base SHA.
6. Artifact is immutable and content-addressed.
7. Evidence binds an exact Artifact/Subject.
8. Subject change invalidates affected Evidence.
9. Runtime claims are not Verification.
10. Permission is capability-based.
11. Production requires Human Authority.
12. Every Release traces to Requirement, Source, Artifact, Evidence, Review and Decision.

## 27. One-sentence definition

**WorkBuddy is the R&D front door; the Control Plane owns engineering state; Ubuntu owns execution; coding agents are replaceable interactive runtimes; deliverables are immutable Artifacts; engineering confidence is Evidence-driven; production remains under Human Authority.**
