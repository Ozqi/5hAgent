# md-mcp

最小 Markdown 读写 MCP server，用于验证 5hAgent 的 MCP 接入。

## 工具

- `read_md`: 读取 `MD_MCP_ROOT` 下的 `.md` 文件。
- `write_md`: 创建或覆盖 `MD_MCP_ROOT` 下的 `.md` 文件。

## 配置

把下面 server 配置加入 `~/.5hAgent/mcp.json` 的 `servers` 数组：

```json
{
  "name": "md",
  "command": "node",
  "args": ["/path/to/5hAgent/testspace/md_mcp/server.js"],
  "env": {
    "MD_MCP_ROOT": "/path/to/5hAgent/testspace/md_mcp_data"
  },
  "enabled": true
}
```

重启 `5hagent` 后应出现：

- `mcp.md.read_md`
- `mcp.md.write_md`

可让 Agent 调用 `mcp.md.write_md` 写入 `notes.md`，再调用 `mcp.md.read_md` 读取验证。
