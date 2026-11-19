# Engineering Platform｜Codex 真实使用场景驱动的最终决策

**Final 1.4｜2026-11-19｜个人优先、2–8 人可信研发团队**  
**公开决策摘要。原始 Codex 会话位于用户私有档案中；不在本公开仓库存放原始日志、个人目录、提示词、工具输出、Provider 凭据或私有档案元数据。**

## 一、最终决定

**保持原生开发工具为主，停止建设第二套 Agent / 工程任务管理平台。** Codex CLI、Codex VS Code 扩展（或另一个明确获准的 CLI）负责实际编码、会话、执行与审批；`codex-tui` 可选用于多仓、多目标 Codex 会话观察和定位；Git、原仓 CI、GitHub/GitLab 负责代码与评审；飞书项目／TAPD 负责产品需求、测试缺陷与业务验收。

**`eng` 不再作为默认 AI 模型执行器。** 仅在真实任务证明存在净收益时，保留确定性的 Git worktree、检查、diff、日志和证据辅助能力；不得为了完整平台而继续增加 Runner 注册、长期任务队列、多模型切换、团队后台或新的工单权威。

## 二、依据：原生 Codex 历史比此前方案覆盖得广

已通过具授权的 GitHub 连接器审阅用户**私有 Codex 会话仓库的完整 Git Tree**，并对可直接读取的原始 JSONL 做跨月份分层内容审计。原始记录支持以下边界，而不是单一 `codex-tui` 工作流：

- Windows + VS Code 下有 MCU／驱动项目开发与本地 Shell 调用；Linux + VS Code、原生交互 CLI、非交互 `codex exec` 也有实际工作轨迹。
- 会话元数据有独立的 `originator`、`source`、Provider、CLI 版本、模型和工作目录；存在子 Agent、已完成、已中断及只有启动事件的会话。**文件数、thread 数、Token 事件数、成功研发交付数不可混为一谈。**
- 历史中可见多种 CLI 版本和 Provider 来源。硬编码特定模型、强制团队锁定相同版本、按 `codex` 进程名判断费用授权都会与真实使用冲突。
- `token_count` 可能是多次累积快照，不能重复累加；Codex 的 `task_complete` 不等于 Git/CI 通过、设备验收或产品 QA 关闭。

**证据边界：**完整目录已核验，原始会话的内容审计尚是样本级，压缩/超大文件需用户在已有认证主机上用离线、流式、脱敏工具复查；不宣称已核查所有模型调用、账单或全部 ChatGPT 历史。细节见用户私有审计归档，不进入公开仓库。

## 三、个人与团队的真实工作流

**日常功能/缺陷：**原始飞书项目或 TAPD 工单（可经企业微信进入）→ 开发者在原生已批准 CLI/IDE 及 ADK 中实施 → 原项目实际检查 → GitHub/GitLab PR/MR 与 CI → 产品/QA 在原工单复测及关闭。仓库 Issue 允许用于技术维护，但不复制业务需求或 Bug。

**嵌入式与跨仓接口：**Windows IDE 或 Ubuntu 开发机均是一等场景。SoC/MCU、sensor/app/api/hdi 等多仓联调使用版本化 UART/API 契约、各自分支/PR 和真实联调证据；不建立 Agent-to-Agent 消息服务。MCU/FOC/OTA 还需固件身份、测试设备、负载、电流、温升或升级实测，不能由 CI 代替。

**KWS/Audio 长周期研发：**复用已有 GitHub Actions/合格执行主机进行事件驱动数据、训练、评估和平台资源验证。已有数据和准入证据继续复用，不以无模型准备、自动巡检或新 `eng` 执行包装代替实际算法研究。

**团队与多主机：**每位开发者独立凭据和本地会话；不固定全员相同 CLI 版本，运行前按实际目标探测兼容性。Codex target/thread 需结合真实主机和 `CODEX_HOME` / App Server 目标定位，不能凭同名 Git 目录判断是同一实例。`codex-tui` 只投影上游状态，历史 rollout 文件不是实时运行权威。

## 四、费用、正确性和 V0 准入

当前关于 Codex／ChatGPT Work 共享额度的使用限制**继续有效**。历史 ChatGPT 登录、Relay 或中转站配置，不等于当前获得模型执行授权；无明确批准时只做无模型校验，不自动启动 Codex 回合，也不以模拟 Runner 宣称实际研发完成。

PR #311 的 Python V0 仅证明无模型执行路径；不应直接将其变成所有人的默认执行器。若确需保留其独立执行能力，首先解决跨 `ENG_HOME` 的 Git common-dir 互斥、持久进程身份/强制中断后的未知处置、`git add -A` 的敏感文件暂存，以及真实 Runtime 账号/费用/退出语义。**若这些额外维护成本超过它节省的工作步骤，应移除执行包装，只保留辅助命令。**

## 五、验收、停止条件与旧系统退出

| 闸口 | 验收依据 |
|---|---|
| 私有档案 | 已完成 Git Tree 核验；全量 JSONL/Zstandard 内容审计仍以本机流式执行和失败清单为准，不能用小样本冒充全部。 |
| 工具兼容 | Windows VS Code、Ubuntu CLI/SSH、多会话、非交互执行分别按真实能力验证；不强制用户迁移到单一 UI。 |
| 真实交付 | 经批准的模型/工具产生源码变更、准确提交、项目检查、PR/CI；需要 QA 或设备证据的按各自权威验收。 |
| 价值验证 | 用两个真实仓库、约五个低风险任务对照原生 CLI + ADK + Git；`eng` 没有可量化净收益则停止发展其执行器。 |
| 旧平台退出 | 单独核实旧主机进程、UNKNOWN、数据库与历史凭证后再移除旧服务；`digital-worker` GitHub 仓库删除独立验收，不能因文档修改宣称已删除。 |

## 六、原有方案关系

本文件是此前工程平台归档决策的 **Final 1.4 脱敏公开修订**，沿用既有 PR #312 更新；PR #311 是待取舍的 V0 实现候选，不以绿色 CI 自动认定生产可用。不在多个仓库建立新的任务中心、模型 Provider 平台或监控巡检系统。私有会话原件与详细审计留在用户可控的私有仓库／私有文件归档中。

参考：`https://github.com/jiying2007/engineering-platform/pull/312`、`https://github.com/jiying2007/engineering-platform/pull/311`、`https://github.com/jiying2007/codex-tui/blob/main/docs/adr/012-upstream-convergence.md`。

**终态：原生工具负责真实执行，仓库与 CI 负责技术交付，业务系统与设备实测负责验收；codex-tui 只做观察，eng 只有在客观有价值时才作为小工具存在。**
