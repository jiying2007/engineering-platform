# engineering-platform（退出与历史参考）

> **2026-11-20：停止作为个人/小团队的新研发执行入口。** 已存在的安装和历史证据不得因本 README 被误认为自动停机、已迁移或已删除。

当前推荐的实际研发入口：

- 日常交互开发：原生 Codex CLI/VS Code、现有 ADK、项目 Git/CI。
- 飞书远程无人值守：\`jiying2007/codex\` 中已有 \`tools/codex_assets/feishu_codex_bot.py\`，在该处补可靠恢复和其他 Agent 的受控调用接口；不是在本仓重建执行器。
- 跨仓/主机会话：\`codex-tui\` 只做定位、注意力与接管，不作为任务/模型执行权威。
- 产品需求与缺陷：飞书项目/TAPD；项目 PR/CI、SIL/HIL 与实机证据分别按权威验收。

[退役和能力吸收 Final 1.5](docs/decisions/engineering-platform-v0-final-20261119.md) 是唯一未来决策；[旧部署切换注意事项](docs/small-team-migration.md) 和 [历史 implementation status](docs/status/IMPLEMENTATION_STATUS.md) 为旧事实。新决定不会把历史 Blocked/Unknown/外部资格写成 Pass。

## 保留内容，不再产品化

- \`tool/\`：历史单主机 Go/SQLite \`eng\` 工具、测试和备份/发布示例。**仅供阅读与提炼防重试/恢复等不变量，禁止将其当作当前飞书执行服务。**
- \`cmd/\`、\`internal/\`、\`db/\`：更早的 Core/Worker/Publisher/PostgreSQL 设计与证明；不是推荐的个人安装。
- \`docs/evidence/m1-terminal-facts/\`：历史 Feature/Debug 事实材料，保留原来源、SHA 和范围；不是生产或设备当前资格。
- \`docs/implementation/\`、\`docs/status/\`：历史合同、审计及未解决缺口。

仓库暂时不归档、不删除历史、不中断任何仍可能在运行的服务。必须先在实际主机核实进程、UNKNOWN/外部效果、私有 DB/产物备份及消费者依赖，再由责任人安排停用与只读归档。此前 PR #311 Python 前台 V0 的无模型通过不是无人值守资格。

**不扩建**新的飞书/Agent Gateway、长期队列、控制平面、重复 Issue 台账或模型配额系统。
