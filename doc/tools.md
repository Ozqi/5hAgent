# Tools - 工具系统

```text
internal/tools/*.go
  -> registry.go 统一注册
internal/agent/tool_use.go
  -> 只读工具并发
  -> 写工具串行
```

## 位置

- `internal/tools/*.go`
- `internal/tools/registry.go`
- `internal/agent/tool_use.go`

## 当前已注册工具

| 工具名 | 文件 | 类型 | 说明 |
| --- | --- | --- | --- |
| `base.read_file` | `read_file.go` | 读 | 读取文件内容，支持 offset/limit |
| `base.write_file` | `write_file.go` | 写 | 创建或覆盖文件，自动创建父目录 |
| `base.edit` | `edit.go` | 写 | 精确字符串替换 |
| `base.glob` | `glob.go` | 读 | 文件模式匹配 |
| `base.grep` | `grep.go` | 读 | 文本搜索 |
| `base.list_dir` | `list_dir.go` | 读 | 列目录 |
| `base.exec_shell` | `exec_shell.go` | 写 | 执行 shell 命令 |
| `task.task` | `task_tool.go` | 混合 | 统一任务管理入口，按 `action` 分流 |
| `skill.skill` | `skill_tool.go` | 写 | 启用或禁用 skill |
| `mcp.<server>.<tool>` | `mcp_tool.go` | 取决于远端 | 外部 MCP server 提供的远端工具包装 |

当前真实注册结果见 [registry.go](../internal/tools/registry.go)。

## 关键文件

### `registry.go`

- `InitRegistry(taskList, skillMgr)` 负责构造并注册所有工具。
- `GetAllTools()` 返回当前工具切片。
- `GetToolByName(name)` 提供按名查找。

### `task_tool.go`

- 统一暴露 `task.task` 工具。
- `action` 支持 `create`、`update`、`get`、`list`、`delete`。
- 底层调用 `internal/agent/tasklist.go`。

### `skill_tool.go`

- 暴露 `skill.skill` 工具。
- `action` 支持 `enable`、`disable`。
- 底层调用 `internal/skill/skill.go`。

### `mcp_tool.go`

- 暴露统一的远端 MCP 工具包装。
- 完整名称格式为 `mcp.<server>.<tool>`。
- 底层通过 `internal/mcp.Client` 调用外部 server。
- 当前仓库只实现 foundation，还没有接入真实 stdio MCP 协议。

## 并发执行策略

工具调度在 [tool_use.go](../internal/agent/tool_use.go)。

- 只读工具走并发执行。
- 写工具走串行执行。
- 结果会按原始调用顺序写回上下文。

当前只读能力由 `toolmeta` 元信息 + `task.task` 的 `action` 共同决定。当前只读工具包括：

- `base.read_file`
- `base.glob`
- `base.grep`
- `base.list_dir`

另外，统一 `task.task` 工具会在运行时进一步按参数分类：

- `{"action":"get"}` -> 只读并发
- `{"action":"list"}` -> 只读并发
- `create` / `update` / `delete` -> 串行写入

## 当前实现备注

这里现在只保留新命名：

- 本地基础工具统一使用 `base.*`
- 任务工具统一使用 `task.task`
- skill 工具统一使用 `skill.skill`
- MCP 工具使用 `mcp.<server>.<tool>`

## 工具返回约定

- `base.read_file`、`base.write_file`、`base.edit`、`base.exec_shell` 等工具多使用 JSON 文本作为结果体。
- 读写类工具优先实现 `tool.EnhancedInvokableTool`。
- `invokeTool()` 会优先走 `EnhancedInvokableTool`，再降级到 `InvokableTool`。

## 添加新工具

1. 在 `internal/tools/` 新增实现文件。
2. 定义输入输出结构体和 JSON tag。
3. 实现 `NewXxxTool()`。
4. 在 `InitRegistry(...)` 中注册。
5. 如果工具确实无副作用，在注册时更新对应 `toolmeta.Meta.ReadOnly`。
6. 更新本文档和 `README.md`。

对于 MCP 工具，当前入口是 `RegisterMCPTools(serverName, client, specs)`，由外部 server discovery 代码先拿到 tool 列表，再统一注册。

## 相关代码

- [registry.go](../internal/tools/registry.go)
- [task_tool.go](../internal/tools/task_tool.go)
- [skill_tool.go](../internal/tools/skill_tool.go)
- [mcp_tool.go](../internal/tools/mcp_tool.go)
- [tool_use.go](../internal/agent/tool_use.go)
- [tasklist.go](../internal/agent/tasklist.go)
