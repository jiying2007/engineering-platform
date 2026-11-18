# eng：个人和小团队工程任务工具

`eng` 在本机 Git 仓库上准备独立工作区，调用你已配置并获准使用的命令或模型 CLI，运行项目检查，保存日志、差异与结果。一个 `eng` 二进制内置 SQLite，无需数据库服务。Issue、PR/MR 和责任人的接受继续留在原项目。

当前执行主机支持 Ubuntu Linux，按可信用户和可信项目设计。使用预编译包需要 Git 和项目自己的 Runtime/工具链，不需要 Go、GCC 或 SQLite 命令行。编译发行包才需要 Go 1.26.11 与本机 C 编译器。

## 安装已有发行包

从对应提交的 Actions artifact 或固定版本发布包取得 `eng-linux-amd64.tgz` 与同名 `.sha256` 文件。以下以 AMD64 包为例；ARM64 需要在对应构建机生成的 `eng-linux-arm64` 包，以实际提供的构建为准。`BUILD.txt` 记录来源 commit、工作树是否修改及构建环境，`eng version` 显示构建标识。

```bash
sha256sum -c eng-linux-amd64.tgz.sha256
tar -xzf eng-linux-amd64.tgz
cd eng-linux-amd64
./install.sh
export PATH="$HOME/.local/bin:$PATH"
eng doctor
```

安装器只校验并安装当前包，不联网、不改 Runtime 登录、不调用模型。可用 `./install.sh --bin-dir /path/to/bin` 选择安装位置。已有 `eng` 时先按下文停止与备份，再使用 `--replace`。

首次 `doctor` 尚无登记时报告 `status: installation_checked` 和 `installation_ready`，`ready` 保持 false。登记项目后，它核对 Git 基线、Runtime 与检查命令的可执行文件、数据目录权限与写入/fsync，并返回磁盘可用字节和细分检查数量。它不执行 Runtime、项目检查或版本探测；`ready: true` 仅表示已配置的本地前提检查通过。

正式发行包在 Ubuntu 22.04 上构建，以限制 glibc 版本要求；自行在更新系统构建的包不能据此声称兼容较旧 Ubuntu。SQLite 已编入二进制，不依赖独立数据库服务。

## 先跑一个完全不调用模型的例子

以下命令在临时目录创建一个独立 Git 仓库，原项目不会被改动。保持当前目录为解压后的发行包目录。

```bash
eng_package="$PWD"
eng_demo="$(mktemp -d)"
git init -b main "$eng_demo/repo"
printf 'eng local example\n' > "$eng_demo/repo/README.md"
cp "$eng_package/examples/project-fixture.yaml" "$eng_demo/repo/.eng.yaml"
git -C "$eng_demo/repo" add README.md .eng.yaml
git -C "$eng_demo/repo" -c user.name='eng example' \
  -c user.email='eng-example@localhost' commit -m 'Initialize local example'

eng --data-dir "$eng_demo/state" runtime add fixture \
  --no-model --stdin task --timeout 1m -- \
  bash "$eng_package/examples/fixture-runtime.sh"
eng --data-dir "$eng_demo/state" project add example "$eng_demo/repo" \
  --config .eng.yaml
eng --data-dir "$eng_demo/state" doctor
eng --data-dir "$eng_demo/state" run example --task "$eng_package/examples/task.md"
eng --data-dir "$eng_demo/state" status
```

示例 Runtime 只生成两个文本文件；项目检查验证产物存在。结果应包括任务分支、独立 worktree、实际命令与退出码、diff 和产物摘要。它证明本地执行与 Git 路径可运行，不能证明模型工程质量、真实固件功能或设备验收。

临时目录保留用于查看现场。后续命令继续使用相同 `--data-dir`，否则会查看另一个独立实例。

## 接入实际项目

### 1. 登记已获准的 Runtime

先确认具体 Runtime 支持的非交互启动、任务输入与停止方式。`eng` 按 `argv` 启动现有程序，不自动选择模型、注册账号或切换费用来源。

```text
eng runtime add team-default --model --stdin task --timeout 45m -- /absolute/path/to/approved-runtime ARGUMENTS
```

将上面的可执行路径和参数替换为已验证的实际命令；这是接口示意，不是某个模型 CLI 的预置配置。必须显式指定 `--model` 或 `--no-model`。`--no-model` 用于确定不调用模型的本地命令，不会检查程序内部是否偷偷调用模型。`--stdin task` 传入任务正文；确实不需要标准输入的命令可用 `--stdin none`。

凭据沿用 Runtime 或操作系统的私有配置，不放进项目 YAML、任务文本或命令参数。已有 Codex/ChatGPT Work 共享额度限制仍适用；登记 Runtime 不扩大执行授权。

### 2. 登记项目与检查

把发行包的 `examples/project.yaml` 复制为项目 `.eng.yaml`，改成项目实际存在的命令：

```yaml
base: main
runtime: team-default
checks:
  - name: build
    argv: ["bash", "scripts/build.sh"]
    timeout: 30m
  - name: test
    argv: ["bash", "scripts/test.sh"]
    timeout: 15m
artifacts: ["out/*.elf", "out/*.map"]
```

```text
eng project add pcr02-linux /work/pcr02 --config .eng.yaml
```

名称和本机仓库路径由登记命令提供。相对 `--config` 路径按项目目录解析；省略该选项时自动读取项目根目录中存在的 `.eng.yaml`。当前 YAML 支持 `root`、`base`、`runtime`、`checks`、`artifacts`，未知字段和多份 YAML 文档会报错；不填写草案中的 `version` 或 `name` 字段。登记时明确传入的 `PATH` 决定本机仓库位置，覆盖 YAML 中的 `root`；可共享的项目文件通常省略 `root`。未指定 `base` 时使用 `main`；check 未指定 timeout 时为 10 分钟。时限必须为正且不超过 24 小时。

每个 artifact pattern 都是所需产物，使用任务工作区内的相对路径；只填写任务确实应产生的文件。未跟踪且匹配这些规则的产物复制到 attempt 目录并记录摘要，不自动加入 Git。检查脚本可以执行代码和访问网络，要作为已获准项目代码对待。一次任务会冻结使用的配置，模型在执行中改动 `.eng.yaml` 不会扩大当前任务的动作范围。

项目登记保存本机配置副本。修改 `.eng.yaml` 后，用同一 `project add` 命令加 `--replace` 更新登记；修改 Runtime 也需重新 `runtime add` 并加 `--replace`。更新只影响之后新建的任务，既有任务及其 resume 继续使用原配置快照。

任务从本地 Git 基线准备 worktree，不自动 fetch，也不复制原开发目录中未提交的修改。所需源码和检查脚本应已存在于选定基线。构建输出与缓存按项目实际规则写入 `.gitignore`；检查若改动待验收源码，原 commit 的检查结果不能替新内容作证，任务会保留现场等待处理。

### 3. 执行和检查结果

```text
eng run pcr02-linux --task issue-123.md
eng status
eng status TASK_ID
eng logs TASK_ID
```

`TASK_ID` 使用运行或状态输出的真实编号。状态和结构化命令结果采用 JSON，`logs` 返回日志文本。`logs TASK_ID --stderr` 查看 Runtime 标准错误，`--attempt N` 选择历史执行，`--tail 200` 选择末尾行数；检查日志和产物位于状态中标明的 attempt 目录。全局选项 `--data-dir /local/path` 放在子命令之前。

前台 `run/resume` 只有完成本地交付才返回退出码 `0`；失败、取消或需要人工处理时保留 JSON 结果并返回非零。serve 模式的 `run/submit/resume` 返回 `0` 只表示已入队，需要继续用 `status` 查看执行结果。`publish` 尚未确认远端结果也返回非零。日志和 `summary.md/result.json` 是当次执行快照，后续接受、恢复和发布的当前事实以 `eng status` 为准。

| 状态 | 含义 |
|---|---|
| `queued` | 已登记，尚未发起本次执行 |
| `running` | 正在准备、执行、检查或整理交付 |
| `waiting` | 缺少输入、需接受结果或存在尚未确认的事实；查看 reason |
| `done` | 本次约定范围的本地交付完成，不表示已经合并或发布 |
| `failed` | 本次执行已确认失败，保留原因和现场 |
| `cancelled` | 本机执行已确认停止，或排队任务未启动 |

配置了检查时，查看每项真实命令、退出码和结果；不能只看模型退出码。没有检查的文档或分析任务保持 `waiting`，阅读 diff 和输出后可用 `eng accept TASK_ID --note '实际接受的范围和理由'` 明确接受。人工接受不会伪造测试记录，也不能覆盖必需检查失败、存活进程或未知结果。

当前成本字段只报告实际可获取的信息。没有 usage 时显示未知，不把未知当零费用。超时或本机取消后，远端模型可能已经执行或计费；没有自动成本封顶或无限修复循环。

## 停止、恢复与排队

```text
eng stop TASK_ID
eng recover
eng resume TASK_ID
eng resume TASK_ID --task followup.md
```

先查看 status/logs，解决缺少材料或检查失败，再显式 resume。恢复保留原工作区和历史 attempt；重新调用 Runtime 会建立新 attempt，不声称等同于模型厂商的会话 continue。恢复检查尚不能确认旧进程结束时，任务保持待处理，不通过手改数据库或删除锁文件绕过。

没有 `serve` 时，`run` 持有本次监督权，终端断开或进程收到取消信号时会按停止流程处理。需要断开提交端后继续执行时，在受管理的终端或服务中运行同一程序的 `serve`。例如在执行主机的持久终端中启动：

```text
eng serve
```

通过另一个终端或现有 SSH 会话提交与查看：

```text
eng submit pcr02-linux --task issue-123.md
eng status
```

这些命令使用同一 data-dir。`serve` 已运行时，`run` 也只入队，由该 `serve` 执行；提交端退出不会停止任务。`serve` 自身退出会停止当前本机进程组，因此不要把它放在随 SSH 断开而退出的裸终端中。

有 `serve` 时，日常命令通过同账号的私有本地 socket 交给执行端处理；Runtime 使用 `serve` 启动时的 PATH 和环境。需要切换工具环境时使用明确的可执行路径，或先完成模式切换再重启 `serve`。客户端不会转发环境变量或凭据；连接建立后失联时先用 `status` 核对，不自动重发任务。

前台和 serve 不得同时拥有调度权。默认串行执行，每个 Git common-dir 同时只有一个活动任务，项目别名和另一个 data-dir 不能绕过同一仓库的互斥。切换模式前停止接收新任务，等待当前任务结束或明确停止，再释放拥有权。

独立 worktree 保留原开发目录，但共享同一 Git 仓库的部分数据；它不是 OS 沙盒。共享使用仅限可信成员。需要源码或凭据彼此隔离时使用各自实例。第一版不提供多主机调度、跨仓原子合并或自动合并。

## 本地交付与发布

任务结果保留自己的分支、result commit、diff 与检查记录。可以继续用现有 Git 和 PR/MR 流程评审、推送与合并。发布不需要成为使用本地任务工具的前置条件；设备刷写、生产 OTA、OTP/Fuse 按具体任务授权处理。

需要 GitHub 便利入口时，可显式使用已有 Git 和 `gh` 凭据：

```text
eng publish TASK_ID --repo OWNER/REPO
```

此命令需要先停止 `serve` 并确认活动执行结束。任务必须为 `done`，worktree 的 HEAD 和内容必须仍匹配记录的结果，`origin` 必须对应指定仓库，基线必须是可建立 PR 的分支。工具先记录发布意图，再推送任务分支、创建或识别 PR，并核对远端仓库、分支和准确 commit；不会合并 PR。

发布结果未知时保留待核对记录。再次调用先读取远端；一次未查到目标不能授权盲目重发。恢复旧备份后同样先核对实际外部事实，不依据旧快照自动补做 push 或 PR。仍无法确认时按现有 GitHub/Git 流程人工处理。

实际项目代码由现有 ADK 和已获准 Runtime 处理。三类任务模板已随发行包内置在 `examples/templates/`： [排查与修复](examples/templates/debug-fix.md)、[功能与变更](examples/templates/feature-change.md)、[审查与发布准备](examples/templates/review-release.md)。复制为自己的 `task.md` 后由 `eng run PROJECT --task task.md` 执行，日常不依赖另外的领域仓库。

模板仅规定清楚的输入和交付记录，不代表 Runtime 能自动验证所有内容；设备验证、发布权限和项目验收仍以实际项目为准。

## 数据、备份和升级

数据目录只能放执行主机的本地磁盘，SQLite 不通过 NFS/SMB 共享。保留数据库、登记配置、任务工作区、日志和产物；状态以数据库为准，日志文件不作为另一套可回放任务系统。

默认路径是 `$XDG_DATA_HOME/eng`，未设置时为 `$HOME/.local/share/eng`；可用 `ENG_DATA_DIR` 或全局 `--data-dir` 指定，命令行优先。备份和恢复必须使用原实例的路径。

```text
eng --data-dir /local/eng-state backup /local/backups/eng-before-upgrade
```

这是离线备份。先停止接收任务，确认前台执行已结束并停止 `serve`；备份入口取得调度权并核对本机进程已停止，仍有活动执行时拒绝备份。目标必须是尚不存在的新目录。

备份包含 SQLite 一致性快照、项目与 Runtime 登记，以及任务工作区、日志和产物；清单记录路径、类型和摘要。符号链接只复制链接本身，不跟随目标。不能直接复制正在使用的 `eng.db` 主文件而忽略 WAL。

此备份用于**同机、原路径恢复**，不是仓库迁移工具。worktree 中的 `.git` 指针原样保留，仍依赖原 Git 主仓库；主仓库、外部工具链和 Runtime 凭据需要分别备份。

恢复前确认原实例和任务进程全部停止，将现有 data-dir 移至独立保留位置，再使用匹配版本恢复到**原路径**。恢复命令拒绝覆盖已经存在的目标目录：

```text
eng --data-dir /local/eng-state restore /local/backups/eng-before-upgrade
eng --data-dir /local/eng-state recover
eng --data-dir /local/eng-state status
```

恢复会校验备份清单；原队列转为待核对，不自动回放。先检查原仓库与 worktree 引用、任务日志以及备份后实际发生的调用和外部操作，再对确实可以继续的任务显式 `resume`。恢复后的 `done` 只保留备份时的本地交付事实，不能替代当前远端或设备状态。

升级时停止接收任务，确认活动进程结束，备份匹配的数据后替换二进制。新版本已执行过任务时优先前向修复；回退前保留新数据与日志，核对升级后真实发生的模型调用和外部操作。恢复旧备份不表示这些事实不存在，旧二进制不能直接读取不兼容的新 schema。

旧多服务系统的切换、备份和历史保留见[迁移说明](../docs/small-team-migration.md)。该步骤需要在实际旧主机完成，本仓安装脚本不会替你停止未知服务。

## 从源码构建

新工具是 `tool/` 下的独立 Go module，不依赖仓库根目录的旧服务模块。

```bash
cd tool
go mod download
go test ./...
./scripts/build-release.sh
./scripts/smoke-release.sh dist/eng-linux-amd64.tgz
```

产物为 `tool/dist/eng-linux-ARCH.tgz` 和校验文件。可用 `ENG_GO_BIN=/path/to/go ./scripts/build-release.sh /path/to/output` 指定编译器和产物目录。脚本原生构建当前 Linux 架构，CGO 在构建时开启；用户安装包不需要 GCC。

将烟测命令中的包名替换为实际构建的架构。`smoke-release.sh` 接收已有包，使用 Python 3 解析结果，可直接用于 CI；它校验安装、拒绝损坏包并运行真实 Git 的无模型例子，不构建、不联网，结束时清理临时文件，失败时打印阶段和诊断。

发布前应在最低支持的 Ubuntu 22.04 构建并实际运行安装与无模型示例。无模型故障测试和真实 Git 测试只验证工具行为；模型质量、目标工具链、设备和团队实际收益需要对应真实任务的结果。
