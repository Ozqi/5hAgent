# claude-context MCP

## 目的

记录 `claude-context` 接入验证要点。当前不是默认启动能力。

## 目标工具

- `index_codebase`
- `search_code`
- `clear_index`
- `get_indexing_status`

## 验证顺序

1. 本地 Milvus 可启动。
2. claude-context MCP server 可启动。
3. `tools/list` 能返回目标工具。
4. 当前 5hAgent 仓库可索引。
5. `search_code` 返回可用代码片段。

## 接入边界

- 不在 `runtime.New` 中同步启动。
- 不把语义检索替代 `base.grep/base.glob/base.read_file`。
- 索引缺失时应给出明确错误，而不是让 Agent 盲查。
