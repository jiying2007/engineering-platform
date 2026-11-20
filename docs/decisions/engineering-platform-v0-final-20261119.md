# Engineering Platform｜退役与能力吸收决策

**Final 1.5｜2026-11-20｜个人与小团队（目标是少一套运行时）**

> 本文件是公开的脱敏决策。私有 \`jiying2007/sessions\` 会话原件、原始提示、路径、凭据、费用账单和完整审计均不进入本仓库。本文件决定未来产品边界，不声明历史模型调用/设备资格/生产主机退出已完成。

## 1. 决策

**停止将 \`engineering-platform\`／\`eng\` 作为新用户入口或独立无人值守工程平台发展。** 本仓的代码与 CI 先保留为历史事实、迁移参考和按需提取的安全测试，不新增另一套 SQLite 任务服务、飞书 Bot、MCP server、Agent Loop、PostgreSQL 控制面或 Provider 资格平台。

- **交互开发**：原生 Codex CLI/VS Code + 当前 ADK；多目标会话观察仍在 \`codex-tui\`。
- **无人值守与飞书远程控制**：由既有 \`jiying2007/codex/tools/codex_assets/feishu_codex_bot.py\` 统一拥有。当前有 WebSocket、白名单、SQLite、worktree、runner、systemd；尚需真实重启恢复与授权运行验收。
- **其他 Agent 调用**：给同一 Codex Bot 增加最小的受控本机 MCP/CLI（或等价受保护接口）；使用同一任务状态机和授权，不建立 eng 第二执行器。不可开放任意远程 Shell。
- **需求/缺陷**：飞书项目或 TAPD；**代码/事实**：Git/CI/HIL/设备；**方法资产**：ADK；**知识**：Knowledge Hub。
- **KWS/Audio**：保留原实验流水线，不使用 eng 包装训练/评估或新巡检来冒充实际算法研发。

## 2. 为什么不保留 eng 作为 optional runner

原生 CLI、已有 \`codex\` 飞书执行服务和 Git/CI 已覆盖会话、代码修改、worktree、检查与本地交付。主线 \`tool/\` Go + SQLite、PR #311 Python V0 若继续执行同一职责，会引入重复凭据和工作区竞争，也延续此前多次重设计但未实质采用的问题。

**PR #311 不应合并**；其绿色无模型检查不代表实际 Agent 质量或价值增益。历史 \`cmd/\`／\`internal/\`／\`tool/\` 不直接迁移到新的 Codex 服务。只有出现已证实的跨多个异构模型 Runtime、多主机真实调度需求且 Codex 不能承接，才以新证据重新评估独立工具；不自动恢复旧平台。

## 3. 选择性吸收清单（不复制实现）

| 来源 | 优先级 | 接收者 | 复用的规则/证据 | 不搬入 |
|---|---|---|---|---|
| \`tool/internal/local/process_linux.go\`, \`store.go\`, \`serve.go\` | P0 | \`codex\` Bot | 进程身份 PID/start_ticks/boot_id、队列持久化、未知效果不盲重试、状态转移/故障回归 | Go 服务、第二 SQLite/调度器 |
| \`tool/internal/local/backup.go\` | P0/P1 | \`codex\` Bot | WAL 一致性快照、备份完整性、恢复不隐式重放 | 同机路径专属 eng 数据模型 |
| \`tool/internal/local/publish.go\` | P1（仅授权后） | \`codex\` Git 交付接口 | 精确 SHA/remote/PR readback、未知结果不重复 Push、发布权限分离 | 自动 Merge / 生产发布 |
| \`docs/evidence/m1-terminal-facts/\` | 保全 | 本仓历史 + Knowledge Hub 索引 | 原始 run/artifact/commit/digest 范围，Feature/Debug 历史结果 | 重新签发证据或宣称当前设备通过 |
| \`tool/examples/templates/\` | 去重 | ADK/Codex 已有任务流程 | 只吸收确实缺少的验收负例 | 复制第三套模板 |

P0 首个落地候选：\`jiying2007/codex\` PR #69（持久化待执行任务载荷，running 重启后 UNKNOWN，不调用模型、不自动重试），实际合入/主机部署仍依项目 CI 和现场验证。

## 4. 退役前必须核验的事实

1. **旧主机**：明确列出真实执行账号、\`eng\`/Core/Worker/Publisher/systemd 进程、正在执行的 Work/Run、DB/WAL/产物和外部 Push/PR/设备操作；UNKNOWN 不得改为 canceled 或 done。
2. **保全**：留存仓库完整 Git 历史、M1 历史证据、必要的数据库一致性备份和只读索引；保存 SHA-256 与恢复抽检，不复制密钥/原始会话到公开 repo。
3. **替代验收**：真实飞书消息→服务→获准 Runtime→实际 Git/测试→飞书回执；重启/失联、同一消息重复投递、超时停止、工作区锁和费用边界各有可复核证据。无模型 CI 只验证工具行为。
4. **依赖切断**：ADK、Codex、TUI、知识库、现有工程项目不得依赖本仓 live 代码/DB；只读历史引用允许，具体使用者需要明确检查。
5. **归档**：完成 1–4 后优先 GitHub Archive（保存原 SHA 和历史 URL）；如用户另行要求永久删除，应先验证可恢复的 Git bundle 和外部引用，不以本决策代替删除操作。

**当前状态**：仓库仍公开且未归档；本决策及 PR 本身没有访问或停止真实用户主机，也没有进行模型调用。旧服务生产 readiness 历史上仍为 BLOCKED，不因新 Codex Bot 路线而变为 PASS。

## 5. Codex 使用史的证据边界

私有会话仓完整 Git Tree 已核对：历史来源包括 Windows/Linux VS Code、交互 CLI、\`codex exec\`、子 Agent 和其他客户端。文件数不等于模型调用数，\`task_complete\` 不等于 PR/CI/QA/设备通过；历史 token 事件也不能简单累加。完整内容校验和账单核对必须在用户受控的私有主机上另行完成，不能以采样或目录统计代替。

**终态**：Codex 拥有唯一无人值守入口，codex-tui 只观察，ADK 负责方法，Knowledge Hub 负责知识，Git/CI/设备/飞书项目或 TAPD 仍分别拥有原始事实与业务验收。eng 保留历史后退出。
