# engineering-platform

面向个人和小团队的本地工程任务工具。当前使用入口是 **[`eng`](tool/README.md)**：一个 Go 二进制、一个本地 SQLite 数据目录、每个任务一个 Git worktree。它调用你已经配置并获准使用的 CLI，运行项目检查，保存日志、差异和产物。

适用范围是一个 Ubuntu 执行主机上的可信用户和可信项目。个人默认前台运行；需要排队时启用同一程序的 `eng serve`。需求、评审和合并继续使用现有 Issue、Git 与项目 CI。

## 从这里开始

1. 从 [Eng local CI](https://github.com/jiying2007/engineering-platform/actions/workflows/eng-local.yml) 对应提交的成功运行下载 `eng-linux-amd64` artifact。
2. 按[安装与首次使用](tool/README.md#安装已有发行包)校验和安装预编译包。
3. 先运行发行包自带的无模型示例，再登记自己的项目、Runtime 和检查命令。

```text
eng doctor
eng runtime add team-default --model --stdin task --timeout 45m -- /absolute/path/to/approved-runtime ARGUMENTS
eng project add my-project /work/my-project --runtime team-default
eng run my-project --text '完成一个有验收条件的小任务' # 或 --task task.md
eng status
```

Runtime 行是接口示意，必须替换为已核实且获准的非交互命令；不是预置模型配置，也不验证账号已登录。`project add` 在未声明 `base` 时使用当前检出的分支（detached HEAD 使用精确 SHA），可不创建 `.eng.yaml`；没有验收检查的任务执行后保持 `waiting`，需人工核验并 `eng accept TASK --note ...`。安装、doctor 和示例都不调用模型。命令行里的 `--text` 会被 shell 历史和进程列表记录；敏感任务请用 `--task FILE`。预编译包使用 SQLite 内嵌驱动，不需要数据库服务、Go 或 C 编译器。

## 日常需要的功能

| 需要做的事 | 入口 |
|---|---|
| 登记多个本地仓库与实际检查 | `eng project add` |
| 创建并执行一个任务 | `eng run PROJECT --task FILE` |
| 查看结果、差异、日志和历史执行 | `eng status TASK`、`eng logs TASK` |
| 停止整组本地进程，保留现场 | `eng stop TASK` |
| 观察中断现场；明确发起下一次尝试 | `eng recover`、`eng resume TASK` |
| 接受没有配置检查的完整本地结果 | `eng accept TASK --note TEXT` |
| 按需在同一主机排队 | `eng serve`、`eng submit` |
| 按需创建 GitHub 分支与 PR | `eng publish TASK --repo OWNER/REPO` |
| 离线备份与原路径恢复 | `eng backup`、`eng restore` |

任务配置和每次尝试的输入会冻结；检查记录绑定准确的 result commit。超时、崩溃与不确定的外部操作不会触发自动模型重试。没有 usage 的费用是未知值。`done` 表示约定的本地交付，接受业务需求、合并和设备验证仍由项目决定。

worktree 不是 OS 沙盒。同一账号下的代码能接触该账号可访问的文件和凭据。共享执行账号不应持有 Git 发布凭据；发布可留在成员自己的工作机。需要源码或凭据相互隔离的成员使用各自实例。

## 构建与验证

源码入口是独立 Go module [`tool/`](tool)。在 Linux 上使用 Go 1.26.11、Git 和本机 C 编译器：

```bash
make check
make build
./bin/eng doctor
make release
```

`make release` 输出本机架构的发行包；正式 AMD64 CI 包在 Ubuntu 22.04 上构建。测试使用真实 SQLite、Git、进程和确定性本地命令，覆盖状态竞争、崩溃、停止、恢复、备份与发布回查；不调用模型。通过这些测试不代表模型工程质量、实际设备通过或旧平台生产资格完成。

## 方法与迁移

- [完整操作说明](tool/README.md)：配置、执行、排队、发布、备份与回退。
- [旧平台迁移说明](docs/small-team-migration.md)：按项目切换、保留历史事实、新状态从新任务开始。
- [排查与修复](tool/examples/templates/debug-fix.md)、[功能与变更](tool/examples/templates/feature-change.md)、[审查与发布准备](tool/examples/templates/review-release.md)：随 `eng` 发行包附带，直接复制到项目 Issue 或任务文件，无需其他仓库。
- 已有 ADK、模型 CLI 和项目工具链继续使用，不新增 Skill 系统、账号平台或 Web 前后端。

## 历史平台的范围

根目录的旧 `cmd/`、`internal/`、`go.mod`、`deploy/` 和相应架构/资格材料保留为历史实现与证据参考。新 `tool/` 不导入它们，不连接旧 PostgreSQL，也不双写运行状态。旧服务不是新用户的安装步骤；不再据此扩展日常产品。

旧 [Implementation Status](docs/status/IMPLEMENTATION_STATUS.md) 继续描述旧平台尚未解决的恢复、生产主机、Provider 和设备资格事实。这次重设计没有把这些问题改写成已修复，也没有停止用户主机上的服务或删除历史证据。现存受保护分支的旧 CI 检查继续真实运行，新工具另外由 `Eng local CI` 验证；旧检查的绿色结果不替代新工具验证。

完成真实项目的低风险试用、保存旧执行现场并确认未决外部效果后，再退出原有服务。迁移不要求维持长期双运行或兼容层。
