# Tools

> 由 Claude Fable 5 于 2026-08-23 阅读 `internal/tools/*.go`、`internal/toolmeta/toolmeta.go` 与 `internal/agent/tool_use.go` 后更新。
> 覆盖范围：内置工具、Registry、模型绑定与 MCP 扩展边界。

## 职责

`internal/tools` 提供 LLM 可调用工具；`internal/agent/tool_use.go` 负责执行工具调用。

## 注册

```text
Registry.Init(skillMgr)
  -> base.*
  -> skill.skill
RegisterContextTool(model, promptDir)
  -> context.context
```

默认启动不注册 `mcp.*`。

## 当前工具

| 工具 | 读写 | 说明 |
| --- | --- | --- |
| `base.read_file` | 读 | offset/limit 读文件 |
| `base.read_md` | 读写 | 列标题树，读取、替换或删除标题 section |
| `base.write_file` | 写 | 创建/覆盖文件 |
| `base.edit` | 写 | 精确字符串替换 |
| `base.glob` | 读 | glob 文件匹配 |
| `base.grep` | 读 | 正则文本搜索 |
| `base.list_dir` | 读 | 目录列表 |
| `base.exec_shell` | 写 | 执行 shell |
| `skill.skill` | 读 | 查看 skill |
| `context.context` | 混合 | inspect/pin/edit/audit/compress |

任务管理不属于固定工具表；需要时由 Skill、MCP 或外置动态工具提供。

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
3. 通过 Registry schema 输出和真实工具调用确认 schema 与 meta 一致。
