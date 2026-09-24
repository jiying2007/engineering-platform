# Embedded AI Engineering Platform — Core Architecture v1

Date: 2026-09-24
Status: **CANONICAL CORE ARCHITECTURE**
Scope: R&D center embedded-software engineering
Supersedes as implementation authority:
- `AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md`
- `REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V43.md`

Those documents remain historical design/research evidence. This document is the clean-slate implementation baseline.

---

## 1. Product definition

engineering-platform is:

> **An AI-native embedded-software engineering platform for turning product/software work into controlled Human + AI engineering execution, exact engineering artifacts, reproducible evidence, verification and review.**

It is optimized for:
- Embedded Linux / BSP;
- MCU / RTOS / bare metal;
- drivers and components;
- boot/storage/OTA;
- performance and reliability debugging;
- multi-component embedded products;
- Device/HIL validation.

It is not:
- a PLM;
- an MES;
- an ERP/QMS/CRM;
- a Digital Twin platform;
- a compliance portal;
- a generic agent marketplace;
- a prompt-management product.

Those systems may integrate through adapters and typed references.

---

## 2. Clean-slate rule

engineering-platform is independent.

It has no compatibility obligation to any historical digital-worker repository, schema, directory, Runtime profile or cross-repository ownership model.

Historical systems may provide useful engineering ideas, but:

> **No legacy contract is preserved unless the new platform independently needs it.**

---

## 3. Primary user journey

~~~text
WorkBuddy / CLI / IDE
        |
        v
Engineering WorkItem
        |
        v
Material Readiness
        |
        v
Embedded Capability / Skill Routing
        |
        v
Task Contract + Verification Plan
        |
        v
Run Input Manifest
        |
        v
Interactive Engineering Run
  Engineer + Codex/Claude
        |
        +--> Observe / Steer / Add Context
        +--> Pause / Resume / Checkpoint
        +--> Human Takeover
        |
        v
Git / Build / CI
        |
        v
Artifact + Delivery Receipt
        |
        v
Evidence
        |
        v
Verification
        |
        v
Independent Review when required
        |
        v
Closure / Knowledge Candidate
~~~

This is the product's core value stream.

---

## 4. Architecture layers

### 4.1 Experience

Primary:
- WorkBuddy.

Engineering-native:
- `eng` CLI;
- IDE integration later.

Experience surfaces may:
- create/inspect Work;
- answer clarification;
- approve controlled actions;
- observe Run;
- steer/pause/resume;
- request takeover;
- inspect Evidence/Verification.

They do not execute privileged engineering actions directly.

### 4.2 Embedded Engineering Domain

Owns:
- WorkItem;
- TaskContract;
- MaterialManifest;
- TargetContext;
- capability routing;
- Skill contracts;
- VerificationPlan;
- Debug HypothesisRegistry.

This layer makes the platform embedded-specific rather than a generic agent runner.

### 4.3 Execution Control

Owns:
- Run;
- RunAttempt;
- execution_epoch;
- Session;
- Steering;
- Checkpoint;
- Worker;
- Workspace/Sandbox;
- Action Gateway;
- External Operation Ledger;
- recovery_epoch.

### 4.4 Engineering Facts

Authoritative fact producers:
- Git;
- Build;
- CI;
- Artifact store;
- Device/HIL;
- test tools.

The platform records exact identities and receipts. It does not infer facts from Runtime prose.

### 4.5 Assurance

Owns:
- Evidence;
- VerificationReport;
- ReviewReport;
- Risk/Exception Decision where required;
- ClosureReceipt.

Invariant:

> **Engineering != Verification != Review != irreversible production authority.**

---

## 5. Embedded domain model

Initial capabilities:

~~~text
embedded.architecture
embedded.linux-bsp
embedded.mcu-rtos
embedded.driver-component
embedded.debug-reliability
embedded.verification
~~~

Initial skills:

~~~text
material-readiness
architecture-impact-analysis
interface-contract-review
linux-bsp-debug
mcu-rtos-debug
driver-integration-review
log-triage
verification-plan-builder
~~~

A Skill is:
- a reusable engineering method;
- owned by one capability;
- explicit about inputs;
- explicit about outputs;
- explicit about BLOCK conditions;
- explicit about action ceiling;
- verifiable by Evidence.

A Skill is not:
- an Agent;
- a prompt;
- a tool wrapper;
- a documentation page.

Automatic planning is future work. Explicit skill routing is Core.

---

## 6. Material readiness

No Formal execution may silently invent missing engineering facts.

MaterialManifest tracks at minimum:
- source repository + full commit SHA;
- product/Target identity;
- relevant logs/dumps;
- build/toolchain context;
- Device identity when required;
- required Acceptance Criteria;
- known missing material.

States:

~~~text
READY
BLOCKED
DEGRADED
~~~

`DEGRADED` requires explicit approval/rationale.

Examples:
- Debug can use reproduction OR authoritative original logs according to Task policy.
- Device/HIL work requires exact device/firmware/test identity.
- Unknown base SHA is BLOCKED for Formal execution.

---

## 7. Work and Task

### WorkItem

Stable engineering work identity.

Minimum:
- work_item_id;
- source requirement/ticket reference;
- title/objective;
- human owner;
- target/product scope;
- risk/assurance class;
- state.

The original product requirement may remain in WorkBuddy or another source system. The platform stores an exact source reference/snapshot when needed; it does not need to become a full product-requirements system.

### TaskContract

Immutable task revision.

Minimum:
- task_contract_id;
- work_item_id;
- task type;
- capability/skill route;
- exact base source;
- MaterialManifest;
- allowed actions;
- expected outputs;
- Acceptance Criteria;
- VerificationPlan;
- content digest.

Changing formal scope creates a new TaskContract revision.

---

## 8. Run and interactive execution

### Run

One logical execution of one exact TaskContract.

Binds:
- task_contract_digest;
- RunInputManifest;
- RuntimeProfile;
- Worker/Workspace profile;
- ToolProfile;
- policy profile.

### RunAttempt

Every process/reconnect/restart epoch is explicit.

Fields:
- run_id;
- attempt_id;
- execution_epoch;
- worker_id;
- started/completed;
- disposition.

Invariant:

> **Only the current execution_epoch can mutate authoritative Run state.**

### Interactive Session

Required operations:
- Observe;
- Attach;
- Structured Steering;
- Add Context;
- Ask Human;
- Pause;
- Resume;
- Checkpoint;
- Human Takeover;
- Abort.

A Runtime restart never becomes an invisible continuation.

---

## 9. Session Supervisor

Ubuntu Worker runs a Session Supervisor responsible for:
- Runtime process / PTY;
- Attempt lifecycle;
- epoch enforcement;
- event capture;
- Steering delivery/ack;
- Checkpoint;
- pause/resume;
- attach/reconnect;
- Human Takeover;
- termination.

Runtime provider adapters are thin:
- Codex first;
- Claude later;
- future Runtime without domain change.

---

## 10. Sandbox and privileged actions

Runtime workspace provides:
- repository worktree;
- isolated HOME/temp;
- resource limits;
- network policy;
- local build/test tools.

Reusable high-value credentials are not placed inside the Runtime workspace.

Privileged actions leave the sandbox through Action Gateway:
- Git push;
- PR/merge request;
- CI dispatch;
- artifact publish;
- device reserve/flash;
- signing;
- release/promotion.

The Runtime requests an action.
Policy decides whether to execute it.

---

## 11. External Operation Ledger

Every non-transactional side effect uses:

~~~text
PLANNED
 -> DISPATCHED
 -> CONFIRMED

or

DISPATCHED
 -> UNKNOWN
 -> RECONCILING
 -> CONFIRMED / SAFE_TO_RETRY / MANUAL
~~~

A timeout is not equivalent to failure.

No UNKNOWN irreversible action is blindly retried.

---

## 12. Checkpoint

Checkpoint is provider-neutral and immutable.

Minimum:
- workspace/source-tree snapshot;
- diff digest;
- objective;
- completed/pending work;
- unresolved questions;
- input/context lineage;
- last authoritative event;
- external-operation cursor.

Checkpoint is not a distributed transaction snapshot.

Resume always reconciles external side effects before replay.

---

## 13. Human Takeover

Human Takeover is first-class.

On takeover:
- `control_owner=HUMAN`;
- Runtime write authority is revoked;
- current diff/tree is recorded;
- old interactive grants become invalid;
- formal continuation requires explicit resume/new epoch as policy dictates.

Human changes still require Verification.

---

## 14. Debug Hypothesis Registry

Debug Tasks use a shared HypothesisRegistry.

Each hypothesis records:
- statement;
- Observed facts;
- Inference;
- supporting Evidence;
- contradicting Evidence;
- proposed experiment;
- status.

Statuses:
- PROPOSED;
- SUPPORTED;
- REJECTED;
- CONFIRMED;
- INCONCLUSIVE.

Root cause is never accepted merely because a Runtime generated plausible prose.

---

## 15. Artifact and delivery

Artifact is immutable and content-addressed.

Examples:
- ELF;
- BIN;
- firmware;
- rootfs;
- OTA package;
- report;
- trace;
- model/config;
- raw test data.

Locator != identity.

Every byte-changing transform creates:
- new Artifact;
- Transform/Build Receipt.

### DeliveryReceipt

Engineering output receipt records:
- exact source;
- Task/Run;
- changes;
- produced Artifacts;
- Build/CI facts;
- test/check facts;
- known limitations;
- unresolved items.

DeliveryReceipt is not Verification PASS.

---

## 16. Evidence

Evidence is an immutable statement produced by a trusted source/procedure.

Minimum:
- subject identity;
- issuer;
- Procedure/version;
- environment;
- result;
- raw Artifact refs;
- applicability.

Runtime self-report is an Engineering Claim, not authoritative Evidence.

Subject change may make existing Evidence stale/inapplicable.

---

## 17. Verification

Verification answers:

> Do the required checks and currently applicable Evidence satisfy the Acceptance Criteria for this exact subject?

VerificationReport binds:
- TaskContract;
- exact subject;
- VerificationPlan;
- Evidence;
- result;
- verifier;
- limitations.

Verification never relies on "Agent said tests passed."

---

## 18. Review

Independent Review is risk/policy driven.

Suggested independence:

~~~text
R0 same-run self-check
R1 new session, same Runtime family
R2 independent Run/context
R3 different qualified Runtime/reviewer system
R4 human domain specialist
~~~

High-risk Tasks may require R4.

No-comments from an AI reviewer is not automatically PASS.

---

## 19. Context

Core context sources:
- repository;
- WorkBuddy requirement/ticket snapshot;
- product/Target docs;
- datasheet/TRM/SDK docs;
- logs;
- prior verified Evidence/known issues;
- user-provided files.

Every Formal context item retains:
- source;
- version/digest;
- provenance;
- trust class;
- freshness where material.

Core provides Context adapters/search/read.

A full Knowledge platform is not required for M1.

---

## 20. Persistence and infrastructure

M1 defaults:

- Go Control API/CLI/Worker;
- PostgreSQL for mutable authoritative state;
- Temporal for durable orchestration/waits/retries;
- S3/MinIO-compatible Artifact store;
- OPA for authorization/policy evaluation;
- OpenTelemetry for observability;
- Toxiproxy for deterministic failure tests;
- Git provider/CI;
- native Ubuntu Worker;
- Codex Runtime adapter.

Do not add another service until a real milestone requires it.

---

## 21. Recovery

Authoritative audit uses append-only events plus chained/signed checkpoints.

Backup/restore creates a new `recovery_epoch`.

After restore, platform enters:

~~~text
RECOVERY_RECONCILIATION
~~~

It blocks irreversible actions until actual Git/CI/Artifact/Device/external state is reconciled.

Rule:

> **Restore records first; reconcile the world second.**

---

## 22. Explicit non-goals for Core

The following are extension domains, not Core M0/M1:
- PLM/PDM;
- MES/ERP/QMS/CRM;
- manufacturing/fleet management;
- PSIRT/CRA workflow;
- DPP/GS1/EPCIS;
- Digital Twin platform;
- license/compliance automation;
- RMA service platform;
- supplier CAPA;
- certification systems;
- privacy platform;
- predictive maintenance;
- SysML/MBSE platform;
- full safety-management platform;
- LIMS;
- generic Skill marketplace;
- autonomous Planner.

Research remains useful, but these capabilities activate only when real product milestones require them.

---

## 23. Maturity rule

Do not claim a Skill/Capability/Runtime/process mature because:
- file exists;
- schema exists;
- synthetic fixture passes;
- CI is green.

Maturity requires retained real engineering Evidence.

Suggested progression:

~~~text
DEFINED
 -> EVALUATED
 -> PILOTED
 -> PROVEN
~~~

Every promotion cites real task/run evidence.

---

## 24. Core invariants

1. Exact source identity for Formal execution.
2. Missing material fails closed.
3. Runtime is replaceable and is never domain authority.
4. Skill is an engineering method, not a prompt.
5. Run restart creates a new Attempt/epoch.
6. Only current epoch can mutate Run.
7. Steering/Takeover are structured authoritative events.
8. Privileged actions leave the Runtime sandbox.
9. External side effects are idempotent/reconciled.
10. Artifact is content-addressed.
11. Runtime claims are not Verification.
12. Engineering != Verification != Review.
13. Debug root cause is hypothesis/evidence driven.
14. Human Takeover never bypasses Verification.
15. Restore never assumes external systems rolled back.
16. Extension domains cannot silently expand Core M0.

---

## 25. Product success criterion

The platform is successful when a real embedded engineer can take a real WorkBuddy requirement or bug and complete:

~~~text
material-ready
 -> expert/skill route
 -> interactive AI+human engineering
 -> build/CI
 -> exact delivery artifacts
 -> evidence
 -> verification
 -> review where required
 -> closure
~~~

with materially lower cycle time and equal or better engineering quality.

That end-to-end result is more important than the number of supported standards, schemas, integrations or agents.
