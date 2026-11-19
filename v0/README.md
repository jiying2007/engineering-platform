# eng V0 — 一个命令做一次工程任务

**日常入口：进入已有 Git 仓库，运行一次任务，不部署平台。** 本目录只有一个 Python 标准库脚本、安装器和无模型测试。要求 Ubuntu/Linux、Git、Python 3.10+。不需要 Go、PostgreSQL、SQLite、Docker、systemd、服务端、定时任务或新账号。

## 安装

```bash
# 在 engineering-platform 仓库根目录
bash v0/scripts/install.sh
export PATH="$HOME/.local/bin:$PATH"
eng doctor
```

安装器**不会覆盖现有 eng**。如果以前安装过 Go 版本，先检查它是否仍有活动任务并备份状态；确认停止后，再运行 `bash v0/scripts/install.sh --replace`，安装器会保留旧二进制副本。安装不调用模型、不会停止旧服务或删除旧数据。

## 一次明确的 Runtime 配置，然后一条命令运行任务

先在目标 Ubuntu 主机安装、登录并验证**获准使用**的既有 CLI，确认其模型费用/额度不属于受限渠道。明确选择一种接口，不自动发现或静默切换：

```bash
# 示例：如果 OpenCode 已安装，且其当前 Provider/Model 已经得到授权
eng use -- opencode run '{prompt}'

cd /path/to/existing-git-repo
eng doctor
eng run '修复已有单测，并说明实际改动' --check 'python3 -m unittest'
```

示例只是接口形式：**eng 不安装、登录、验证或授权 OpenCode，也不承诺它调用的模型免费**。不要未经许可使用 Codex/ChatGPT Work 共享额度。`eng use` 仅保存命令参数，**不会调用模型**。

需要以标准输入传任务的 CLI，配置为 `eng use -- EXISTING_CLI [ARGS]`（不写 `{prompt}`）；需要任务文件路径的 CLI，可用 `{task_file}` 占位符。Runner 不通过 shell 展开，命令参数不得写 API Key、Token 或密码；身份沿用 CLI 原有的私有配置。**只信任自己选择的 CLI 与仓库代码；worktree 不是沙盒。**

较大或敏感的需求不直接写在 shell 历史中：

```bash
eng run --task /path/to/task.md --check 'make test'
```

`eng run` 的内联文本上限 16 KiB，任务文件最多 1 MiB；较大输入必须用标准输入或 `{task_file}` 传给 Runner，不能用 `{prompt}` 拼成过长的命令行参数。

## 实际交付与恢复

`eng run` 不会修改原仓库检出。它从当前 HEAD 建立一个 `eng/v0-*` 分支和独立 worktree；Runner 修改这个工作区；结束后保留 `status.json`、`task.md`、runner/check 日志、`result_commit` 和 `diff.patch`。使用 `eng status` 查看最近 20 条，或 `eng status TASK_ID` 查看单条。

- 只有 **Runner 返回 0、确实产生结果提交、所有显式检查返回 0 且检查未修改工作区**，才标记 `locally_verified`。这仍不是 PR 审查或业务完成。
- 没有检查、没有代码变化，或检查改了源码，标为 `needs_review`；Runtime/检查失败为 `failed`。退出码分别为 `0` / `2` / `1`。
- 取消、超时或存在无法确认的进程/远端效果，保留 `unknown`（退出码 `3`），**不自动重试**。先查看记录和真实外部结果再操作。
- 执行上下文、登录/凭据与成本仍归现有 CLI；**未读取到模型用量时始终标为未知，绝不写零费用**。

继续工作时，直接 `cd` 到 `status.json` 中的 `worktree`，审阅并手工处理已有更改；不创建第二套自动 Resume/Reconcile 系统。按现有 Git/`gh` 建 PR、评审和合并；V0 **不会自动 push、合并、创建后台进程或注册定时任务**。同一个 Git common-dir 一次只运行一个 V0 任务。

任务不应依赖当前工作区未提交的改动，V0 始终从 Git HEAD 建隔离 worktree。仓库里如有可执行 Git smudge/clean/process filters，会拒绝运行，避免隐式子进程执行。源码仍需可信，进程组不能等同于容器沙盒。

运行前可不调用模型地核查：

```bash
eng doctor
eng run '说明目标任务' --dry-run --check 'make test'
```

`doctor` / `--dry-run` **不会检查真实模型登录或调用权限**，仅检查本地命令与 Git 前提；真实模型能力必须在用户批准的执行环境中用一次实际研发任务验证。

## 测试与范围

```bash
python3 -m unittest discover -s v0/tests -v
```

测试仅调用确定性本地 Python Runner、真实 Git 和可执行检查，不消耗任何模型额度。本 V0 不是长期兼容旧工程平台：旧 Go `tool/`、PostgreSQL Core 的历史由精确 Git 提交与证据归档保留，不作为新用户安装、配置或执行前置。**在实际部署主机确认旧任务安全退出之前，不要擅自删除旧服务或其数据。**
