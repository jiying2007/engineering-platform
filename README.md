# engineering-platform

**给个人和小团队使用的极简本地工程任务入口**：现在默认采用 [eng V0](v0/README.md)。它只是一个 Python 标准库脚本，直接复用已有 Git、已批准的 AI CLI、项目测试与 GitHub；没有独立服务、任务数据库、Provider 管理平台或多 Agent 编排。

## 第一次使用

```bash
# Linux / Ubuntu；需要 Git 和 Python 3.10+，不需要 Go/PostgreSQL/Docker
bash v0/scripts/install.sh
export PATH="$HOME/.local/bin:$PATH"

# 仅在确认 CLI/模型已获准、登录与费用来源之后手工选择 Runner：
eng use -- opencode run '{prompt}'

cd /path/to/existing-git-project
eng doctor
eng run '修复日志解析的已有问题' --check 'python3 -m unittest'
eng status
```

这里的 `opencode` **仅是命令接口示例**，不会自动安装、登录、验证模型或调用费用受限的 Codex/ChatGPT Work。任何真实模型调用都取决于用户明确配置且已获准的既有 CLI；`eng use`、`eng doctor`、测试、构建和安装不会调用模型。没有授权的 Runtime 时工具会明确拒绝执行。

## 做哪些事

V0 从现有 Git HEAD 建一个独立 worktree，执行一次明确的 Runner 和真实检查，保留日志、变更提交、diff 和简短的 `status.json`。**没有检查、检查失败、无代码改动或外部结果未知不会宣称通过**。原开发目录保持不变。V0 不自动重试、不自动发 PR、不自动合并、不启动后台服务，超时/中断保留证据。需要人工接着做时直接进入其 worktree，沿用 Git/`gh`。

任务参数写在 shell 历史中，敏感或大任务使用 `eng run --task FILE`；命令行长度限制和 CLI 支持格式见 [V0 操作说明](v0/README.md)。

## 开发与验收

```bash
make check     # 真实 Git、进程和确定性本地 Runner 的无模型测试
make build     # 复制 V0 脚本至 bin/eng
make release   # 生成小型源码发行包与 SHA-256
```

**边界**：V0 的测试成功只证明本地执行框架，不证明真实 AI 代码质量、用户 Ubuntu 主机已安装、设备验收或模型费用。验收必须来自已获准 Runtime 的真实开发任务和原仓检查，不得用 fixture 冒充。

原多服务/Go 代码及其公开证据在 Git 历史中保留。当前尚未实地核验所有旧服务停止，因此不能以这次默认入口切换冒充旧系统已退出，也不能宣称 [`digital-worker`](https://github.com/jiying2007/digital-worker) GitHub 仓库已经删除。历史工作只读回查，V0 不兼容旧状态数据库，也不迁移旧 UNKNOWN 结果。
