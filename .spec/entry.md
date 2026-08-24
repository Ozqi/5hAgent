# Entry / UI Spec

## 职责

入口层负责把用户操作转成 daemon control 请求，并展示结果。它包含 CLI 命令、基础文本输出和 Bubble Tea TUI。

## 覆盖范围

| 路径 | 职责 |
| --- | --- |
| `cmd/walle/main.go` | Cobra root、全局 flag、固定 `daemon` 子命令。 |
| `cmd/walle/interactive_command.go` | 默认 TUI 的 daemon 启动/复用逻辑。 |
| `cmd/walle/process_commands.go` | `ps`、`attach` 命令和 completion。 |
| `internal/cli/ui.go` | 非 TUI 错误输出。 |
| `internal/tui/app.go` | `AppModel`、Bubble Tea `Update/View`、状态聚合。 |
| `internal/tui/commands.go` | 本地/attached 输入、`/stop`、`/model`。 |
| `internal/tui/remote.go` | `ProcessClient` 到 TUI 的适配。 |
| `internal/tui/render.go`、`tool_render.go` | 布局和工具事件展示。 |

## CLI 命令契约

| 命令 | 行为 |
| --- | --- |
| `walle` | 复用或启动 `walle daemon`，再 attach `interactive` TUI。 |
| `walle daemon` | 固定托管一个可 attach 的交互 Agent。 |
| `walle ps` | 查询 supervisor socket，展示 `ProcessSnapshot.Name`。 |
| `walle attach <id>` | interactive 进 TUI；通用 process 可跟随 worklog。 |

不存在 `walle run`、`daemon --poll` 或 `daemon --interactive`。

全局 flag：`--debug`、`--session`、`--continue/-c`、`--llm-format`、`--llm-model`、`--model/-m`。`interactiveDaemonArgs` 必须把它们传给后台 daemon，最后追加 `daemon`。

## 默认 TUI 链路

1. `runTUI` 调 `startInteractiveClient`。
2. 先尝试 `AttachProcess(runDir, "interactive")`。
3. attach 失败时启动当前二进制的 `daemon` 子命令。
4. stdout/stderr 追加到 `~/.walle/run/interactive.log`。
5. 最多等待 30 秒轮询 `supervisor.sock`，成功后进入 `LaunchAttachedTUI`。

## Slash commands

| 输入 | 行为 |
| --- | --- |
| 普通文本 | attached 模式发给 daemon；本地嵌入模式调用 `Agent.RunStream`。 |
| `/stop` | 取消当前 run。 |
| `/detach` | 退出 TUI，不停止 daemon Agent。 |
| `/skill` | 处理 skill list/get/reload。 |
| `/compress` | 压缩当前 context。 |
| `/mcp` | 管理 MCP 配置。 |
| `/session` | 查询或切换 session，具体能力取决于运行模式。 |
| `/model`、`/provider` | 选择 provider/model。 |

不存在固定 `/task` 或 `/run`。任务管理由外部能力按需提供。

## Remote event 映射

`ProcessEvent.Type` 支持 `user`、`assistant`、`thinking`、`tool`、`system`、`error`、`done`、`state`、`picker`、`model`。连接断开时 TUI 进入 `disconnected`。

## 状态边界

- 默认 TUI 不直接创建 Runtime；交互 Runtime 由 daemon 持有。
- `AppModel` 保存当前帧 UI 状态；远端状态来自 attach snapshot 和 `ProcessEvent`。
- `internal/cli` 不保存状态。
- `interactive.log` 只记录 daemon 启动 stdout/stderr，不是业务状态源。
- `followFile` 只观察通用 process worklog，不控制 Agent 生命周期。

## 禁止

- 在入口层实现 Agent、工具、任务管理、session 或 provider 业务。
- 在 `View` 中做网络、磁盘扫描、模型调用或 shell 执行。
- 把 `detach` 当作停止远端 Agent。
- 恢复已删除的 TaskList 命令或 daemon flag。
