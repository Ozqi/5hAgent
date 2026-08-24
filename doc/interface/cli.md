# TUI

> 由 Claude Fable 5 于 2026-08-23 阅读 `cmd/walle/*.go`、`internal/tui/*.go`、`internal/runtime/daemon_session.go` 与 `internal/systemd/control.go` 后更新。
> 覆盖范围：默认 TUI、daemon attach、slash command 与状态渲染。

## 职责

`internal/tui` 是独立 Bubble Tea 客户端包，只负责输入、渲染和事件转发。默认 `walle` 自动启动或复用 daemon，再通过 Unix Socket attach 其交互 Agent；daemon 不依赖具体 TUI 实现。

## 关键文件

| 文件 | 作用 |
| --- | --- |
| [app.go](../../internal/tui/app.go) | `AppModel`、viewport、事件渲染 |
| [commands.go](../../internal/tui/commands.go) | 本地与 attached 输入处理 |
| [remote.go](../../internal/tui/remote.go) | daemon attach 客户端入口 |
| [markdown.go](../../internal/tui/markdown.go) | Markdown 终端渲染 |
| [ui.go](../../internal/cli/ui.go) | 非 TUI 错误输出 |

## 布局

```text
history viewport
runtime status       # model/state/turn/tools
slash hints          # 只在输入 / 时出现
input bar
footer metadata      # path/git/msg count，固定存在
bottom spacer        # 空白占位，不显示 busy spinner
```

## Slash 命令

| 命令 | 行为 |
| --- | --- |
| `/provider [name]` | 选择 Provider 或完成认证 |
| `/model [provider/model]` | 查看或切换当前模型 |
| `/skill list|get|reload` | 调 `commands.HandleSkill` |
| `/compress` | 调 `commands.HandleCompress` |
| `/mcp` | 管理 MCP 配置 |
| `/session` | 查看当前 session；本地嵌入模式还可 new/list/切换 |
| `/stop` | 取消当前 Agent run |
| `/detach` | 退出 attached TUI，不停止 Agent |

任务管理命令不固定进 Runtime；需要时由 Skill、MCP 或外置动态工具提供。

## 运行状态

- `busy/currentStatus/spinnerFrame` 驱动顶部状态行。
- `runCancel` 保存当前 run 的 cancel func；`/stop` 调用它。
- 迟到 token 在 `busy=false` 后被忽略。
- footer 显示路径但不显示 `dir` 字样。
- git 主仓库显示 `git <branch>`；linked worktree 显示 `worktree <branch>`。
- 消息时间显示在首行右侧。

## 工具事件

```text
ToolEvent(call)   -> running hint
ToolEvent(result) -> 原地更新为 done
ToolEvent(error)  -> 原地更新为 error
```

工具标题使用蓝色工具名和蓝色状态符号；error 保留红色。

## 历史恢复

`loadHistoryEntries` 会把 assistant `tool_calls` 与后续 `schema.Tool` 结果合并成压缩工具提示，避免 `walle -c` 展开完整工具输出。

## 验证

TUI 视觉改动以真实 tmux 画面为准。不要每个小改动都启动/重启；一组相关改动完成后，再用当前 `walle debug` 或临时 tmux session 验收。
