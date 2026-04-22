# Agent - 核心流程

```text
cmd/5hagent/main.go
  -> agent.NewAgent(...)
  -> tools.InitRegistry(...)
  -> Agent.RunStream(...)
     -> injectSkills()
     -> model.Stream()/Generate()
     -> exeTools()
     -> task/context updates
```

## 位置

- `internal/agent/agent.go`
- `internal/agent/tool_executor.go`
- `internal/agent/tasklist.go`

## 概述

`internal/agent` 负责三件事：

- 维护 ReAct 主循环
- 调度工具调用并把结果写回上下文
- 管理持久化任务列表

## Agent 结构

`Agent` 组合了以下核心依赖：

- `model.ToolCallingChatModel`
- `[]tool.BaseTool`
- `toolMap`
- `Config`
- `State`
- `context.Manager`
- `skill.Manager`

见 [agent.go](../internal/agent/agent.go)。

## 初始化流程

主入口在 `cmd/5hagent/main.go`：

1. 初始化 `TaskList`
2. 创建 LLM client
3. 加载 `prompt/` 目录
4. 取出 `main_agent_system`
5. 创建 `Agent`
6. 初始化工具注册表
7. 把工具绑定到模型
8. 进入 REPL

## Run 与 RunStream

### `Run()`

非流式执行路径：

1. 首次对话时注入主 system prompt
2. 注入已启用的 skills
3. 添加用户消息
4. 调用 `model.Generate()`
5. 若有工具调用，则执行工具并继续下一轮
6. 若无工具调用，则返回最终响应

### `RunStream()`

流式执行路径与 `Run()` 类似，但会：

- 调用 `model.Stream()`
- 边读 chunk 边回调 token
- 合并分片的 `ToolCall`
- 在上下文过长时调用 `Compress()`

## Skill 注入

`injectSkills()` 在首次对话时把所有已启用 skill 追加为独立 system message。

相关代码：

- [injectSkills](../internal/agent/agent.go)
- [skill manager](../internal/skill/skill.go)

## 工具执行

工具执行逻辑已从 `agent.go` 中拆到 `tool_executor.go`。

### `exeTools()`

- 先按工具名把调用分成只读和写入两类
- 统一 `task` 工具会额外解析 `action`
- 只读工具走 `exeToolsConcurrent()`
- 写工具走 `executeSingleTool()`

### `exeToolsConcurrent()`

- 使用 goroutine 并发执行
- 用 channel 收集结果
- 按原始请求顺序把结果写回上下文

### `invokeTool()`

- 优先尝试 `tool.EnhancedInvokableTool`
- 否则降级到 `tool.InvokableTool`

### 当前实现备注

- 并发名单定义在 `tool_executor.go` 的 `readOnlyTools`
- 统一 `task` 工具通过 `action` 进一步区分只读与写入
- 旧的 `task_get` / `task_list` 名称仍保留在分类逻辑中

## 上下文交互

Agent 依赖 `internal/context/ctx.go` 提供：

- `CreateContext()`
- `GetMessages()`
- `AddMessage()`
- `Compress()`

当前压缩策略是简单截断：超过 50 条时保留最近 30 条。

## TaskList

`tasklist.go` 提供持久化任务列表：

- `CreateTask()`
- `GetTask()`
- `UpdateTaskStatus()`
- `ListTasks()`
- `ListTasksByStatus()`
- `DeleteTask()`
- `GetProgress()`

任务保存在 `.5hagent/tasks.json`，但只有在实际创建任务后文件才会出现。

## 相关代码

- [agent.go](../internal/agent/agent.go)
- [tool_executor.go](../internal/agent/tool_executor.go)
- [tasklist.go](../internal/agent/tasklist.go)
- [main.go](../cmd/5hagent/main.go)
