> **Historical architecture foundation.** The current clean-slate implementation authority is [Embedded AI Engineering Platform — Core Architecture v1](EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md). v1.2 remains design evidence and is no longer the M0 implementation baseline.

# AI Native Engineering Platform v1.2 — Final Architecture Baseline

Status: **FINAL ARCHITECTURE BASELINE / M0 INPUT**
Date: 2026-09-23
Repository: `jiying2007/engineering-platform`

> This document supersedes v1.0/v1.1 as the canonical target architecture.
> Round 2–9 reviews remain historical design evidence; unresolved items are now implementation contracts for M0, not open top-level architecture questions.

---

# 1. 定位

本系统正式定义为：

> **AI Native Engineering Control Plane**

它不是：

- WorkBuddy 调 Codex/Claude 的代理层；
- Prompt 管理平台；
- 多 Agent Demo；
- Coding Portal；
- 聊天记录知识库；
- 单纯的 CI/CD 封装。

它负责把：

```text
产品需求
  -> 正式工程合同
  -> Human + AI 研发执行
  -> Git / Build / CI
  -> Immutable Artifact
  -> Device / HIL
  -> Evidence
  -> Verification
  -> Independent Review
  -> Human Authority
  -> Release / Closure
```

变成一条：

> **可追踪、可恢复、可交互、可审计、可验证、可撤销、可重放验证、Runtime Provider Neutral 的正式研发链。**

---

# 2. 最终五层架构

只保留五个 Plane，不再增加 Agent/Skill/Knowledge/Security Plane。

```text
┌──────────────────────────────────────────────────────┐
│                  Experience Plane                    │
│                                                      │
│ Windows / WorkBuddy                                  │
│ Requirement / Query / Clarification / Approval       │
│ Risk Acceptance / Delivery / Release Status          │
└───────────────────────┬──────────────────────────────┘
                        │ Enterprise Connector / MCP
                        ▼
┌──────────────────────────────────────────────────────┐
│                   Control Plane                      │
│                                                      │
│ Identity / Authority / Delegation                    │
│ Requirement / Work / Task                            │
│ Target / Interface / Compatibility                   │
│ Risk / Assurance / Verification Plan                 │
│ Policy / Workflow / Scheduler                        │
│ Run / Attempt / Checkpoint                           │
│ Artifact / Evidence / Verification                   │
│ Review / Decision / Release / Closure                │
└───────────────────────┬──────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────┐
│                  Execution Plane                     │
│                                                      │
│ Ubuntu Engineering Worker                            │
│ Execution Sandbox / Workspace                        │
│ Session Supervisor / Runtime Gateway                 │
│ Codex / Claude / Future Runtime                      │
│ Local Tools                                          │
└───────────────┬──────────────────┬───────────────────┘
                │                  │
                ▼                  ▼
       Platform Action        Device / HIL
          Gateway                 Agent
                │                  │
                └─────────┬────────┘
                          ▼
┌──────────────────────────────────────────────────────┐
│                   Evidence Plane                     │
│                                                      │
│ Immutable Artifacts / Manifests                      │
│ Raw Evidence / Evaluation Evidence                   │
│ Build Provenance / Attestation                       │
│ Audit Journal / Signed Checkpoints                   │
└───────────────────────┬──────────────────────────────┘
                        ▼
┌──────────────────────────────────────────────────────┐
│                  Assurance Plane                     │
│                                                      │
│ Verification / Independent Review                    │
│ Risk Decision / Policy Exception                     │
│ Human Authority / Release Authorization              │
│ Promotion Reconciliation / Closure                   │
└──────────────────────────────────────────────────────┘
```

---

# 3. 权威边界

必须固定以下 Authority：

| Concern | Authority |
|---|---|
| 人机协作入口 | WorkBuddy |
| 正式研发状态 | Engineering Control Plane |
| 工程执行 | Ubuntu Worker |
| Runtime 推理与编码 | Codex / Claude / Future Runtime |
| Git 集成事实 | Git Provider + Integration Subject |
| 构建/设备事实 | CI / Trusted Builder / Device Agent / HIL |
| 交付物身份 | Artifact Registry + content digest |
| 工程证明 | Evidence + Verification |
| 安全/风险例外 | Typed Decision |
| 生产决策 | Human Authority |
| 编排 | Temporal |
| Mutable business truth | PostgreSQL |

核心规则：

> **Runtime output is a proposal, never authority by itself.**

> **Conversation is Context; Requirement Revision is Authority.**

> **Object-store path / URL is a locator; digest is content identity.**

---

# 4. Experience Plane：WorkBuddy

WorkBuddy 定位：

> **Corporate R&D Front Door**

负责：

- submit / revise requirement；
- 查看 Work/Task/Run 高层状态；
- 回答 Clarification；
- Plan approval；
- Risk Acceptance / Waiver；
- Release request / approval；
- Delivery result。

WorkBuddy 不直接暴露：

- SSH；
- shell；
- Git write；
- unrestricted Codex prompt；
- Device flash；
- production credential；
- signing key。

推荐企业只暴露一个研发 Connector：

```text
enterprise_engineering
```

高层工具示例：

```text
submit_requirement
revise_requirement
get_requirement
get_work_status
get_blockers
answer_clarification
approve_plan
accept_risk
get_verification_status
request_release
approve_release
get_release_status
get_delivery_result
```

Connector 的 service credential **不等于最终 Human identity**。

高风险动作必须有 Control Plane 可验证的 actor assertion / delegation；无法可信透传最终用户身份时，必须进入独立认证的 Control Plane approval。

---

# 5. 领域模型

不使用“固定几个对象”的营销式表达。

## 5.1 Stable Aggregate Identities

正式稳定身份：

- Requirement
- Work
- Task
- Target
- Interface Contract
- Run
- Artifact
- Baseline
- Verification
- Review
- Risk
- Decision
- Release

## 5.2 Immutable Revisions / Records

不可变版本或记录：

- Requirement Revision
- Task Revision
- Target Revision
- Interface Contract Revision
- Run Attempt
- Checkpoint
- Run Input Manifest
- Run Receipt
- Integration Subject
- Subject Manifest
- Verification Plan
- Evidence
- Transform Receipt
- Release Manifest
- Closure Manifest
- Approval / Waiver / Risk Acceptance record

关系概览：

```text
Requirement
 └─ Requirement Revision
        │
        ▼
       Work
        │
        ├─ Target Revision
        ├─ Assurance Profile
        ├─ Verification Plan
        │
        ▼
Task ─ Task Revision DAG
        │
        ▼
       Run
    ├─ Run Input Manifest
    ├─ Run Attempt
    ├─ Checkpoint
    └─ Run Receipt
        │
        ▼
Integration Subject
        │
        ▼
Artifact Derivation Graph
        │
        ▼
Subject Manifest
    ├─ Raw Evidence
    ├─ Evaluation Evidence
    └─ Verification
          │
          ▼
        Review
          │
          ▼
        Decision
          │
          ▼
    Release Manifest
          │
          ▼
        Release
          │
          ▼
    Closure Manifest
```

---

# 6. Requirement Contract 与 READY Gate

Requirement 是产品与研发的正式合同。

Requirement Revision 一旦进入 Formal Development 即不可原地修改：

```text
REQ r3 -> REQ r4
```

READY 不是“JSON 字段填完”。

Requirement Revision 进入 READY 至少要求：

- Product owner 明确；
- goal / scope / non-goals 明确；
- Target/Variant 已知或明确 N/A；
- Acceptance Criteria 可验证；
- constraints 无冲突；
- risk classification 完成；
- confidentiality/data classification 完成；
- dependencies 已声明；
- blocking clarification 已关闭；
- Assurance Profile 可确定；
- Verification Plan 可形成。

READY 本身是绑定 exact Requirement Revision digest 的 Decision。

---

# 7. Acceptance Criterion

AC 不能只是自由文本。

最小结构：

```yaml
ac_id: AC-01
statement: 连续100次回充不得明显振荡
verification_mode: device_statistical
metric: docking_success_rate
target_scope:
  target_revision: ...
sample_requirement:
  count: 100
baseline_ref: BASE-...
authority:
  owner: product
```

允许的 verification mode 包括：

- automated_test
- measurement
- statistical_evaluation
- independent_human_review
- security_review
- compliance/manual_inspection

如果无法机器量化，也必须明确“由什么方式、什么 authority 证明”。

---

# 8. Risk 与 Assurance Profile

风险不是 release note 中的一句话。

Risk 状态至少包括：

```text
IDENTIFIED
ANALYZED
MITIGATED
ACCEPTED
TRANSFERRED
CLOSED
EXPIRED_ACCEPTANCE
```

Known Risk != Accepted Risk。

根据风险属性选择 versioned Assurance Profile，例如：

```text
A0  low-risk internal/dev
A1  normal product code
A2  device/firmware operational risk
A3  boot/OTA/security/safety critical
A4  irreversible/production critical
```

Assurance Profile 决定最低：

- Verification class；
- Evidence issuer trust；
- reviewer independence；
- quorum/approval；
- rollback/migration proof；
- provenance；
- retention；
- break-glass eligibility。

Profile label 只是可读名称，真正 authority 来自 policy bundle digest。

---

# 9. Verification Plan

在实现完成之前冻结 Verification Plan。

它映射：

```text
AC
 -> Target Revision
 -> Assurance Profile
 -> Procedure / Baseline
 -> Expected Evidence
 -> Trusted Issuer class
 -> Sample / repetition
 -> Independent Review
 -> Final-stage release checks
```

AI/Planner 可以 propose，但不能自批准。

若 Plan materially changes，产生新的 Plan digest；原 approve_plan 不自动继承。

---

# 10. Task / Task Revision / Replan

Task 是稳定身份，Task Revision 是不可变工程合同。

Task Revision 至少绑定：

- Work；
- type；
- source input；
- expected outputs；
- Task base commit(s)；
- Target/Interface Contract；
- policy profile；
- Verification Plan requirements。

Task Replan：

```text
pause
 -> TASK rN+1
 -> impact analysis
 -> old Run disposition
 -> new Run
```

旧 Run 必须明确：

- ABORT
- SUPERSEDE
- KEEP_FOR_EVIDENCE_ONLY
- CONTINUE_DIAGNOSTIC
- NO_IMPACT_CONTINUE

不能创建 Task r2 后让 r1 Run 继续拥有原有 production-capable privileges。

---

# 11. Run / Attempt / Input Lineage

Run 表示一次具有固定 Runtime identity 与 Task Revision 的逻辑工程执行。

Run 不是 PID，也不是 provider CLI session。

Run 必须绑定：

- Task Revision；
- runtime/provider profile；
- task_base_commit；
- Run Input Manifest digest；
- policy bundle；
- capability grant；
- optional parent_run；
- optional resume_from_checkpoint。

## Run Attempt

进程/Worker 重启属于同一 Run 的新 Attempt：

```text
RUN-812
 ├─ ATT-01
 └─ ATT-02
```

Runtime identity 切换：

```text
RUN-812 Codex
RUN-813 Claude
```

必须新 Run。

## Execution Fencing

只有一个 Attempt 可以拥有当前 execution epoch。

所有 mutating callback 带：

```text
run_id
attempt_id
execution_epoch
```

旧 epoch 一律拒绝。

---

# 12. Run Input Manifest

Formal Run 启动前生成不可变 Run Input Manifest：

- Requirement Revision；
- Task Revision；
- Target Revision；
- source repo/commit/tree；
- multi-repo dependencies；
- selected context + trust class；
- runtime profile；
- provider eligibility；
- policy bundle digest；
- capability grant；
- environment/toolchain profile；
- parent Run / Checkpoint lineage。

Run Input Manifest 回答：

> **Runtime 开始时到底拿到了什么正式输入？**

---

# 13. Interactive Session

正式 Runtime 是 Interactive Run Session，不是 batch job。

必须支持：

- Execute
- Observe
- Attach
- Structured Steering
- Add Context
- Ask Human
- Request Permission
- Pause / Resume
- Checkpoint
- Human Takeover
- Abort
- Runtime Switch via new Run

## Session Supervisor

Ubuntu Worker 核心是 Session Supervisor：

- process supervision；
- PTY/runtime channel；
- Attempt management；
- event capture；
- steering；
- checkpoint；
- pause/resume；
- takeover；
- reconnect/recovery。

Runtime Adapter 只做 provider-specific translation。

## Session Gateway

Control API 和 streaming data plane 分离：

```text
eng CLI
  ├─ HTTPS -> Control API
  └─ WebSocket -> Session Gateway
                    -> Worker outbound stream
                    -> Session Supervisor
```

Attach token 绑定：

- actor；
- run_id；
- execution_epoch；
- audience；
- view/interact scope；
- expiry；
- nonce。

epoch/takeover/abort 变化后旧 write token 失效。

---

# 14. Structured Steering 与命令顺序

正式 Steering 不是 PTY 文本日志。

最小结构：

```text
steering_id
run_id
execution_epoch
actor
sequence
content_digest
delivery_state
acknowledged_at
```

状态：

```text
ACCEPTED -> DELIVERED -> ACKNOWLEDGED
        -> EXPIRED / REJECTED / STALE_EPOCH
```

命令 precedence 必须由 state/guard table 固定，例如：

- ABORT dominates future steering；
- HUMAN_TAKEOVER 后 Runtime 无权写 workspace；
- PAUSE 阻止新 privileged dispatch；
- stale aggregate version / epoch reject；
- release-takeover 需要 checkpoint/result snapshot。

---

# 15. Human Takeover

```bash
eng run takeover RUN-812
```

Takeover：

- 改 control_owner；
- 记录 actor；
- 记录 tree/diff digest；
- revoke Runtime write authority；
- 重新计算 Evidence applicability；
- 必要时 invalidate old Subject。

结束后：

```bash
eng run checkpoint RUN-812
eng run release-takeover RUN-812
eng run resume RUN-812
```

人工修改不会绕过 Verification。

---

# 16. Checkpoint

Checkpoint 是 provider-neutral authority。

包含：

- workspace tree/diff/snapshot digest；
- provider native session ref（仅 convenience）；
- current objective；
- completed/pending work；
- pending question/approval；
- input lineage；
- last event sequence；
- external-operation ledger cursor。

关键规则：

> **Checkpoint 不是分布式事务快照。**

恢复必须：

```text
restore local state
 -> reconcile external operation ledger
 -> resolve UNKNOWN effects
 -> continue
```

不能重复已发生的 PR/CI/flash/release 等副作用。

---

# 17. Run Receipt

Run 完成生成 immutable Run Receipt：

- Run/Attempt lineage；
- Run Input Manifest；
- final tree/commit；
- produced Artifact；
- Evidence；
- steering/takeover references；
- policy/approval decisions；
- external side-effect summary；
- runtime/provider identity/version；
- terminal result；
- usage/cost metadata。

Run Receipt 是执行 provenance，不是 Verification。

---

# 18. Workspace 与 Sandbox

Git worktree：

> Source Workspace Isolation

不是完整安全边界。

Formal Run 还需要：

- isolated worktree；
- temporary HOME；
- rootless container / Linux namespace；
- cgroup；
- process isolation；
- controlled mount；
- network egress policy；
- ephemeral credential only when unavoidable。

Repository/build/test scripts 均按 hostile code 对待。

---

# 19. Local Tools 与 Platform Action Gateway

必须分成两类。

## Local execution

在 sandbox 内：

- compiler；
- grep/search；
- unit test；
- local script；
- source read/write。

## Privileged Platform Actions

在 sandbox 外由平台执行：

- Git push；
- PR create/merge；
- CI dispatch；
- Artifact publish；
- Device reserve/flash；
- signing；
- production promotion；
- secret-backed API。

Runtime 请求 action，不获取底层 reusable credential。

---

# 20. Identity / Capability / Policy

Connector authenticated != User authorized。

身份链：

```text
WorkBuddy User
 -> Enterprise Identity
 -> Engineering Actor
 -> Role / Authority / Delegation
 -> Resource / Environment
 -> Capability Evaluation
```

权限采用 capability/resource/environment 模型，例如：

```text
filesystem.workspace_write
git.feature_branch_push
git.merge
device.reserve
device.dev_flash
device.production_flash
release.propose
release.production_approve
artifact.sign
```

Policy inheritance：

- explicit DENY dominates ALLOW；
- child scope 可收紧，不能静默放宽 parent；
- 放宽必须 typed authorized exception；
- production non-overridable policy 只能走 named emergency policy；
- decision 记录 effective policy bundle digest。

Platform administrator != Release Authority。

---

# 21. Delegation / Approval / Waiver

Human authority 可正式 delegation，但必须绑定：

- delegator；
- delegate；
- scope；
- project/environment；
- valid window；
- max risk/action class；
- re-delegation rule；
- revocation。

不能通过 delegation 绕过 requester != approver 等 separation-of-duties。

Risk Acceptance / Waiver / Policy Exception 必须是 typed Decision，绑定：

- subject/release digest；
- rule/risk ID；
- environment；
- authority；
- expiry；
- max uses；
- compensating controls；
- follow-up Work。

---

# 22. Common Schema Envelope

M0 所有 authority-bearing schema 使用统一 profile。

典型字段：

```yaml
schema_name:
schema_version:
canonicalization_version:

id:
organization_id:
project_id:
environment:

created_at:
created_by:

aggregate_version:

content_digest:
  algorithm:
  value:

correlation_id:
causation_id:

labels: {}
extensions: {}
```

必须冻结：

- null vs absent；
- timestamp precision；
- enum casing/evolution；
- ID case sensitivity；
- extension namespace；
- size limit；
- hash-included fields；
- metadata-only fields。

---

# 23. Canonical Serialization

所有 content-addressed structured object 必须：

```text
schema-valid object
 -> canonical representation
 -> canonical serialization
 -> digest
```

JSON 采用明确的 canonicalization profile（推荐 RFC 8785/JCS 或等价严格 profile）。

必须记录：

- schema_version；
- canonicalization_version；
- digest algorithm；
- digest format。

不能直接 hash pretty JSON/API response。

Business ID 和 content digest 分离。

---

# 24. Typed Reference

禁止 authority-bearing schema 中大量使用模糊：

```text
ref: xxx
```

采用 typed ref：

```yaml
kind: artifact
id: ART-812
content_digest: sha256:...
```

Git：

```yaml
provider: git
repository_id: ...
commit: ...
tree_digest: ...
```

mutable URL 只能做 locator，正式 authority 必须绑定 immutable revision 或 snapshot Artifact。

---

# 25. Target / Product Configuration

嵌入式产品必须有 Target / Target Revision。

示例：

```yaml
target_id: PCR02
revision: 7

variant:
  sku: PCR02-CN
  hardware_revision: V3

components:
  linux:
    soc: SSC305
  main_mcu:
    part: GD32L235
  motor_mcu:
    part: MM32SPIN023C
  charger_mcu:
    part: HC32F072

interfaces:
  motor_protocol: v4
  dock_protocol: v2

boot_constraints:
  bootloader_min: ...
  partition_layout: ...
```

Verification/Release 绑定 exact Target Revision digest。

---

# 26. Interface / Compatibility Contracts

正式管理：

- MCU protocol；
- IPC/RPC；
- OTA metadata；
- partition layout；
- persistent storage schema；
- calibration/config schema；
- bootloader handoff。

兼容约束机器可读：

```text
motor-fw 0.6.x
requires
main-mcu protocol >= 4
```

software version、Artifact digest、compatibility identity 三者分开。

Breaking interface change 自动扩大 impact/Verification scope。

---

# 27. Integration Subject

PR head PASS 不等于 main PASS。

Integration Subject 固定：

- target repo；
- target base commit；
- change/PR head；
- integration candidate commit/tree；
- merge strategy；
- dependent PR/repos；
- Target/config context。

```text
feature F + main M -> integration candidate I
```

Release-quality Verification 绑定 I。

Base branch 漂移后旧 Integration Evidence 不自动继续有效。

merge/squash/rebase 最终 provider-created commit/tree 必须 reconciliation。

---

# 28. Artifact

Artifact 是 immutable/content-addressed deliverable：

- ELF/BIN；
- firmware；
- OTA；
- container；
- model；
- report；
- raw trace；
- SBOM；
- source snapshot；
- manifests。

Artifact fetch 在 build/verify/flash/release 时都重新验证 digest。

Object Store path 不是 identity。

Two-phase finalize：

```text
request upload
 -> scoped upload
 -> verify bytes/digest
 -> finalize Artifact record
```

---

# 29. Build Provenance 与 Dependency

Release-quality Artifact 记录：

- builder/workflow identity；
- source commit/tree；
- toolchain/environment image digest；
- build config digest；
- dependency lock/resolved dependency digest；
- registry/mirror identity；
- output digest；
- optional SBOM；
- provenance/attestation。

Reproducibility classification：

- REPRODUCIBLE
- CONTROLLED
- RECORDED_LEGACY
- UNKNOWN

Trusted provenance/signing secret 不得进入 user-controlled build step。

---

# 30. Artifact Derivation Graph

真实发布链：

```text
source
 -> build
unsigned BIN
 -> sign
signed BIN
 -> package
OTA
 -> encrypt/compress
final distribution Artifact
```

每个 transform 产生新 Artifact，并记录 Transform Receipt：

- parent digest(s)；
- transform type；
- implementation/version digest；
- config；
- trusted executor；
- output digest；
- attestation/receipt。

Signing 是 privileged transform，不是 metadata flag。

---

# 31. Evidence

Evidence 是 immutable historical statement。

必须分离：

## Result

```text
PASS
FAIL
INCONCLUSIVE
...
```

## Applicability

```text
APPLICABLE
STALE
SUPERSEDED
REVOKED
INVALID
```

Applicability 取决于：

- Subject match；
- issuer trust；
- procedure validity；
- fixture/calibration；
- policy freshness；
- quarantine/revocation；
- validity window。

即使 Subject bytes 未变，issuer compromise / calibration expiry 仍可要求 re-verification。

---

# 32. Evidence Issuer / Trust Root

可信 Evidence 必须有 issuer envelope：

- issuer_id；
- issuer_type；
- trust_domain；
- key/cert identity；
- valid window；
- allowed Evidence class；
- project/environment scope；
- software/verifier identity；
- status ACTIVE/SUSPENDED/REVOKED。

Runtime self-report 只能成为 Engineering Claim。

Trusted CI / Device Agent / Verifier 才能按 policy 产生 authority-bearing Evidence。

Key compromise 必须触发 dependency impact：

```text
Issuer/Key
 -> Evidence
 -> Verification
 -> Review
 -> Release
```

历史记录不删除。

---

# 33. Raw Evidence 与 Evaluation

Raw truth 与评价分离：

```text
Raw Evidence Artifact
 -> Procedure Revision
 -> Evaluation Result Evidence
```

例如 current/temp trace 保持不变；阈值修订后可以重新 evaluate。

非确定性测试 Procedure 必须定义：

- sample count；
- repetition；
- warm-up；
- environment；
- random seed（适用时）；
- confidence rule；
- outlier rule；
- stop rule。

---

# 34. Subject Manifest

Verification/Review/Decision 绑定 exact Subject Manifest：

- Requirement Revision；
- Task Revision；
- Target Revision；
- Integration Subject；
- source tree；
- Artifact digest；
- procedure；
- baseline；
- fixture；
- environment/toolchain；
- policy bundle。

```text
subject_digest
```

是 Verification authority 的核心绑定点。

---

# 35. Baseline

“不得低于当前 baseline”必须绑定 immutable Baseline：

- Target；
- Procedure；
- Raw Evidence；
- metrics；
- statistical definition；
- selection/approval Decision；
- digest。

Baseline 更新不改历史，生成新 baseline identity。

---

# 36. Verification

Verification 回答：

> **针对这个 exact Subject，当前可用 Evidence 是否满足规定的 AC / policy？**

它不是 Runtime 自述。

记录：

- verification_id；
- subject_digest；
- Evidence set；
- Verification Plan；
- Procedure/Baseline；
- executor identity；
- policy；
- result；
- timestamp。

Verification 可重新计算，不修改原 Evidence。

---

# 37. Independent Review

Review != Verification。

Verification：

> declared checks 是否满足。

Review：

> 独立 reviewer 是否接受冻结 engineering subject、diff、风险与证据。

Independence level：

- R0 same-run self-check
- R1 independent session
- R2 independent Run/context
- R3 different runtime/provider
- R4 human specialist

policy 按 Assurance Profile 选择最低等级。

Reviewer 不修改 reviewed subject。

---

# 38. Release Bundle

嵌入式 Release 是 component set：

```yaml
bundle:
  target_revision_digest: ...
  components:
    - role: linux
      artifact_digest: ...
    - role: main_mcu
      artifact_digest: ...
    - role: motor_mcu
      artifact_digest: ...
    - role: charger_mcu
      artifact_digest: ...
```

批准绑定整个 Bundle/Release Manifest digest。

单组件 PASS 不证明组合 PASS。

---

# 39. Final-byte Verification

签名/打包/加密会改变 bytes。

Evidence reuse across transforms 必须 policy-driven。

某些 source/static Evidence 可继承；但以下通常必须针对 final Artifact：

- signature verification；
- package manifest；
- install/unpack；
- boot/smoke；
- compatibility；
- final bundle digest。

原则：

> **Freeze exact integration subject; build immutable artifacts; apply only trusted recorded transforms; verify transform-sensitive properties against final bytes; promote and reconcile exact Release Bundle.**

---

# 40. Release Manifest

Release Manifest 至少绑定：

- organization/project/environment；
- Target Revision；
- Integration Subject；
- final Release Bundle；
- Subject Manifest；
- required Verification；
- required Review；
- Risk/Exception Decisions；
- rollback target；
- migration/update Procedure；
- promotion target/channel。

Human approval 绑定 exact manifest digest。

Manifest 变更 -> old approval 不适用。

---

# 41. OTA / Migration / Rollback

Release 必须说明：

```text
Configuration A
 -> migration/update
 -> Configuration B
```

定义：

- supported source configuration；
- precondition；
- component update order；
- power-loss behavior；
- safe retry；
- rollback target；
- persistent-data compatibility；
- recovery/quarantine path。

多组件 update 必须有明确状态机，不能 Agent 临场决定。

Rollback Artifact/Release 必须在 production authorization 之前就明确。

---

# 42. Device / HIL

正式子系统：

- Device Registry；
- Lease Manager；
- Device Agent；
- Fixture Registry；
- Procedure Registry。

Device lease：

- lease_id；
- owner Run；
- expiry；
- fencing token。

旧 token 一律失败。

权威 Verification 前必须检测实际状态：

- device/board identity；
- current component versions/digests；
- bootloader；
- target revision；
- fixture/calibration；
- test software；
- loaded Bundle。

Mismatch -> fail closed，不产生 authoritative PASS。

Flash timeout -> UNKNOWN/quarantine/reconcile，禁止 blind retry。

---

# 43. Clarification

Runtime 问题必须结构化：

- question_id；
- exact context digest；
- required authority；
- answering actor；
- answer；
- scope；
- resulting classification。

最终分类：

```text
Run Steering
Task Revision
Requirement Revision
```

WorkBuddy free-form answer 不能直接改变 Acceptance。

---

# 44. Supersession / Cancellation

Requirement/Task Revision superseded 后必须做 Impact Decision。

Active Run 可被：

- UNAFFECTED；
- PENDING_IMPACT_REVIEW；
- SUPERSEDED；
- CONTINUE_DIAGNOSTIC。

Cancellation：

- revoke execution lease；
- revoke future privileged grants；
- invalidate pending approval applicability；
- cancel queued operations where safe；
- reconcile UNKNOWN operation；
- keep historical Artifact/Evidence；
- late callback 不能 resurrect Run。

---

# 45. Integration 与 Release 生命周期分离

## Integration

```text
PROPOSED
 -> CANDIDATE_CREATED
 -> VERIFIED
 -> MERGED
 -> RECONCILED
```

## Release

```text
CANDIDATE
 -> FINAL_ARTIFACTS_READY
 -> VERIFIED
 -> REVIEWED
 -> AUTHORIZED
 -> PROMOTED
 -> CONFIRMED
```

Merge 不是 Release。

Work CLOSED 也不等于 production released。

---

# 46. Work 生命周期与四种 Done

必须区分：

1. **Requirement Ready**
2. **Engineering Done**
3. **Assurance Done**
4. **Release / Closure Done**

Work phase：

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

blocker/condition 与 phase 分开。

CLOSED 由 Closure Policy 决定，而不是 UI toggle。

Reopen 不删除历史 Closure Manifest。

---

# 47. Run 状态模型

避免单一巨型 enum。

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

UI 可派生 display state。

---

# 48. Command / Event / Audit

所有 mutation：

- command_id；
- idempotency_key；
- actor；
- expected aggregate version；
- correlation_id。

Event：

- event_id；
- aggregate sequence；
- event type；
- actor/source；
- timestamp；
- causation/correlation；
- schema version；
- payload digest。

Event 是 audit facts，不要求整个系统 Event Sourcing。

PostgreSQL 当前 projection 仍是 business authority。

---

# 49. Tamper-evident Audit

“应用 append-only”不够。

增加：

- sequence；
- event/payload digest；
- journal/hash chaining 或 segment digest；
- periodic signed audit checkpoint；
- checkpoint 写入 independently protected/WORM-capable store（按风险）。

目标是检测：

- deletion；
- reorder；
- mutation；
- rollback inconsistency。

不使用 blockchain。

---

# 50. External Operation Ledger

外部副作用视为 at-least-once。

状态：

```text
PLANNED
 -> DISPATCHED
 -> CONFIRMED
```

未知结果：

```text
DISPATCHED
 -> UNKNOWN
 -> RECONCILING
 -> CONFIRMED
    | SAFE_TO_RETRY
    | MANUAL_INTERVENTION
```

适用于：

- Git push/merge；
- PR；
- CI；
- Device flash；
- Artifact publish；
- Release promotion。

HTTP timeout 绝不能自动等价“失败，可重试”。

---

# 51. PostgreSQL / Temporal / Outbox

最终规定：

> PostgreSQL = mutable business authority  
> Temporal = durable orchestration

状态变更：

```text
DB transaction
 ├─ validate expected_version
 ├─ mutate projection
 ├─ append audit event
 └─ append outbox
commit
```

commit 后才：

- start/signal Temporal；
- notify WorkBuddy；
- dispatch integration action。

禁止请求路径直接 dual-write DB + Temporal + external system。

Temporal Workflow state 不能覆盖 PostgreSQL truth。

---

# 52. Recovery Epoch / Disaster Recovery

DB restore 到过去时，现实世界不会回滚。

restore 后：

```text
RECOVERY MODE
 -> increment recovery_epoch
 -> freeze irreversible operations
 -> reconcile external systems
 -> reconstruct confirmed side effects
 -> verify artifact/audit/trust state
 -> authorized recovery exit
```

不同 store 的 authority：

- PostgreSQL：business state；
- Temporal：orchestration；
- Object Store：immutable Artifact/Evidence bytes；
- Audit checkpoint store：tamper evidence；
- Trust/Credential store：issuer/key state；
- Session transcript：operational/debug，弱于 release Evidence。

---

# 53. Trust / Prompt Injection / Hostile Context

以下全部视为潜在 hostile data：

- repository source/comments；
- README；
- issues；
- logs；
- webpages；
- tool response；
- MCP response；
- generated content；
- Knowledge retrieval。

规则：

- Untrusted context 不能改变 policy；
- Runtime 不能自增 capability；
- Runtime 不能获得 production secret；
- Platform Action Gateway 参数验证；
- high-risk action 人审；
- Verification 不依赖 Runtime prose；
- policy/verifier/sandbox definition change 使用加强 Review。

Prompt Injection 不能靠更强 system prompt 解决。

---

# 54. Context Trust 与 Confidentiality

两个维度独立。

## Trust

- AUTHORITATIVE
- TRUSTED_EVIDENCE
- REFERENCE
- UNTRUSTED

## Confidentiality

- PUBLIC
- INTERNAL
- CONFIDENTIAL
- RESTRICTED
- SECRET_REFERENCE_ONLY

Provider routing 同时考虑：

- confidentiality；
- provider eligibility；
- project policy；
- data residency。

---

# 55. Runtime Provider Boundary

Provider profile 至少记录：

- provider/tenant；
- model/deployment profile；
- data retention/training class；
- supported confidentiality；
- region/data residency；
- auth mode；
- tool/network behavior；
- observable version/family。

Provider output 永远不是 authority。

---

# 56. Credential / Secret

默认：

```text
Runtime
 -> Platform Action Gateway
 -> Credential Broker
 -> external system
```

优先代理 action，不把 secret 注入 Agent。

必须保证：

- secret 不进 Manifest；
- secret 不进 Event；
- secret 不进 published Artifact；
- transcript/log redaction；
- ephemeral TTL；
- Run termination/takeover 时 revoke；
- best-effort zeroization；
- secret scanning gate（适用时）。

---

# 57. Source / CI / Supply-chain Trust

CI PASS 只有在 verifier 本身可信时才有效。

CI Evidence 必须绑定：

- exact integration subject；
- workflow/config digest；
- verifier image/software；
- protected verifier identity；
- issuer；
- procedure/policy。

PR 同时修改 product code + release verifier 时，不能让新 verifier 自动给自己 release authority。

依赖解析需记录：

- registry/mirror；
- lock digest；
- resolved package digest；
- base image digest；
- SDK/compiler；
- model/data dependency。

---

# 58. Worker Trust

Ubuntu Worker 有独立 machine identity：

- worker_id；
- enrollment；
- mTLS/workload identity；
- project/environment scope；
- capabilities；
- health；
- software version；
- lease/heartbeat；
- revocation。

Worker 主动建立 outbound authenticated channel。

Worker trust class 可包括：

- DEV_WORKER
- VERIFIED_BUILD_WORKER
- DEVICE_LAB_WORKER
- RELEASE_WORKER

普通开发机不是 production signer。

---

# 59. Protocol Version Negotiation

以下都必须声明协议版本：

- Worker；
- eng CLI；
- WorkBuddy Connector；
- Device Agent；
- Runtime Adapter；
- Session Gateway。

连接时交换：

- software version；
- protocol min/max；
- feature/capability set。

不兼容则 reject/quarantine，而不是猜。

平台升级不能把 active Formal Run 悄悄 stranded。

---

# 60. Release Promotion Reconciliation

Merge / Release / OTA 都是 external side effects。

Release promotion 后必须查询 remote reality：

- exact digest/bundle；
- environment；
- channel；
- target；
- promotion receipt。

“HTTP 200”不是最终 Evidence。

最终 production bytes/state 必须 reconciliation。

---

# 61. Quarantine 与 Incident

可以 quarantine：

- Worker；
- Device；
- Fixture；
- Artifact；
- Release；
- Evidence Issuer；
- Toolchain/environment；
- Dependency source。

Quarantine 不删除历史。

它阻止新的 trusted use，并触发依赖影响分析。

Field evidence/incident 作为新记录挂到 immutable Release，不回写过去。

---

# 62. Deletion / Retention

Immutable 不等于永远不能合法删除 bytes。

需要：

- retention class；
- deletion/tombstone；
- actor/authority；
- reason；
- dependency impact。

即使 raw bytes 被合法删除，也要保留：

> 曾经存在，但当前不可重建。

不能让 deletion 看起来像“从未存在”。

---

# 63. Closure Manifest

Work 关闭时生成不可变 Closure Manifest：

- Requirement Revisions；
- Task Revisions；
- Target；
- Runs / Attempts；
- Integration Subject；
- final source；
- final Artifacts；
- Evidence；
- Verification；
- Review；
- Risk / Decision；
- Release；
- open/accepted risk；
- follow-up Work。

WorkBuddy 的最终交付摘要从 Closure Manifest 派生。

---

# 64. Break-glass / Degraded Mode

## Explore degraded mode

Control Plane 故障时，可继续：

- local read；
- analysis；
- experiments。

但不能声明 Formal provenance，也不能使用 privileged shared resources。

## Formal break-glass

必须：

- authorized Break-Glass Decision；
- bounded scope/time；
- signed/local evidence bundle；
- production authority 仍保留；
- 后续 mandatory reconciliation/import；
- 显著 audit marker。

Break-glass 不是 hidden bypass。

---

# 65. 技术栈

参考实现：

| 模块 | 建议 |
|---|---|
| Control API | Go |
| Business DB | PostgreSQL |
| Durable Workflow | Temporal |
| Object/Artifact Store | S3 / MinIO |
| CLI | Go |
| Worker/Supervisor | Go |
| Engineering plugins | Go/Python |
| Session Gateway | Go，WebSocket/stream |
| Worker channel | mTLS gRPC/stream |
| Workspace | Git worktree |
| Sandbox | rootless container / namespace + cgroup |
| Runtime | Codex first, Claude second |
| Secret | Vault / enterprise secret manager |
| Observability | OpenTelemetry |
| Device Agent | Go/Rust/Python lightweight agent |
| WorkBuddy | Enterprise MCP Connector |

初期保持 monorepo，不拆大量微服务。

---

# 66. 推荐仓库结构

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
│   ├── target/
│   ├── interface/
│   ├── run/
│   ├── artifact/
│   ├── evidence/
│   ├── verification/
│   ├── review/
│   ├── risk/
│   ├── decision/
│   ├── release/
│   ├── policy/
│   ├── audit/
│   └── workflow/
├── runtimes/
│   ├── contract/
│   ├── codex/
│   └── claude/
├── worker/
│   ├── sandbox/
│   ├── workspace/
│   ├── supervisor/
│   └── local-tools/
├── gateway/
│   ├── actions/
│   └── credentials/
├── connectors/
│   └── workbuddy/
├── device/
│   ├── registry/
│   ├── lease/
│   ├── agent/
│   ├── fixtures/
│   └── procedures/
├── schemas/
│   ├── domain/
│   ├── manifests/
│   ├── events/
│   ├── policy/
│   └── protocols/
├── docs/
│   ├── architecture/
│   ├── adr/
│   └── reviews/
├── deploy/
└── tests/
    ├── unit/
    ├── integration/
    ├── property/
    ├── failure-injection/
    └── adversarial/
```

---

# 67. M0 — Contract Freeze

M0 不做“完整 AI 平台”。

M0 必须冻结：

1. Domain authority / aggregate boundary ADR
2. PostgreSQL / Temporal ADR
3. Worker trust / Session Gateway ADR
4. Local Tool vs Platform Action Gateway ADR
5. Content-addressing / canonicalization ADR
6. Evidence Issuer / Trust Root ADR
7. Policy / Human Authority / SoD ADR
8. Idempotency / reconciliation ADR
9. Break-glass ADR
10. Target / Variant / Compatibility / Bundle ADR
11. Integration Subject / Merge ADR
12. Artifact Transform / Signing ADR
13. Schema / protocol evolution ADR
14. Readiness / Assurance / Verification Plan ADR
15. Recovery / Audit Integrity ADR

同时产出：

- Common schema envelope
- JSON Schemas
- Manifests schemas
- Command/Event schema
- Structured Steering schema
- State/Guard tables
- Policy schema
- Initial OpenAPI
- Worker protocol
- Session Gateway protocol
- Device Agent protocol
- Failure matrix
- Property/invariant test specification

M0 完成前不大规模写业务代码。

---

# 68. M1 — Minimal Vertical Slice

M1 只支持：

- one WorkBuddy Connector；
- one project/target path；
- PostgreSQL；
- Temporal；
- one Session Gateway；
- one Ubuntu Worker；
- one repo operationally；
- Codex；
- one CI provider。

但必须包含完整核心语义：

```text
Requirement READY
 -> Verification Plan
 -> Work / Task Revision
 -> Run Input Manifest
 -> fenced Run Attempt
 -> interactive Codex
 -> Steering / Checkpoint / Resume / Takeover
 -> exact integration subject
 -> Artifact
 -> trusted CI Evidence
 -> Verification
 -> Closure Manifest
 -> WorkBuddy
```

原则：

> **Cut breadth, not semantics.**

---

# 69. M1 NO-GO

以下任一存在，M1 不算完成：

- manifest hash 非确定；
- stale epoch 能 mutate Run；
- duplicate command 能重复 privileged side effect；
- UNKNOWN operation 被 blind retry；
- Runtime/普通 Worker 能自发 trusted PASS Evidence；
- CI verifier 可以被 subject 自己改坏后自证；
- artifact consumption 不重新验证 digest；
- approval 未绑定 exact digest/scope；
- stale Session token 还能 steering；
- Requirement 非 READY 可启动 Formal Run；
- superseded/cancelled Run 仍拥有 privileged grant；
- old schema 无法继续验证；
- merge result 未 reconciliation；
- confirmed external effects 能被 checkpoint resume 重放；
- audit reconstruction 需要解析 terminal transcript；
- secret 出现在 manifest/event/published artifact；
- DB rollback 后还能立即生产操作；
- Control Plane upgrade 能 strand active Run without Decision。

---

# 70. M2 — Embedded Trusted Loop

增加：

- Target/Variant fully implemented；
- Device Registry；
- lease/fencing；
- Device Agent；
- Fixture/Calibration；
- flash/procedure；
- Raw Evidence；
- statistical evaluation；
- HIL；
- Release Bundle；
- compatibility；
- OTA migration/rollback；
- final-byte verification。

M2 NO-GO 包括：

- target 未确认可 flash；
- actual config 未测量；
- incompatible bundle 能部署；
- fixture/issuer quarantine 后还能出 trusted Evidence；
- flash timeout blind retry；
- rollback 只写文档未验证；
- final Release Bundle 可换组件不产生新 digest/approval。

---

# 71. M3 — Multi Runtime

增加：

- Claude Adapter；
- provider-neutral Runtime Contract；
- Runtime switching；
- fallback/comparison。

Runtime 切换必须产生新 Run，并保留 parent/checkpoint lineage。

---

# 72. M4 — Planner / Skill

只有真实 Work 数据积累后引入：

- Capability Registry；
- Skill Registry；
- Context Resolver；
- automatic Task DAG proposal；
- runtime routing。

Skill 需要：

- owner/origin；
- version；
- digest；
- required capability；
- trust；
- compatibility；
- review status。

Skill 不能自授 capability。

---

# 73. M5 — Engineering Knowledge

只沉淀：

- Known Issue；
- Design Rule；
- Compatibility Rule；
- Recovery Runbook；
- Test Method；
- Decision Pattern。

Knowledge 项带：

- source provenance；
- trust；
- confidentiality；
- revision/supersession；
- freshness；
- curator；
- digest。

Retrieved Knowledge 仍然是 Context，不自动成为 Authority。

---

# 74. Pilot

三个 Pilot：

## Pilot A — 普通 MCU 功能

```text
Requirement -> Code -> Build -> CI -> Evidence
```

## Pilot B — Linux/BSP Debug

```text
RCA -> Steering -> Pause -> Takeover -> Resume
```

## Pilot C — Motor / Device

```text
Code -> Firmware -> Flash -> HIL
-> Raw Evidence -> Evaluation -> Verification
-> Release Bundle
```

这三类足以暴露大多数架构问题。

---

# 75. Failure Injection

必须至少覆盖：

- stale Worker write；
- duplicate callback；
- network partition；
- Runtime crash；
- Session Gateway restart；
- Temporal outage；
- PostgreSQL failover；
- PostgreSQL rollback restore；
- artifact substitution；
- revoked issuer；
- modified verifier workflow；
- prompt injection；
- malicious build script；
- secret exfiltration attempt；
- Device flash timeout；
- Release timeout after actual success；
- stale approval replay；
- policy child widen parent DENY；
- power loss during OTA；
- incompatible component bundle。

---

# 76. Property / Invariant Tests

架构原则必须可执行。

示例：

- stale execution_epoch 永远不能改变 Run；
- modified Release Manifest 永远不能复用旧 approval；
- stale/revoked Evidence 永远不能满足 current Verification；
- duplicate command 永远不能产生 duplicate privileged side effect；
- Human Takeover 时 Runtime 不能写；
- revoked issuer 不能产生新的 trusted Evidence；
- parent DENY 不能被 child scope 静默放宽；
- final promotion bytes 必须等于 approved Release Bundle；
- DB recovery mode 未退出前不能 production promotion。

---

# 77. Observability / Cost / Admission

OpenTelemetry correlation 覆盖：

- Control API；
- Temporal；
- Worker；
- Session Gateway；
- Action Gateway；
- CI；
- Device。

运营指标：

- active Run；
- resume success/RTO；
- stale epoch rejection；
- reconciliation count；
- approval wait；
- device utilization；
- artifact finalize failure；
- model/API/CI/compute cost。

Formal Run policy 可限制：

- wall time；
- model/API budget；
- retries；
- tool count；
- network transfer；
- concurrency；
- future multi-agent depth。

成本永远不是 Verification 结果。

---

# 78. Retention / Archival

分类保存：

- business state；
- audit；
- runtime transcript；
- checkpoint；
- raw Evidence；
- release Artifact；
- device trace；
- signatures/attestations。

Release-grade closure 可导出独立 archival verification package：

- manifests；
- schemas；
- signatures；
- Evidence metadata/raw evidence policy；
- Verification/Review/Decision；
- key/cert chain refs；
- source snapshot as required。

长期审计不依赖未来 live Git/Control Plane 一定存在。

---

# 79. 公司级 Non-negotiable Invariants

最终冻结：

1. Requirement Revision is immutable.
2. Task Revision is immutable.
3. Formal Requirement must pass readiness before execution.
4. Formal Run has explicit immutable input lineage.
5. Only one fenced Run Attempt may own execution.
6. Checkpoint is provider-neutral and does not roll back external reality.
7. Artifact is immutable and content-addressed.
8. Structured manifests use deterministic canonical serialization.
9. Evidence is immutable and has a verifiable issuer.
10. Evidence result and applicability are separate.
11. Verification binds an exact Subject Manifest.
12. Verification, Review, Risk Decision and Release Approval are distinct.
13. Runtime claims are not Verification.
14. External side effects are idempotent or reconciled.
15. Unknown external outcome is never blindly retried.
16. Human approval binds exact digest, scope and authority.
17. Connector/service/platform admin identity is not automatically Human Authority.
18. Permission is actor/resource/environment/capability based and enforced outside the model.
19. Production/signing credentials remain outside untrusted Runtime/build steps.
20. PR head Verification does not automatically prove final integrated source.
21. Signing/packaging transforms create new Artifacts and lineage.
22. Final release-sensitive checks bind exact final bytes.
23. Release Bundle binds exact Target Revision and component set.
24. Revision supersession propagates impact to downstream authority.
25. Work closure and production release are separate concepts.
26. Recovery after business-state rollback reconciles external reality before irreversible actions resume.
27. Audit history is tamper-evident at signed checkpoint boundaries.
28. Every Release is reconstructable from immutable manifests, provenance, Evidence, Review and Decisions.

---

# 80. 最终主链

```text
Product
  │
  ▼
WorkBuddy
  │
  ▼
Requirement Revision
  │
  ├─ Readiness
  ├─ Risk / Assurance Profile
  └─ Verification Plan
  │
  ▼
Engineering Control Plane
  │
  ├─ Work
  ├─ Task Revision
  ├─ Target / Interface / Compatibility
  ├─ Policy / Authority
  └─ Workflow
  │
  ▼
Run Input Manifest
  │
  ▼
Ubuntu Engineering Worker
  │
  ▼
Execution Sandbox
  │
  ▼
Run / Fenced Attempt
  │
  ├─ Codex / Claude
  ├─ Steering
  ├─ Clarification
  ├─ Pause / Resume
  ├─ Human Takeover
  └─ Checkpoint
  │
  ▼
Run Receipt
  │
  ▼
Integration Subject
  │
  ▼
Immutable Artifact
  │
  ├─ Trusted Build / Provenance
  ├─ Controlled Transform / Signing
  └─ Final Release Bundle
  │
  ▼
Device / HIL / CI
  │
  ▼
Raw Evidence
  │
  ▼
Evaluation Evidence
  │
  ▼
Verification
  │
  ▼
Independent Review
  │
  ▼
Risk / Policy Decision
  │
  ▼
Human Authority
  │
  ▼
Release Manifest
  │
  ▼
Promotion + Reconciliation
  │
  ▼
Release
  │
  ▼
Closure Manifest
  │
  ▼
WorkBuddy
```

---

# 81. 一句话定义

> **办公入口 WorkBuddy 化，研发治理 Control Plane 化，工程执行 Ubuntu 化，Coding Agent Runtime 化，过程交互 Session 化，配置 Target 化，交付物 Artifact 化，工程证明 Evidence 化，验证 Assurance 化，生产决策 Human Authority 化。**

---

# 82. 实施决策

从本版本开始：

- **v1.2 是 canonical target architecture。**
- v1.0/v1.1 与 Round 2–9 Review 作为设计历史保留。
- 不再增加新的顶层 Plane。
- 不再继续泛化宏观架构。
- 下一阶段正式进入 **M0 Contract Freeze**。
- 只有 M0 ADR / Schema / State Guard / Protocol / Invariant Test 完成后，才进入大规模 M1 实现。
