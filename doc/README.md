# 5hAgent 文档索引

```text
doc/
  -> agent.md
  -> tools.md
  -> context.md
  -> skill_injection.md
  -> prompt.md
  -> commonds.md
  -> llm.md / logger.md / cli.md
```

## 核心文档

### [agent.md](agent.md)

说明 Agent 主循环、流式输出、工具执行、skill 注入和任务持久化。

对应代码：

- `internal/agent/agent.go`
- `internal/agent/tool_use.go`
- `internal/agent/tasklist.go`

### [tools.md](tools.md)

说明当前真实工具注册结果、并发策略和 `task` / `skill` 统一工具入口。

对应代码：

- `internal/tools/*.go`
- `internal/tools/registry.go`
- `internal/agent/tool_use.go`

### [context.md](context.md)

说明消息上下文、创建、克隆和简单压缩机制。

对应代码：

- `internal/context/ctx.go`

### [skill_injection.md](skill_injection.md)

说明 `.5hagent/skills/*/SKILL.md` 的加载与注入。

对应代码：

- `internal/skill/skill.go`
- `internal/agent/agent.go`
- `internal/tools/skill_tool.go`

### [prompt.md](prompt.md)

说明 prompt 文件如何从 `prompt/*.md` 读取并注入主流程。

对应代码：

- `internal/utils/utils.go`
- `cmd/5hagent/main.go`

### [commonds.md](commonds.md)

说明 `/skill` 和 `/task` 两个 CLI 斜杠命令。

对应代码：

- `internal/commands/skill.go`
- `internal/commands/task.go`

## 辅助文档

- [llm.md](llm.md): LLM client
- [logger.md](logger.md): 日志系统
- [cli.md](cli.md): CLI 输出

## 快速导航

- 想看主循环：[`agent.md`](agent.md)
- 想看工具注册：[`tools.md`](tools.md)
- 想看 skill 机制：[`skill_injection.md`](skill_injection.md)
- 想看 prompt 加载：[`prompt.md`](prompt.md)
- 想看 slash commands：[`commonds.md`](commonds.md)

## 文档规则

- 文档必须和代码匹配。
- 优先写当前实现，不把未来计划写成既成事实。
- 文档应简明，先给结构图，再给关键文件和关键函数。
- 如果引用代码，优先使用相对路径链接。

**最后更新**: 2026-04-22
