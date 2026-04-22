# Tools - 工具系统

```text
internal/tools/*.go
  -> registry.go 统一注册
internal/agent/tool_executor.go
  -> 只读工具并发
  -> 写工具串行
```

## 位置

- `internal/tools/*.go`
- `internal/tools/registry.go`
- `internal/agent/tool_executor.go`

## 当前已注册工具

| 工具名 | 文件 | 类型 | 说明 |
| --- | --- | --- | --- |
| `read_file` | `read_file.go` | 读 | 读取文件内容，支持 offset/limit |
| `write_file` | `write_file.go` | 写 | 创建或覆盖文件，自动创建父目录 |
| `edit` | `edit.go` | 写 | 精确字符串替换 |
| `glob` | `glob.go` | 读 | 文件模式匹配 |
| `grep` | `grep.go` | 读 | 文本搜索 |
| `list_dir` | `list_dir.go` | 读 | 列目录 |
| `exec_shell` | `exec_shell.go` | 写 | 执行 shell 命令 |
| `task` | `task_tool.go` | 混合 | 统一任务管理入口，按 `action` 分流 |
| `skill` | `skill_tool.go` | 写 | 启用或禁用 skill |

当前真实注册结果见 [registry.go](../internal/tools/registry.go)。

## 关键文件

### `registry.go`

- `InitRegistry(taskList, skillMgr)` 负责构造并注册所有工具。
- `GetAllTools()` 返回当前工具切片。
- `GetToolByName(name)` 提供按名查找。

### `task_tool.go`

- 统一暴露 `task` 工具。
- `action` 支持 `create`、`update`、`get`、`list`、`delete`。
- 底层调用 `internal/agent/tasklist.go`。

### `skill_tool.go`

- 暴露 `skill` 工具。
- `action` 支持 `enable`、`disable`。
- 底层调用 `internal/skill/skill.go`。

## 并发执行策略

工具调度在 [tool_executor.go](../internal/agent/tool_executor.go)。

- 只读工具走并发执行。
- 写工具走串行执行。
- 结果会按原始调用顺序写回上下文。

当前 `readOnlyTools` 名单包含：

- `read_file`
- `glob`
- `grep`
- `list_dir`
- `task_get`
- `task_list`

另外，统一 `task` 工具会在运行时进一步按参数分类：

- `{"action":"get"}` -> 只读并发
- `{"action":"list"}` -> 只读并发
- `create` / `update` / `delete` -> 串行写入

## 当前实现备注

这里有一个兼容性点：

- `tool_executor.go` 仍保留旧的 `task_get` / `task_list` 名称
- 同时也会对统一 `task` 工具解析 `action`
- 这样旧名称与新统一入口都能被正确分类

## 工具返回约定

- `read_file`、`write_file`、`edit`、`exec_shell` 等工具多使用 JSON 文本作为结果体。
- 读写类工具优先实现 `tool.EnhancedInvokableTool`。
- `invokeTool()` 会优先走 `EnhancedInvokableTool`，再降级到 `InvokableTool`。

## 添加新工具

1. 在 `internal/tools/` 新增实现文件。
2. 定义输入输出结构体和 JSON tag。
3. 实现 `NewXxxTool()`。
4. 在 `InitRegistry(...)` 中注册。
5. 如果工具确实无副作用，再更新 `internal/agent/tool_executor.go` 的只读名单。
6. 更新本文档和 `README.md`。

## 相关代码

- [registry.go](../internal/tools/registry.go)
- [task_tool.go](../internal/tools/task_tool.go)
- [skill_tool.go](../internal/tools/skill_tool.go)
- [tool_executor.go](../internal/agent/tool_executor.go)
- [tasklist.go](../internal/agent/tasklist.go)
