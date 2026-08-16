# LLM 调用链路

## 一次 TUI 输入

```text
TUI submit
  -> Agent.RunStream
  -> Context.AddMessage(user)
  -> model.Stream(messages)
  -> collect ToolCalls
  -> execute tools
  -> Context.AddMessage(assistant/tool)
  -> assistant text done
```

## 关键结构

| 结构 | 说明 |
| --- | --- |
| `schema.Message` | role/content/tool_calls/tool_call_id/reasoning/extra |
| `schema.ToolCall` | id/type/function.name/function.arguments |
| `schema.ToolInfo` | 传给 model 的工具 schema |
| `ResponseMeta` | token usage / finish reason |

## ToolCall 合并

流式响应可能拆分 tool name 和 arguments。`toolCollector` 按 index/id 合并，只有 name 非空且 arguments 是合法 JSON 时才发给 worker。

## 工具结果回灌

```text
assistant message with tool_calls
schema.ToolMessage(result, tool_call_id)
```

模型下一轮会同时看到 assistant tool call 和 tool result。

## 非流式路径

`DisableStream=true` 时走 `model.Generate`，用于兼容部分 OpenAI-compatible 供应商。语义仍保持 assistant tool call -> tool result -> 下一轮。
