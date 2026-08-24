# 模型提供商去 Eino 化

## 背景

当前模型调用链仍以 Eino 为中心：

- `internal/llm/client.go` 通过 `eino-ext` 创建 Claude/OpenAI model adapter。
- `internal/runtime/runtime.go`、`internal/agent`、`internal/context` 使用 `model.ToolCallingChatModel` 和 `schema.Message`。
- 工具注册和工具调用结构也依赖 Eino 的 tool/schema 类型。

如果目标是让本项目完全摆脱字节 Eino 框架，模型 provider / model 这一层可以先收敛到项目自有接口，再逐步把 message、tool call、tool binding 的 Eino 类型移出核心路径。

## 目标状态

- 项目内定义自己的最小模型接口，例如 `Generate`、`Stream`、`BindTools` 或等价能力。
- Claude、OpenAI/Ollama/OpenRouter、Codex 等 provider 由项目自有 adapter 直接调用上游 HTTP/API。
- Agent、Runtime、Context、Session 只依赖项目内 message/tool call 类型。
- Eino 相关代码集中在临时兼容层；完成迁移后可删除 `eino` / `eino-ext` 依赖。

## 第一版边界

- 先只做模型 provider / model 抽象，不顺手重写工具系统、TUI、daemon 或任务系统。
- 保持现有配置语义：`LLM_MODEL=provider/model`、`LLM_<PROVIDER>_FORMAT`、`--model` 等入口继续可用。
- OpenAI-compatible provider 先覆盖 OpenAI、Ollama、OpenRouter；Claude 单独保留 Anthropic 协议 adapter。
- tool call 数据结构优先兼容现有模型输出，避免同时改 prompt 和工具执行语义。
- 压缩、debug request、token 统计等现有能力要能挂到新接口上。

## 建议步骤

1. 盘点 Eino 依赖点：`internal/llm`、`internal/agent`、`internal/context`、`internal/tools` 和文档。
2. 新增项目内 message/tool/model 类型，并写清楚和现有 Eino 类型的一一映射。
3. 先让 `internal/llm` 返回项目自有模型接口，保留一个 Eino adapter 作为过渡实现。
4. 为 OpenAI-compatible stream/tool call 写项目自有 adapter，优先跑通现有 daemon/TUI 主链路。
5. 迁移 Claude provider，再迁移 context 压缩和 debug request。
6. 移除核心包里的 Eino 类型引用；确认 `go.mod` 不再需要 `eino` / `eino-ext` 后删除依赖。

## 验收

- `go list -deps ./... | grep eino` 无输出，或只剩明确标记的临时兼容包。
- `go.mod` 不再直接依赖 `github.com/cloudwego/eino` 和 `github.com/cloudwego/eino-ext`。
- 现有模型配置、TUI 对话、daemon attach、tool call、context 压缩、debug request 仍可用。
- 文档中的项目定位从 Go + Eino runtime 更新为项目自有 Agent runtime。

## 风险和待核验

- Eino 的流式 tool call 分片合并规则需要用真实 provider 输出核对。
- Claude/OpenAI/Ollama 的 tool call schema 差异要保留最小兼容表。
- `schema.Message` 已进入 session JSONL，迁移时要保留旧 session 可读性。
- 如果先改工具系统，diff 会快速膨胀；第一阶段只处理模型 provider 边界。
