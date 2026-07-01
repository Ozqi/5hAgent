# 一次 LLM 调用流程：含 Tool Call

> 由 GPT-5.5 于 2026-06-23 阅读 `internal/runtime/runtime.go`、`internal/llm/client.go`、`internal/agent/agent.go`、`internal/agent/tool_use.go`、Eino `schema.Message`、Eino OpenAI adapter 后更新。
> 覆盖范围：5hAgent 一次流式 LLM 调用、工具调用收集/执行/回灌、Eino 框架职责、5hAgent 自身职责、OpenAI-compatible 接口形态。

## 摘要

5hAgent 不直接拼每个 LLM provider 的 HTTP 请求。当前主链路是：

```text
5hAgent Runtime
  -> llm.NewClient(provider config)
  -> Eino ToolCallingChatModel
  -> Agent.RunStream(messages)
  -> Eino provider adapter 转成原生 API 请求
  -> provider 返回流式 message chunk
  -> Eino adapter 转回 schema.Message / schema.ToolCall
  -> 5hAgent 收集 tool call、执行工具、写回 tool result
  -> 下一轮 LLM 继续推理
```

核心边界：

- **Eino 框架**负责抽象统一的 `model.ToolCallingChatModel`、`schema.Message`、`schema.ToolCall`，并由 provider adapter 负责原生请求/响应转换。
- **5hAgent**负责 runtime 初始化、prompt 注入、上下文管理、ReAct 循环、工具注册、工具执行、工具结果回灌、日志与报告。
- **LLM provider / adapter**负责实际模型推理、流式输出、把原生工具调用转换成 Eino tool call。

## 关键类型与参数

### `llm.Config`：5hAgent 的 provider 初始化配置

位置：[`internal/llm/client.go`](../internal/llm/client.go)

```go
type Config struct {
    Provider             string // claude / openai
    APIKey               string // API Key；Ollama OpenAI-compatible 本地模式可填 dummy
    BaseURL              string // provider endpoint，例如 Anthropic 或 http://localhost:11434/v1
    Model                string // 模型名称
    MaxTokens            int    // 最大生成 token 数
    ThinkingBudgetTokens int    // Claude extended thinking 预算；OpenAI 忽略；0 表示关闭
}
```

来源：`utils.LoadConfig()` 从 `~/.5hAgent/.env` 读取当前 provider 的配置，例如：

```env
LLM_MODEL=ollama/hf.co/bartowski/Qwen_Qwen3.6-27B-GGUF:Q3_K_M
LLM_OLLAMA_FORMAT=openai
LLM_OLLAMA_BASE_URL=http://localhost:11434/v1
LLM_OLLAMA_API_KEY=dummy
LLM_OLLAMA_MAX_TOKENS=4096
```

### `schema.Message`：Eino 统一消息结构

Eino 中的关键字段：

```go
type Message struct {
    Role             RoleType
    Content          string
    ReasoningContent string
    ToolCalls        []ToolCall
    ResponseMeta     *ResponseMeta
}
```

角色：

```go
System    = "system"
User      = "user"
Assistant = "assistant"
Tool      = "tool"
```

5hAgent 会把系统 prompt、用户输入、assistant tool call、tool result 都放入 `messageCtx` 中，下一轮调用时全部传给模型。

### `schema.ToolCall`：Eino 统一工具调用结构

Eino 定义：

```go
type ToolCall struct {
    Index    *int
    ID       string
    Type     string
    Function FunctionCall
}

type FunctionCall struct {
    Name      string
    Arguments string // JSON string
}
```

重要字段：

| 字段 | 含义 | 谁提供 |
| --- | --- | --- |
| `Index` | 流式/多工具调用时用于合并分片 | provider adapter / Eino |
| `ID` | tool call 与 tool result 的配对 ID | provider 或 5hAgent 补齐 |
| `Function.Name` | 工具名，例如 `base.list_dir` | LLM 输出，经 adapter 转换 |
| `Function.Arguments` | 工具参数 JSON 字符串 | LLM 输出，经 adapter 转换 |

`ID` 的作用：assistant 消息中发出 tool call 后，tool result 必须用同一个 ID 回灌，模型下一轮才能知道哪个结果对应哪个调用。

## 启动阶段：模型和工具如何绑定

入口：[`runtime.New`](../internal/runtime/runtime.go)

关键步骤：

1. `utils.LoadConfig()` 读取 provider 配置。
2. `llm.NewClient(ctx, llmConfig)` 创建 Eino `ToolCallingChatModel`。
3. `utils.LoadSystemPrompt(promptDir, provider, model)` 加载 `main.md`，可选叠加模型 prefix。
4. `tools.InitRegistry(...)` 初始化工具注册表。
5. `tools.GetAllTools()` 取出所有本地/MCP/Skill/Task 工具。
6. 每个工具调用 `Info(ctx)` 生成 Eino `schema.ToolInfo`。
7. `client.GetModel().WithTools(toolInfos)` 绑定工具。
8. `ag.SetModel(modelWithTools)` 和 `ag.SetTools(allTools)` 注入 Agent。

简化伪代码：

```go
appConfig := utils.LoadConfig()
client := llm.NewClient(ctx, &llm.Config{...})

systemPrompt := utils.LoadSystemPrompt(promptDir, appConfig.LLM.Provider, appConfig.LLM.Model)
ag := agent.NewAgent(... SystemPrompt: systemPrompt ...)

toolInfos := []*schema.ToolInfo{}
for _, t := range tools.GetAllTools() {
    info, _ := t.Info(ctx)
    toolInfos = append(toolInfos, info)
}
modelWithTools, _ := client.GetModel().WithTools(toolInfos)
ag.SetModel(modelWithTools)
ag.SetTools(allTools)
```

责任划分：

| 步骤 | 谁完成 |
| --- | --- |
| 读取 `.env` | 5hAgent |
| 根据 provider 创建模型 adapter | 5hAgent 调 Eino adapter |
| 把工具定义转成 provider 原生 tools | Eino adapter |
| 本地工具注册与执行 | 5hAgent |

## 一次 `RunStream` 的 ReAct 流程

入口：[`Agent.RunStream`](../internal/agent/agent.go)

### 1. 注入系统上下文

首次对话时：

```go
a.ensureConversationSetup(messageCtx)
```

会注入：

- `System` message：`SystemPrompt`
- Skill 注入的 system messages，如果有

### 2. 添加用户消息

```go
userMsg := &schema.Message{
    Role:    schema.User,
    Content: input,
}
a.ctxManager.AddMessage(messageCtx, userMsg)
```

### 3. 开始一轮 LLM stream

```go
messages, _ := a.ctxManager.GetMessages(messageCtx)
reader, err := a.model.Stream(streamCtx, messages)
```

这里 `a.model` 是已经 `WithTools(toolInfos)` 后的 Eino 模型。

### 4. 读取流式 chunk

每个 chunk 是一个 `*schema.Message`，可能包含：

- `chunk.Content`：普通 assistant 文本
- `chunk.ReasoningContent`：thinking/reasoning 文本
- `chunk.ToolCalls`：工具调用分片或完整工具调用
- `chunk.ResponseMeta`：token usage / finish reason 等

5hAgent 做的事：

```go
if len(chunk.ToolCalls) > 0 {
    for _, tc := range collector.Add(chunk.ToolCalls) {
        queuedCalls = append(queuedCalls, tc)
        toolQueue <- toolRequest{idx: idx, tc: tc}
    }
}

if chunk.Content != "" {
    fullContent.WriteString(chunk.Content)
}
```

### 5. 工具调用收集与本地 ID 补齐

位置：[`internal/agent/tool_use.go`](../internal/agent/tool_use.go)

`toolCollector` 用 `Index` 合并流式工具调用分片：

```go
type toolCallState struct {
    tc         schema.ToolCall
    dispatched bool
}

type toolCollector struct {
    states map[int]*toolCallState
    byID   map[string]int
}
```

合并字段：

- `ID`
- `Function.Name`
- `Function.Arguments`

不同 OpenAI-compatible 上游对 tool call `id` 的支持并不完全一致；本地模型或兼容网关可能返回空 ID。5hAgent 会在收集阶段补：

```go
if tc.ID == "" && tc.Function.Name != "" {
    tc.ID = fmt.Sprintf("call_local_%d", idx)
}
```

这一步是 **5hAgent 完成的**，不是 Eino 或上游原生完成的。

### 6. 执行工具

工具执行由后台 goroutine 消费 `toolQueue`：

```go
result, execErr := a.exeToolCall(ctx, req.tc, req.idx, req.idx+1, false)
toolResultCh <- execResult{idx: req.idx, tc: req.tc, result: result, err: execErr}
```

`exeToolCall` 做：

1. 根据 `tc.Function.Name` 找工具：`a.toolMap[name]`
2. 把 `tc.Function.Arguments` 作为 JSON 参数传给工具
3. 收集工具结果或错误
4. 写 debug 日志和 callback

工具参数例子：

```json
{
  "path": "/Users/bytedance/Proj/5hAgent/5hWorkSpace"
}
```

工具名例子：

```text
base.list_dir
base.read_file
task.task
```

### 7. 写回 assistant tool call message

如果本轮有 tool call，5hAgent 会先把 assistant message 放入上下文：

```go
finalMessage := &schema.Message{
    Role:      schema.Assistant,
    Content:   content,
    ToolCalls: toolCalls,
}
a.ctxManager.AddMessage(messageCtx, finalMessage)
```

即使 `Content` 为空，只要有 tool call，也必须加入上下文。否则下一条 tool result 会找不到它对应的 assistant tool call。

### 8. 写回 tool result message

每个工具结果通过：

```go
schema.ToolMessage(result, tc.ID)
```

加入上下文。

关键是：

```text
assistant.ToolCalls[i].ID == tool.ToolCallID
```

例如：

```text
assistant tool call id = call_local_0
tool result tool_call_id = call_local_0
```

### 9. 进入下一轮 ReAct

工具结果写回后：

```go
continue
```

进入下一轮：

```text
Turn 2:
  messages = system + user + assistant(tool_call) + tool(result)
  model.Stream(messages)
```

模型基于工具结果继续输出：

- 可能再次调用工具
- 或输出最终自然语言回答

### 10. 没有工具调用时返回最终内容

如果本轮没有 tool call：

```go
return content, nil
```

无头模式会把该内容写入报告。

## 一次成功日志示例

从本地 Qwen/OpenAI-compatible 接口验证日志：

```text
[DEBUG][REACT] Turn 1
[DEBUG][LLM] Start: messages=2
[DEBUG][STREAM] Chunk#31: tools=1
[DEBUG][STREAM]   [0] id= name=base.list_dir
[DEBUG][COLL] Add chunk: id= name=base.list_dir args="{...}"
[DEBUG][TOOL] Start: name=base.list_dir
[DEBUG][TOOL] End: name=base.list_dir result=...
[DEBUG][TOOL] Tool calls: 1
[DEBUG][STREAM]   [0] id=call_local_0 name=base.list_dir
[DEBUG][TOOL] Success: base.list_dir
[DEBUG][REACT] Turn 2
[DEBUG][LLM] Start: messages=4
...
[DEBUG][REACT] Turn 3
[DEBUG][LLM] End: prompt=3662 completion=157 total=3819
```

关键点：

- 本地兼容接口可能返回空 `id=`。
- 5hAgent 补成 `call_local_0`。
- 工具执行成功后进入 Turn 2。
- 最终 Turn 3 输出报告正文。

## OpenAI-compatible 接口是什么样

当前 `openai` provider 通过 Eino OpenAI adapter 访问 OpenAI-compatible Chat Completions 接口。Ollama 的 OpenAI-compatible endpoint 也是这个形态。

### 请求：Chat Completions

对应 HTTP：

```http
POST /v1/chat/completions
Content-Type: application/json
```

请求体大致：

```json
{
  "model": "hf.co/bartowski/Qwen_Qwen3.6-27B-GGUF:Q3_K_M",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "stream": true,
  "max_tokens": 4096,
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "base.list_dir",
        "description": "...",
        "parameters": {
          "type": "object",
          "properties": {
            "path": {"type": "string"}
          },
          "required": ["path"]
        }
      }
    }
  ]
}
```

### Tool call

OpenAI-compatible tool call 通常是：

```json
{
  "id": "call_xxx",
  "type": "function",
  "function": {
    "name": "base.list_dir",
    "arguments": "{\"path\":\".\"}"
  }
}
```

规范接口包含 `id`，但本地兼容实现可能缺失或不稳定。5hAgent 对空 ID 仍会补 `call_local_<idx>`，保证后续 tool result 可配对。

## Claude / Anthropic 原生接口差异

Claude 原生 Messages API 的工具调用是 content block 形态，典型结构：

```json
{
  "role": "assistant",
  "content": [
    {
      "type": "tool_use",
      "id": "toolu_xxx",
      "name": "base.list_dir",
      "input": {"path": "/tmp"}
    }
  ]
}
```

工具结果回传：

```json
{
  "role": "user",
  "content": [
    {
      "type": "tool_result",
      "tool_use_id": "toolu_xxx",
      "content": "..."
    }
  ]
}
```

Claude 原生 tool use 有 `id`，所以 Eino Claude adapter 可以把它映射到 `schema.ToolCall.ID`。OpenAI-compatible tool call 也通常有 `id`，但本地兼容网关不一定稳定返回，所以 5hAgent 仍保留空 ID 补齐逻辑。

## 责任边界总表

| 环节 | Eino 框架/adapter | 5hAgent | Provider 原生接口 |
| --- | --- | --- | --- |
| 统一消息结构 | 定义 `schema.Message` | 使用并保存到上下文 | 各 provider 有自己的 JSON |
| 统一工具结构 | 定义 `schema.ToolCall` / `ToolInfo` | 注册本地工具并提供 tool schema | Claude/OpenAI-compatible 各自不同 |
| 创建模型 | 提供 `ToolCallingChatModel` 接口和 provider adapter | 根据 `.env` 选择 adapter | 实际 HTTP API |
| 绑定工具 | `WithTools(toolInfos)` 转 provider 原生 tools | 收集 `tools.GetAllTools()` | 原生 `tools` 或 tool_use schema |
| 流式输出 | `Stream()` 返回 `StreamReader[*schema.Message]` | 循环读取 chunk | SSE/JSON stream |
| 工具调用 ID | Claude/OpenAI adapter 尽量映射上游 ID | 对空 ID 补 `call_local_<idx>` | 兼容网关可能缺失 ID |
| 工具执行 | 不执行本地业务工具 | `exeToolCall` 调用本地 tool | Provider 不执行本地工具 |
| 工具结果回灌 | 提供 `schema.ToolMessage` | 写入 message context | 下一轮请求中被 adapter 转原生消息 |
| ReAct loop | 不管理业务循环 | 5hAgent 控制 turn、continue、最终报告 | 每次只处理一次模型请求 |
| 日志/报告 | 无业务日志 | debug log、task report | 无 |

## 当前实现中的关键设计点

### 1. 空正文 + tool call 是有效 assistant message

本地模型常返回：

```text
content=""
tool_calls=[...]
```

这不是空消息。5hAgent 必须把它加入上下文，否则 tool result 无法配对。

### 2. Tool call ID 是框架配对字段，不一定来自模型

Claude 原生有 ID；OpenAI-compatible 通常有 ID，但兼容网关可能缺失。
所以对空 ID：

```text
Function.Name + Function.Arguments 完整
但 ID 为空
```

时，5hAgent 本地生成：

```text
call_local_0
call_local_1
```

### 3. Provider adapter 会丢失/转换部分原生细节

5hAgent 看到的是 Eino 统一结构，不是原生 JSON。  
例如 OpenAI-compatible 原生 `tool_calls` 会被 adapter 映射成统一 `ToolCall`；Claude 原生 `tool_use` block 也会被 adapter 映射成统一 `ToolCall`。

### 4. 5hAgent 的工具名是内部注册名

日志里的：

```text
base.list_dir
base.read_file
task.task
```

来自 5hAgent 工具注册表，不是 provider 自带能力。provider 只决定是否按 schema 生成调用。

## 一句话总结

一次带工具的 LLM 调用中，provider 只负责“根据 messages + tools 生成 assistant 文本或工具调用”；Eino 负责“把不同 provider 的原生 JSON 统一成 `schema.Message`/`schema.ToolCall`”；5hAgent 负责“维护 ReAct 循环、执行本地工具、补齐缺失的 tool call ID、把 tool result 写回上下文并继续下一轮”。
