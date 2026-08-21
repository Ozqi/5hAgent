# Logger / Worklog

## 职责

`internal/logger` 只负责文件日志、ANSI 颜色和 `TruncateString`；`internal/toolevent` 负责工具事件格式化和 sink；`runtime/headlessWorkLog` 负责把 headless/process 执行流写入项目 `.5hagent`。

## 文件

| 文件 | 作用 |
| --- | --- |
| [logger.go](../../internal/logger/logger.go) | 带标签日志、普通 log 文件 |
| [toolprint.go](../../internal/toolevent/toolprint.go) | 工具调用/结果/错误文本格式与 sink |
| [color.go](../../internal/logger/color.go) | ANSI 颜色 |
| [worklog.go](../../internal/runtime/worklog.go) | headless/process 工作日志 |

## ToolEvent

TUI 和 worklog 都消费 `toolevent.ToolEvent`；sink 通过 `toolevent.SetToolEventSink` / `toolevent.PushToolEventSink` 管理：

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
<project>/.5hagent/agents/<agent>/logs/<timestamp>-<task>.md
```

段落：

- `# Headless Work Log`
- `## Assistant <RFC3339>`
- `## Tool Event <RFC3339>`
- `## Status <RFC3339>`

## 边界

- logger 不处理工具事件，也不决定 TUI 布局。
- worklog 是执行证据，不是可恢复 session。
