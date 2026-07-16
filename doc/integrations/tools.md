# Tools

## 职责

`internal/tools` 提供 LLM 可调用工具；`internal/agent/tool_use.go` 负责执行工具调用。

## 注册

```text
Registry.Init(taskList, skillMgr)
  -> base.*
  -> task.task
  -> skill.skill
RegisterContextTool(model, promptDir)
  -> context.context
```

默认启动不注册 `mcp.*`。

## 当前工具

| 工具 | 读写 | 说明 |
| --- | --- | --- |
| `base.read_file` | 读 | offset/limit 读文件 |
| `base.read_md` | 读 | 列 Markdown 标题树、读取标题 section |
| `base.write_file` | 写 | 创建/覆盖文件 |
| `base.edit` | 写 | 精确字符串替换 |
| `base.glob` | 读 | glob 文件匹配 |
| `base.grep` | 读 | 正则文本搜索 |
| `base.list_dir` | 读 | 目录列表 |
| `base.exec_shell` | 写 | 执行 shell |
| `task.task` | 混合 | task CRUD |
| `skill.skill` | 读 | 查看 skill |
| `context.context` | 混合 | inspect/pin/audit/compress |

## 执行

```text
toolCollector -> toolQueue -> exeToolCall -> invokeTool -> addToolResult
```

当前工具在单 worker 内顺序执行；stream 读取可与工具执行重叠。

## 错误格式

工具失败写入 tool message，包含：

- 原始错误
- 模型传入参数
- 简短修正建议

## 添加工具

1. 在 `internal/tools` 实现 typed input/output。
2. 在 `registry.go` 注册 full name 和 metadata。
3. 加入能力测试，确认 schema 和 meta 一致。
