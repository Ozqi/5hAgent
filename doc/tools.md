# Tools - 工具系统

## 位置
`internal/tools/` 目录

## 工具列表

| 工具名 | 文件 | 类型 | 并发 | 功能 |
|--------|------|------|------|------|
| read_file | read_file.go | 只读 | ✅ | 读取文件内容，支持分页 |
| write_file | write_file.go | 写 | ❌ | 创建/覆盖文件 |
| edit | edit.go | 写 | ❌ | 精确字符串替换编辑文件 |
| glob | glob.go | 只读 | ✅ | 文件模式匹配，支持 ** 递归 |
| grep | grep.go | 只读 | ✅ | 代码搜索（ripgrep/grep） |
| list_dir | list_dir.go | 只读 | ✅ | 列出目录内容 |
| exec_shell | exec_shell.go | 写 | ❌ | 执行 shell 命令 |
 | task_create | task_tools.go | 写 | ❌ | 创建任务 |
| task_update | task_tools.go | 写 | ❌ | 更新任务状态 |
| task_get | task_tools.go | 只读 | ✅ | 获取任务详情 |
| task_list | task_tools.go | 只读 | ✅ | 列出所有任务 |
| task_delete | task_tools.go | 写 | ❌ | 删除任务 |

**总计**: 12 个工具（6 个只读，6 个写入）

## 核心工具详解

### 文件操作

#### read_file - 文件读取
- **实现**: `bufio.Scanner` 逐行读取
- **特性**: 支持 offset/limit 分页，输出带行号格式
- **代码**: `read_file.go`

#### write_file - 文件写入 (Phase 3)
- **实现**: `os.WriteFile` + `os.MkdirAll`
- **特性**: 创建/覆盖文件，自动创建父目录
- **代码**: `write_file.go`

#### edit - 文件编辑
- **实现**: `os.ReadFile` + `strings.ReplaceAll` + `os.WriteFile`
- **特性**: 精确字符串替换，替换所有匹配项
- **代码**: `edit.go`

### 文件搜索

#### glob - 文件模式匹配
- **实现**: `filepath.Glob` (简单模式) / `filepath.Walk` (递归模式)
- **特性**: 支持 `*`, `**`, `?` 通配符，递归目录搜索
- **代码**: `glob.go`

#### grep - 代码搜索 (Phase 3)
- **实现**: `ripgrep` (优先) / `grep` (fallback)
- **特性**: 
  - 使用 `exec.CommandContext` 调用外部命令
  - 支持正则表达式和文件类型过滤
  - 返回文件、行号、列号、匹配文本
- **代码**: `grep.go`

#### list_dir - 目录列表 (Phase 3)
- **实现**: `os.ReadDir` (非递归) / `filepath.Walk` (递归)
- **特性**: 列出目录内容，支持递归遍历，返回文件信息
- **代码**: `list_dir.go`

### 执行

#### exec_shell - Shell 命令执行
- **实现**: `exec.CommandContext` + `cmd.CombinedOutput`
- **特性**: 支持超时控制，分别捕获 stdout/stderr，返回退出码
- **代码**: `exec_shell.go`

### 任务管理 (Phase 3)

所有任务工具基于 `internal/agent/tasklist.go` 的 TaskList 实现：
- **存储**: JSON 文件持久化 (`.miniagent/tasks.json`)
- **并发**: `sync.RWMutex` 保证线程安全
- **状态**: pending, in_progress, completed, failed

#### task_create - 创建任务
- **实现**: `TaskList.CreateTask()`
- **输入**: id, title, description
- **输出**: Task 对象

#### task_update - 更新任务
- **实现**: `TaskList.UpdateTaskStatus()`
- **输入**: id, status
- **特性**: 自动记录完成时间

#### task_get - 获取任务
- **实现**: `TaskList.GetTask()`
- **输入**: id
- **输出**: Task 详情

#### task_list - 列出任务
- **实现**: `TaskList.ListTasks()` / `TaskList.ListTasksByStatus()`
- **特性**: 可选状态过滤，返回任务列表 + 进度统计

#### task_delete - 删除任务
- **实现**: `TaskList.DeleteTask()`
- **输入**: id
- **特性**: 永久删除任务

## 工具注册

**位置**：`registry.go`

所有工具在 `init()` 函数中自动注册到全局 registry。

## 并发执行策略

**只读工具**（支持并发）：
- read_file, glob, grep, list_dir
- task_get, task_list

**写工具**（必须串行）：
- write_file, edit, exec_shell
- task_create, task_update, task_delete

**实现位置**：`agent.go:438-445`

## 添加新工具

1. 在 `internal/tools/` 创建 `new_tool.go`
2. 定义 Input/Output 结构体
3. 实现 `NewXxxTool()` 函数
4. 在 `registry.go` 的 `init()` 中注册
5. 如果是只读工具，在 `agent.go:readOnlyTools` 中添加

**模板**：
```go
type NewToolInput struct {
    Param string `json:"param" jsonschema:"required,description=Parameter description"`
}

type NewToolOutput struct {
    Result string `json:"result"`
}

func NewNewTool() (tool.EnhancedInvokableTool, error) {
    return utils.InferEnhancedTool(
        "new_tool",
        "Tool description for LLM",
        func(ctx context.Context, input NewToolInput) (*schema.ToolResult, error) {
            // 实现逻辑
            output := NewToolOutput{Result: "..."}
            outputJSON, _ := json.Marshal(output)
            return &schema.ToolResult{
                Parts: []schema.ToolOutputPart{
                    {Type: schema.ToolPartTypeText, Text: string(outputJSON)},
                },
            }, nil
        },
    )
}
```

