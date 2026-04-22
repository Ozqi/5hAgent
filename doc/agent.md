# Agent - 核心模块

## 位置

`internal/agent/agent.go` (~686行)
`internal/agent/skill.go` (~90行) - Skill 命令处理

## 结构

```go
type Agent struct {
    model        model.ToolCallingChatModel  // LLM
    tools        []tool.BaseTool             // 工具列表
    toolMap      map[string]tool.BaseTool    // O(1)查找
    config       *Config                     // 配置
    state        *State                      // 状态
    ctxManager   *agentctx.Manager           // 上下文管理
    skillManager *skill.Manager              // 技能管理
}
```

## 核心函数

### NewAgent(model, tools, config)

创建Agent，构建toolMap，初始化技能管理器

**新增**: 加载 `.5hagent/skills/*/SKILL.md` 技能定义

### Run(ctx, messageCtx, input) - 非流式

ReAct循环：

1. 注入SystemPrompt(首次)
2. **注入启用的Skills**（作为独立System消息）
3. 添加用户消息
4. 循环(最多MaxTurns):
   - 调用LLM生成响应
   - 检查ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回响应

### RunStream(ctx, messageCtx, input, onToken) - 流式

流式ReAct循环：

1. 注入SystemPrompt(首次)
2. **注入启用的Skills**（作为独立System消息）
3. 添加用户消息
4. **上下文压缩检查**
   - 超过50条消息时自动压缩
   - 保留最近30条
5. 循环(最多MaxTurns):
   - 流式调用LLM
   - 读取chunks并调用onToken回调
   - **ToolCall合并**（避免多工具调用时arguments错误合并）
   - 收集完整响应和ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回完整内容

### exeTools(ctx, messageCtx, toolCalls) - 多路执行

**核心思想**: 只读工具可以并发执行（无副作用），写工具必须串行执行（避免竞态条件）

**执行流程**:

1. **工具分类** (agent.go:482-504)

   ```go
   readOnlyTools := map[string]bool{
       "read_file": true,  "glob": true,      "grep": true,
       "list_dir": true,   "task_get": true,  "task_list": true,
   }
   ```

   - 遍历 `toolCalls`，根据工具名分类到 `readOnlyCalls` 或 `writeCalls`
   - 跳过 `Function.Name` 为空的无效调用

2. **并发执行只读工具** (agent.go:507-511)

   ```go
   if len(readOnlyCalls) > 0 {
       a.exeToolsConcurrent(ctx, messageCtx, readOnlyCalls)
   }
   ```

   - 调用 `exeToolsConcurrent()` 并发执行所有只读工具
   - 例如：同时读取 3 个文件，而不是依次读取

3. **串行执行写工具** (agent.go:514-601)

   ```go
   for idx, tc := range writeCalls {
       // 1. 显示工具执行提示
       logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, false)

       // 2. 查找工具
       t := a.findTool(tc.Function.Name)
       if t == nil {
           // 添加错误消息到上下文
           errMsg := schema.ToolMessage("tool not found: ...", tc.ID)
           a.ctxManager.AddMessage(messageCtx, errMsg)
           continue
       }

       // 3. 执行工具（支持两种接口）
       if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
           // 返回 *schema.ToolResult（支持多媒体）
           toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
           result = formatToolResult(toolResult)
       } else if invokable, ok := t.(tool.InvokableTool); ok {
           // 返回 string（简单文本）
           result, err = invokable.InvokableRun(ctx, tc.Function.Arguments)
       }

       // 4. 处理结果
       if err != nil {
           // 添加错误消息
           errMsg := schema.ToolMessage("tool execution failed: ...", tc.ID)
       } else {
           // 添加成功结果
           resultMsg := schema.ToolMessage(result, tc.ID)
       }
       a.ctxManager.AddMessage(messageCtx, resultMsg)
   }
   ```

   - 按顺序执行每个写工具（避免文件冲突、数据竞争）
   - 每个工具执行完毕后立即将结果添加到上下文
   - 错误不会中断流程，会记录错误消息并继续

4. **结果添加到上下文**
   - 所有工具结果（成功或失败）都作为 `ToolMessage` 添加到 `messageCtx`
   - LLM 在下一轮会看到这些结果，决定下一步操作

**为什么这样设计**:

- 只读工具并发 → 提升性能（读取 5 个文件从 5s 降到 1s）
- 写工具串行 → 保证安全（避免同时修改同一文件导致冲突）

### exeToolsConcurrent(ctx, messageCtx, toolCalls) - 并发执行实现

**并发模型**: 使用 goroutine + channel 收集结果，保证结果顺序

**执行流程**:

1. **创建结果通道** (agent.go:609-616)

   ```go
   type toolResult struct {
       idx    int              // 原始索引（用于排序）
       tc     schema.ToolCall  // 工具调用信息
       result string           // 执行结果
       err    error            // 执行错误
   }
   results := make(chan toolResult, len(toolCalls))
   ```

2. **启动 goroutine 并发执行** (agent.go:619-654)

   ```go
   for idx, tc := range toolCalls {
       go func(idx int, tc schema.ToolCall) {
           // 1. 显示工具执行提示（标记为 concurrent）
           logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, true)

           // 2. 查找工具
           t := a.findTool(tc.Function.Name)
           if t == nil {
               results <- toolResult{idx: idx, tc: tc, err: fmt.Errorf("tool not found")}
               return
           }

           // 3. 执行工具（同样支持两种接口）
           var result string
           var execErr error
           if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
               toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
               result = formatToolResult(toolResult)
           } else if invokable, ok := t.(tool.InvokableTool); ok {
               result, execErr = invokable.InvokableRun(ctx, tc.Function.Arguments)
           }

           // 4. 发送结果到 channel
           results <- toolResult{idx: idx, tc: tc, result: result, err: execErr}
       }(idx, tc)
   }
   ```

   - 每个工具在独立的 goroutine 中执行
   - 通过闭包捕获 `idx` 和 `tc`，避免循环变量问题

3. **收集结果** (agent.go:657-661)

   ```go
   collectedResults := make([]toolResult, len(toolCalls))
   for i := 0; i < len(toolCalls); i++ {
       res := <-results
       collectedResults[res.idx] = res  // 按原始索引存储
   }
   ```

   - 从 channel 接收所有结果
   - 使用 `res.idx` 恢复原始顺序（重要！）

4. **按顺序添加到上下文** (agent.go:664-682)
   ```go
   for _, res := range collectedResults {
       if res.err != nil {
           errMsg := schema.ToolMessage("tool execution failed: ...", res.tc.ID)
       } else {
           resultMsg := schema.ToolMessage(res.result, res.tc.ID)
       }
       a.ctxManager.AddMessage(messageCtx, resultMsg)
   }
   ```

   - 按原始调用顺序添加结果（保证上下文的逻辑一致性）
   - LLM 看到的结果顺序与请求顺序一致

**关键设计点**:

- **保序**: 虽然并发执行，但结果按原始顺序添加到上下文
- **错误隔离**: 一个工具失败不影响其他工具执行
- **性能提升**: 3 个 `read_file` 并发执行，总耗时 = max(t1, t2, t3)，而非 t1+t2+t3

## 关键特性

### Skill 注入机制

- 首次对话时，在 System Prompt 之后注入启用的技能
- 每个技能作为独立的 System 消息
- 技能格式：Markdown + YAML frontmatter（遵循 Claude Code 规范）
- 命令：`/skill list|enable|disable`

详见 `doc/skill_injection.md`

### 流式输出与ToolCall合并

使用列表+ID索引追踪工具调用，避免多工具调用时arguments错误合并

### 边输出边执行工具

流式输出过程中，检测到完整的工具调用（valid JSON）时立即异步执行，无需等待整个流结束

### 上下文压缩

自动检查消息数量，超过阈值时压缩历史消息

### 工具并发执行

只读工具自动并发执行，提升性能

## 相关文档

- 工具系统: `doc/tools.md`
- 上下文管理: `doc/context.md`
- Skill 注入: `doc/skill_injection.md`
- Phase 2 总结: `doc/phase2_summary.md`

# TaskList

## 概述

TaskList 提供持久化任务列表管理功能，支持长程任务的规划、追踪和执行。

## 架构

```
internal/tasklist/
  └── tasklist.go          # 任务列表核心实现

internal/tools/
  └── task_tools.go        # 任务管理工具（Agent 可调用）

.5hagent/
  └── tasks.json           # 任务持久化存储
```

## 核心组件

### 1. TaskList (`internal/tasklist/tasklist.go`)

**关键类型**:

- `Task`: 任务结构，包含 ID、标题、描述、状态、时间戳
- `TaskStatus`: 任务状态枚举（pending, in_progress, completed, failed）
- `TaskList`: 任务列表管理器，支持并发安全的 CRUD 操作

**关键函数**:

- `NewTaskList(filePath)`: 创建任务列表，自动加载已有任务
- `CreateTask(id, title, desc)`: 创建新任务
- `UpdateTaskStatus(id, status)`: 更新任务状态
- `GetTask(id)`: 获取任务详情
- `ListTasks()`: 列出所有任务
- `ListTasksByStatus(status)`: 按状态筛选任务
- `DeleteTask(id)`: 删除任务
- `GetProgress()`: 获取进度统计

**持久化**:

- 自动保存到 JSON 文件
- 使用 `sync.RWMutex` 保证并发安全

### 2. 任务管理工具 (`internal/tools/task_tools.go`)

Agent 可调用的 5 个工具:

| 工具          | 功能     | 输入                   | 输出                |
| ------------- | -------- | ---------------------- | ------------------- |
| `task_create` | 创建任务 | id, title, description | Task 对象           |
| `task_update` | 更新状态 | id, status             | 更新后的 Task       |
| `task_get`    | 获取详情 | id                     | Task 对象           |
| `task_list`   | 列出任务 | status (可选)          | 任务列表 + 进度统计 |
| `task_delete` | 删除任务 | id                     | 成功消息            |

**只读工具**: `task_get`, `task_list` 支持并发执行

## 使用示例

### 初始化

```go
// cmd/5hagent/main.go
taskListPath := ".5hagent/tasks.json"
if err := tools.InitTaskList(taskListPath); err != nil {
    log.Fatal(err)
}
```

### Agent 调用示例

```
用户: 帮我创建一个任务，实现用户登录功能

Agent:
1. 调用 task_create:
   {
     "id": "task-001",
     "title": "实现用户登录功能",
     "description": "包括前端表单、后端验证、JWT token 生成"
   }

2. 调用 task_update:
   {
     "id": "task-001",
     "status": "in_progress"
   }

3. 执行实现...

4. 调用 task_update:
   {
     "id": "task-001",
     "status": "completed"
   }
```

### 查看进度

```
用户: 当前任务进度如何？

Agent: 调用 task_list
返回:
{
  "tasks": [...],
  "progress": {
    "total": 5,
    "pending": 2,
    "in_progress": 1,
    "completed": 2,
    "failed": 0
  }
}
```

## 数据格式

### tasks.json 示例

```json
{
  "task-001": {
    "id": "task-001",
    "title": "实现用户登录",
    "description": "包括前端表单、后端验证、JWT token 生成",
    "status": "completed",
    "created_at": "2026-04-20T10:00:00Z",
    "updated_at": "2026-04-20T11:30:00Z",
    "completed_at": "2026-04-20T11:30:00Z"
  },
  "task-002": {
    "id": "task-002",
    "title": "添加单元测试",
    "description": "为登录模块添加单元测试",
    "status": "in_progress",
    "created_at": "2026-04-20T11:35:00Z",
    "updated_at": "2026-04-20T12:00:00Z"
  }
}
```

## 并发安全

- 使用 `sync.RWMutex` 保护任务列表
- 读操作（`GetTask`, `ListTasks`）使用读锁，支持并发
- 写操作（`CreateTask`, `UpdateTaskStatus`, `DeleteTask`）使用写锁

## 未来扩展

- [ ] 任务依赖关系（blocked_by, depends_on）
- [ ] 任务优先级
- [ ] 任务标签/分类
- [ ] 任务执行日志
- [ ] 自动规划（根据大任务生成子任务）
- [ ] 进度追踪（估算剩余时间）

## 相关文件

- `internal/tasklist/tasklist.go`: 核心实现
- `internal/tools/task_tools.go`: Agent 工具
- `internal/tools/registry.go`: 工具注册
- `cmd/5hagent/main.go`: 初始化入口
- `internal/agent/agent.go`: 并发工具列表配置
