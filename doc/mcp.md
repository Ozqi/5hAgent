# MCP 模块

## 概述

MCP (Model Context Protocol) 工具通过 `mcp.Client` 接口接入框架，框架不实现 MCP 传输层，只负责：

- 将 `Client.CallTool` 包装为 Eino `InvokableTool`
- 注册到全局工具注册表（与 base/task/skill 同级）
- 工具名格式 `mcp.{serverName}.{toolName}`

传输连接（stdio/sse/http）由调用方自行实现。

## 关键文件

```
internal/mcp/mcp.go          # 协议类型 + Client 接口
internal/tools/mcp_tool.go   # MCPTool 包装器
internal/tools/registry.go   # RegisterMCPTools 注册入口
internal/toolmeta/toolmeta.go # CategoryMCP 分类
```

## 核心类型

### mcp.ServerConfig

MCP Server 启动配置，供调用方读取以启动进程。

```go
type ServerConfig struct {
    Name           string
    Command        string
    Args           []string
    Env            map[string]string
    StartupTimeout time.Duration
}
```

### mcp.ToolSpec

单个 MCP 工具的元数据，注册时传入 `RegisterMCPTools`。

```go
type ToolSpec struct {
    Name        string
    Description string
    ReadOnly    bool
}
```

### mcp.Client

MCP 传输层接口，框架不实现，由调用方提供。

```go
type Client interface {
    CallTool(ctx context.Context, toolName string, arguments string) (string, error)
}
```

### tools.MCPTool

`mcp.Client` → Eino `InvokableTool` 的适配器，由 `RegisterMCPTools` 内部使用，一般不直接构造。

```go
type MCPTool struct {
    serverName string
    client     mcp.Client
    spec       mcp.ToolSpec
}
```

## 注册流程

```
mcp.Client 实现（调用方提供）
  → tools.RegisterMCPTools(serverName, client, specs)
      → tools.NewMCPTool → 加入全局 registry
      → toolmeta.Register(CategoryMCP, ...)
```

注册后 MCP 工具与 base/task/skill 工具一起通过 `tools.GetAllTools()` / `tools.GetToolByName()` 供 Agent 使用。

## 工具名格式

```
mcp.{serverName}.{toolName}
例：mcp.claude_context.search_code
```

`mcp.FullToolName(serverName, toolName)` 拼接全名。

## 工具分类 (toolmeta)

| Category | 来源           | 示例                              |
|----------|----------------|-----------------------------------|
| base     | 内置文件工具   | base.read_file                    |
| task     | TaskList       | task.task                         |
| skill    | Skill 系统     | skill.skill                       |
| mcp      | MCP Server     | mcp.claude_context.search_code    |

## 执行路径

Agent 执行 MCP 工具时，与普通工具完全一致地经过 `exeToolsPar` → `exeToolCall` → `MCPTool.InvokableRun` → `mcp.Client.CallTool`。

失败时 `toolHint` 对 `CategoryMCP` 返回统一提示：check the remote tool arguments and server-specific requirements before retrying.

## 使用示例

```go
// 1. 实现 mcp.Client（传输层由调用方负责）
type myMCPClient struct{}

func (c *myMCPClient) CallTool(ctx context.Context, toolName, arguments string) (string, error) {
    // 例：通过 stdio 连接本地 MCP Server 并调用
    return callMCP(toolName, arguments)
}

// 2. 注册 MCP 工具
err := tools.RegisterMCPTools("claude_context", &myMCPClient{}, []mcp.ToolSpec{
    {Name: "search_code", Description: "semantic search", ReadOnly: true},
    {Name: "read_resource", Description: "read a resource", ReadOnly: true},
})

// 3. 启动 Agent，MCP 工具自动参与调度
```

## 注意事项

- 框架不管理 MCP Server 进程生命周期，由调用方在 `main.go` 中自行启动并管理
- `mcp.ServerConfig` 仅供调用方参考，不被框架直接使用
- MCP 工具参数以 JSON 字符串形式透传，`MCPTool` 不做 schema 校验
- `ReadOnly: true` 的 MCP 工具会被 `toolmeta.IsReadOnly` 识别，Agent 在规划阶段会跳过幂等检查
