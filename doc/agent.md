# Agent - 核心模块

## 位置

`internal/agent/agent.go` (~686行)

## 结构

```go
type Agent struct {
    model      model.ToolCallingChatModel  // LLM
    tools      []tool.BaseTool             // 工具列表
    toolMap    map[string]tool.BaseTool    // O(1)查找
    config     *Config                     // 配置
    state      *State                      // 状态
    ctxManager *agentctx.Manager           // 上下文管理
}
```

## 核心函数

### NewAgent(model, tools, config)

创建Agent，构建toolMap

### Run(ctx, messageCtx, input) - 非流式

ReAct循环：

1. 注入SystemPrompt(首次)
2. 添加用户消息
3. 循环(最多MaxTurns):
   - 调用LLM生成响应
   - 检查ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回响应

### RunStream(ctx, messageCtx, input, onToken) - 流式

流式ReAct循环：

1. 注入SystemPrompt(首次)
2. 添加用户消息
3. **上下文压缩检查**
   - 超过50条消息时自动压缩
   - 保留最近30条
4. 循环(最多MaxTurns):
   - 流式调用LLM
   - 读取chunks并调用onToken回调
   - **ToolCall合并**（避免多工具调用时arguments错误合并）
   - 收集完整响应和ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回完整内容

### exeTools(ctx, messageCtx, toolCalls) - 支持并发

执行工具列表（智能分类）：

1. **分类工具**
   - 只读工具: read_file, glob, grep, list_dir, task_get, task_list → 并发执行
   - 写工具: write_file, edit, exec_shell, task_create, task_update, task_delete → 串行执行
2. 并发执行只读工具: `exeToolsConcurrent()`
3. 串行执行写工具
4. 添加结果到messageCtx

### exeToolsConcurrent(ctx, messageCtx, toolCalls)

并发执行只读工具：

1. 创建结果channel
2. 为每个工具启动goroutine
3. 收集结果并按原顺序添加到上下文

## 关键特性

### 流式输出与ToolCall合并

使用列表+ID索引追踪工具调用，避免多工具调用时arguments错误合并

### 上下文压缩

自动检查消息数量，超过阈值时压缩历史消息

### 工具并发执行

只读工具自动并发执行，提升性能

## 相关文档

- 工具系统: `doc/tools.md`
- 上下文管理: `doc/context.md`
- 流式输出: `doc/stage2_streaming.md`

# TaskList

## 概述

TaskList 提供持久化任务列表管理功能，支持长程任务的规划、追踪和执行。

## 架构

```
internal/tasklist/
  └── tasklist.go          # 任务列表核心实现

internal/tools/
  └── task_tools.go        # 任务管理工具（Agent 可调用）

.miniagent/
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
// cmd/miniagent/main.go
taskListPath := ".miniagent/tasks.json"
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
- `cmd/miniagent/main.go`: 初始化入口
- `internal/agent/agent.go`: 并发工具列表配置
