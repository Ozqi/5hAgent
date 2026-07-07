# TUI

## 职责

`internal/cli/tui.go` 是 Bubble Tea 终端界面。它只管输入、渲染和事件转发；Agent 执行仍在 `internal/agent`。

## 关键文件

| 文件 | 作用 |
| --- | --- |
| [tui.go](../../internal/cli/tui.go) | `AppModel`、输入、slash、viewport、工具事件 |
| [markdown_stream.go](../../internal/cli/markdown_stream.go) | Markdown 终端渲染 |
| [ui.go](../../internal/cli/ui.go) | 非 TUI 错误输出 |

## 布局

```text
history viewport
runtime status       # model/state/turn/tools
slash hints          # 只在输入 / 时出现
input bar
footer metadata      # path/tasks/msg count，固定存在
bottom spacer        # 空白占位，不显示 busy spinner
```

## Slash 命令

| 命令 | 行为 |
| --- | --- |
| `/task` | 调 `commands.HandleTask` |
| `/skill` | 调 `commands.HandleSkill` |
| `/compress` | 调 `commands.HandleCompress` |
| `/mcp` | 管理 MCP 配置 |
| `/session` | new/list/switch/save/drop |
| `/run` | 执行 task.md 中可运行任务 |
| `/model` | 切换 provider/model |
| `/stop` | cancel 当前 Agent run |

## 运行状态

- `busy/currentStatus/spinnerFrame` 驱动顶部状态行。
- `runCancel` 保存当前 run 的 cancel func；`/stop` 调用它。
- 迟到 token 在 `busy=false` 后被忽略。
- footer 显示路径但不显示 `dir` 字样。
- 消息时间显示在首行右侧。

## 工具事件

```text
ToolEvent(call)   -> running hint
ToolEvent(result) -> 原地更新为 done
ToolEvent(error)  -> 原地更新为 error
```

工具标题使用蓝色工具名和蓝色状态符号；error 保留红色。

## 历史恢复

`loadHistoryEntries` 会把 assistant `tool_calls` 与后续 `schema.Tool` 结果合并成压缩工具提示，避免 `5hagent -c` 展开完整工具输出。

## 验证

```bash
go test ./internal/cli
```

TUI 视觉改动必须用 tmux 抓屏验证。
