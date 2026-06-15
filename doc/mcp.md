# MCP - Model Context Protocol

## 架构

```mermaid
flowchart TB
    subgraph Config["配置"]
        mcp_json["~/.5hAgent/mcp.json"]
    end

    subgraph Server["MCP 服务器"]
        load["LoadMCPServers()"]
        stdio["mcp.StdioClient"]
        process["进程管理"]
    end

    subgraph Registry["工具注册"]
        list["ListTools()"]
        specs["mcp.ToolSpec[]"]
        register["RegisterMCPTools()"]
        mcp_tool["mcp.*"]
    end

    subgraph Execute["执行"]
        agent["Agent"]
        invoke["MCPTool.InvokableRun()"]
        call["client.CallTool()"]
    end

    mcp_json --> load
    load --> stdio
    stdio --> process
    process --> list
    list --> specs
    specs --> register
    register --> mcp_tool
    agent --> invoke
    invoke --> call
```

## 位置

- `internal/mcp/mcp.go` - 协议类型
- `internal/mcp/client_stdio.go` - StdioClient 实现
- `internal/tools/mcp_tool.go` - 工具封装
- `internal/commands/mcp.go` - `/mcp` 命令

## 核心类型

### mcp.ServerConfig

MCP 服务器启动配置（[mcp.go:16-22](internal/mcp/mcp.go)）：

```go
type ServerConfig struct {
    Name           string            // 服务器名称
    Command        string            // 启动命令
    Args           []string          // 命令参数
    Env            map[string]string // 环境变量
    StartupTimeout time.Duration     // 启动超时
}
```

### mcp.ToolSpec

工具元数据（[mcp.go:24-28](internal/mcp/mcp.go)）：

```go
type ToolSpec struct {
    Name        string // 工具名
    Description string // 描述
    ReadOnly    bool   // 是否只读
}
```

### mcp.Client

传输层接口（[mcp.go:30-32](internal/mcp/mcp.go)）：

```go
type Client interface {
    CallTool(ctx context.Context, toolName string, arguments string) (string, error)
}
```

## 配置

配置文件：`~/.5hAgent/mcp.json`

```json
{
  "servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "enabled": true
    }
  ]
}
```

## 注册流程

```mermaid
sequenceDiagram
    participant Main as main.go
    participant Load as LoadMCPServers
    participant Client as StdioClient
    participant List as ListTools
    participant Reg as RegisterMCPTools

    Main->>Load: 读取 ~/.5hAgent/mcp.json
    Load-->>Main: []ServerConfig
    Main->>Client: NewStdioClient(config)
    Client->>Client: 启动进程
    Main->>List: 获取工具列表
    List-->>Main: []ToolSpec
    Main->>Reg: RegisterMCPTools(server, client, specs)
    Reg->>Reg: 创建 MCPTool
    Reg->>Reg: 注册到 toolmeta
```

## 工具名格式

```
mcp.{serverName}.{toolName}
例：mcp.filesystem.list_directory
```

`mcp.FullToolName(serverName, toolName)` 拼接全名。

## 工具分类

| Category | 来源         | 示例                          |
| -------- | ------------ | ----------------------------- |
| base     | 内置文件工具 | base.read_file                |
| task     | TaskList     | task.task                     |
| skill    | Skill 系统   | skill.skill                   |
| mcp      | MCP Server   | mcp.filesystem.list_directory |

## 执行路径

```
Agent.RunStream() → toolQueue → exeToolCall() → MCPTool.InvokableRun() → client.CallTool()
```

## /mcp 命令

| 命令                                  | 说明       |
| ------------------------------------- | ---------- |
| `/mcp list`                           | 列出服务器 |
| `/mcp add <name> <command> [args...]` | 添加       |
| `/mcp remove <name>`                  | 删除       |
| `/mcp enable <name>`                  | 启用       |
| `/mcp disable <name>`                 | 禁用       |

> 注意：运行时修改配置需要重启生效

## 错误处理

MCP 工具执行失败时，错误信息直接透传 MCP 服务器的原始返回，不添加额外提示干扰。

## mcp_list_tools

`mcp.list_tools` 是内置工具，用于列出当前已连接的 MCP 服务器及其工具列表：

```
Agent >>> 使用 mcp_list_tools 查看可用工具
```

| 参数 | 类型 | 说明 |
|------|------|------|
| server | string | MCP 服务器名称（可选，不填则列出所有） |

## 相关代码

- [mcp.go](../internal/mcp/mcp.go)
- [client_stdio.go](../internal/mcp/client_stdio.go)
- [mcp_tool.go](../internal/tools/mcp_tool.go)
- [mcp_list_tools.go](../internal/tools/mcp_list_tools.go)
- [commands/mcp.go](../internal/commands/mcp.go)
