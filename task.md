# Shared Task List

> This file is the single source of truth for active 5hAgent tasks.
> Edit task entries carefully and keep the managed markers intact.

<!-- 5hagent:tasks:start -->
## Shared Tasks

### refactor-eino | 使用 Eino 框架重构
- status: completed
- description: 分阶段重构已完成
  - ✅ Phase 1: Callback 日志系统
  - ✅ Phase 2: 简化 streamToolCollector
  - ✅ Phase 3: 删除未使用代码
  - ⏸️ Phase 4: 保持现状（流式执行模式与 ToolsNode 不兼容）
- created_at: 2026-05-01T06:35:00Z
- updated_at: 2026-05-01T06:45:00Z

### test-notion-mcp-001 | 测试Notion MCP
- status: completed
- description: MCP 配置完成，连接成功(22工具)，搜索和读取正常。发现需用 OPENAPI_MCP_HEADERS 代替 NOTION_API_KEY。
- evidence: "Registered 22 tools from notion" + search 返回实际数据
- created_at: 2026-04-26T18:16:54Z
- updated_at: 2026-05-01T10:40:00Z

<!-- 5hagent:tasks:end -->
