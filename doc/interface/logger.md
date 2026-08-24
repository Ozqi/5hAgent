# Logger / Worklog

> 由 Claude Fable 5 于 2026-08-23 阅读 `internal/logger/*.go`、`internal/toolevent/toolprint.go` 与 `internal/runtime/worklog.go` 后更新。
> 覆盖范围：日志、工具事件与通用 process worklog。

## 职责

`internal/logger` 只负责文件日志、ANSI 颜色和 `TruncateString`；`internal/toolevent` 负责工具事件结构与格式化；`runtime/processWorkLog` 负责把通用 process 执行流写入项目 `.walle`。

## 文件

| 文件 | 作用 |
| --- | --- |
| [logger.go](../../internal/logger/logger.go) | 带标签日志、普通 log 文件 |
| [toolprint.go](../../internal/toolevent/toolprint.go) | 工具调用/结果/错误事件与文本格式 |
| [color.go](../../internal/logger/color.go) | ANSI 颜色 |
| [worklog.go](../../internal/runtime/worklog.go) | 通用 process 工作日志 |

## ToolEvent

TUI 和 worklog 都消费 `toolevent.ToolEvent`；事件接收器由每个 `Agent` 的 `SetToolEventSink` 管理：

```text
Kind: call/result/error
Name: full tool name
Args: raw JSON args
Result/Error: structured text
Text: formatted display text
```

## Worklog

位置：

```text
<project>/.walle/agents/<process>/logs/<timestamp>-<process>.md
```

段落：

- `# Process Work Log`
- `## Assistant <RFC3339>`
- `## Tool Event <RFC3339>`
- `## Status <RFC3339>`

## 边界

- logger 不处理工具事件，也不决定 TUI 布局。
- worklog 是执行证据，不是可恢复 session。
