# MCP 模块

## 概述

MCP (Model Context Protocol) 工具通过 `mcp.Client` 接口接入框架。5hAgent 已完整集成 MCP Stdio Client，支持通过配置文件启动和管理 MCP 服务器。

核心功能：
- 将 `Client.CallTool` 包装为 Eino `InvokableTool`
- 注册到全局工具注册表（与 base/task/skill 同级）
- 工具名格式 `mcp.{serverName}.{toolName}`
- 自动启动和关闭 MCP 服务器进程

## 配置 MCP 服务器

在 `~/.5hAgent/config.yaml` 中配置 MCP 服务器：

```yaml
mcp:
  servers:
    - name: filesystem
      command: npx
      args:
        - -y
        - @modelcontextprotocol/server-filesystem
        - /tmp
      startup_timeout: 10s
    - name: brave_search
      command: npx
      args:
        - -y
        - @modelcontextprotocol/server-brave-search
      env:
        BRAVE_API_KEY: your_api_key_here
      startup_timeout: 10s
```

### 配置字段说明

- `name`: 服务器名称（必需，只能包含字母、数字、下划线）
- `command`: 启动命令（必需）
- `args`: 命令参数（可选）
- `env`: 环境变量（可选）
- `startup_timeout`: 启动超时时间（可选，默认 10s）

## 可用的 MCP 服务器

### 官方服务器

1. **filesystem** - 文件系统操作
   ```yaml
   - name: filesystem
     command: npx
     args: [-y, @modelcontextprotocol/server-filesystem, /path/to/directory]
   ```

2. **brave-search** - Brave 搜索引擎
   ```yaml
   - name: brave_search
     command: npx
     args: [-y, @modelcontextprotocol/server-brave-search]
     env:
       BRAVE_API_KEY: your_key
   ```

3. **github** - GitHub 仓库操作
   ```yaml
   - name: github
     command: npx
     args: [-y, @modelcontextprotocol/server-github]
     env:
       GITHUB_TOKEN: your_token
   ```

4. **postgres** - PostgreSQL 数据库
   ```yaml
   - name: postgres
     command: npx
     args: [-y, @modelcontextprotocol/server-postgres]
     env:
       POSTGRES_CONNECTION_STRING: postgresql://...
   ```

更多服务器见：https://github.com/modelcontextprotocol/servers

## 使用示例

启动 5hAgent 后，MCP 工具会自动注册。在对话中可以直接使用：

```
用户: 列出 /tmp 目录下的文件
Agent: [调用 mcp.filesystem.list_directory]

用户: 搜索最新的 AI 新闻
Agent: [调用 mcp.brave_search.search]
```

## 故障排查

### MCP 服务器启动失败

检查日志输出：
```
[MCP] Starting MCP server: filesystem
[MCP] Failed to start MCP server filesystem: ...
```

常见原因：
1. `npx` 未安装或不在 PATH 中
2. MCP 服务器包未安装（首次运行 npx 会自动安装）
3. 启动超时（增加 `startup_timeout`）
4. 环境变量缺失（检查 `env` 配置）

### 工具调用失败

检查：
1. 工具名称是否正确（格式：`mcp.{server}.{tool}`）
2. 参数格式是否符合工具要求
3. MCP 服务器是否正常运行

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
