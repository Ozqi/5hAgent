# Tools - 工具系统

## 位置
`internal/tools/` 目录

## 工具列表

| 工具名 | 文件 | 行数 | 类型 | 并发 | 功能 |
|--------|------|------|------|------|------|
| read_file | read_file.go | 115 | 只读 | ✅ | 读取文件内容，支持分页 |
| glob | glob.go | 120 | 只读 | ✅ | 文件模式匹配，支持 ** 递归 |
| exec_shell | exec_shell.go | 90 | 写 | ❌ | 执行 shell 命令 |
| edit | edit.go | 95 | 写 | ❌ | 精确字符串替换编辑文件 |

## 工具详解

### 1. read_file - 文件读取

**输入**：
```json
{
  "path": "/path/to/file",
  "offset": 1,     // 起始行号（默认1）
  "limit": 100     // 读取行数（默认100）
}
```

**输出**：
```json
{
  "content": "1\tline1\n2\tline2\n...",
  "total_lines": 150
}
```

**实现要点**：
- 使用 `bufio.Scanner` 逐行读取
- 支持 offset/limit 分页
- 输出带行号（`行号\t内容`）

**代码位置**：`read_file.go:33-114`

---

### 2. glob - 文件模式匹配 ⭐ 新增

**输入**：
```json
{
  "pattern": "**/*.go",
  "path": "."      // 基础目录（默认当前目录）
}
```

**输出**：
```json
{
  "files": ["cmd/main.go", "internal/agent/agent.go", ...],
  "count": 14
}
```

**支持的模式**：
- `*`: 匹配任意字符（不含 `/`）
- `**`: 递归匹配目录
- `?`: 匹配单个字符

**实现要点**：
- 简单模式：使用 `filepath.Glob()`
- 递归模式（含 `**`）：使用 `filepath.Walk()` + 自定义匹配
- 结果自动排序

**代码位置**：
- `glob.go:29-73` - NewGlobTool
- `glob.go:76-110` - recursiveGlob 递归匹配

**示例**：
```bash
# 查找所有 Go 文件
{"pattern": "**/*.go"}

# 查找 internal 目录下的 Go 文件
{"pattern": "**/*.go", "path": "internal"}
```

---

### 3. exec_shell - Shell 命令执行

**输入**：
```json
{
  "command": "ls -la",
  "timeout": 30    // 超时秒数（默认30）
}
```

**输出**：
```json
{
  "stdout": "...",
  "stderr": "...",
  "exit_code": 0
}
```

**实现要点**：
- 使用 `exec.CommandContext` 支持超时
- 分别捕获 stdout/stderr
- 返回退出码

**代码位置**：`exec_shell.go:33-89`

---

### 4. edit - 文件编辑 ⭐ 新增

**输入**：
```json
{
  "path": "/path/to/file",
  "old_string": "exact string to replace",
  "new_string": "replacement string"
}
```

**输出**：
```json
{
  "success": true,
  "message": "Replaced 2 occurrence(s)",
  "replacements": 2
}
```

**实现要点**：
- 使用 `os.ReadFile` 读取全文
- 使用 `strings.ReplaceAll` 替换所有匹配
- 使用 `os.WriteFile` 写回文件
- 必须精确匹配（包括空格、换行）

**代码位置**：`edit.go:29-95`

**注意事项**：
- `old_string` 必须完全匹配，否则返回 `success: false`
- 替换所有出现的位置（不支持只替换一次）
- 适合小文件，大文件可能内存占用高

---

## 工具注册

**位置**：`registry.go`

```go
func init() {
    // 按顺序注册所有工具
    registry = append(registry, NewReadFileTool())
    registry = append(registry, NewExecShellTool())
    registry = append(registry, NewGlobTool())      // Phase 2 新增
    registry = append(registry, NewEditTool())      // Phase 2 新增
}

func GetAllTools() []tool.BaseTool {
    return registry
}
```

## 工具接口

所有工具实现 Eino 的 `tool.EnhancedInvokableTool` 接口：

```go
type EnhancedInvokableTool interface {
    Info(ctx context.Context) (*schema.ToolInfo, error)
    InvokableRun(ctx context.Context, argumentsInJSON *schema.ToolArgument) (*schema.ToolResult, error)
}
```

**创建工具**：使用 `utils.InferEnhancedTool`
```go
return utils.InferEnhancedTool(
    "tool_name",
    "tool description",
    func(ctx context.Context, input InputStruct) (*schema.ToolResult, error) {
        // 实现逻辑
        return &schema.ToolResult{...}, nil
    },
)
```

## 并发执行策略

**只读工具**（可并发）：
- `read_file`: 读取文件不修改状态
- `glob`: 文件搜索不修改状态

**写工具**（必须串行）：
- `exec_shell`: 可能修改文件系统
- `edit`: 直接修改文件

**实现位置**：`agent.go:428-445`
```go
readOnlyTools := map[string]bool{
    "read_file": true,
    "glob":      true,
}
```

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
