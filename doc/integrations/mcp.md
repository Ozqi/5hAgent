# MCP

## 职责

`internal/mcp` 是 MCP stdio client 和协议类型。当前 runtime 启动阶段不启动 MCP server。

## 文件

| 文件 | 作用 |
| --- | --- |
| [mcp.go](../../internal/mcp/mcp.go) | `ServerConfig`、`ToolSpec`、`FullToolName` |
| [client_stdio.go](../../internal/mcp/client_stdio.go) | JSON-RPC stdio client |
| [mcp_tool.go](../../internal/tools/mcp_tool.go) | MCP tool Eino 封装 |
| [mcp.go](../../internal/commands/mcp.go) | `/mcp` 配置命令 |

## 当前状态

- `/mcp` 只管理 `~/.walle/mcp.json` 配置。
- 默认工具列表没有 `mcp.*`。
- `RegisterMCPTools` 保留，但默认启动链路不调用。
- 需要执行 MCP 工具时，应显式连接并注册，避免坏 MCP 配置阻塞默认 TUI/daemon 启动。

## Stdio client

`NewStdioClient` 会：

1. 启动进程。
2. 建立 stdin/stdout/stderr pipe。
3. 启动 read loop。
4. 发送 initialize。
5. 缓存 `tools/list` 结果。

子进程退出会 cancel startup wait，避免等满 timeout。

## 工具名

```text
mcp.<server>.<tool>
```

`server/tool` 名会做最小清洗，避免空白和特殊字符破坏 full name。
