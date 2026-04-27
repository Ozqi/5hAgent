# Agent

```text
cmd/5hagent/main.go
  -> agent.NewAgent(...)
  -> tools.InitRegistry(...)
  -> Agent.RunStream(...)
     -> model.Stream()
     -> tool_use.go: streamToolCollector / executeToolCall()
     -> TaskList / Context updates
```

## 位置

- [`cmd/5hagent/main.go`](../cmd/5hagent/main.go)
- [`internal/agent/agent.go`](../internal/agent/agent.go)
- [`internal/agent/tool_use.go`](../internal/agent/tool_use.go)
- [`internal/task/tasklist.go`](../internal/task/tasklist.go)
- [`internal/task/task_actions.go`](../internal/task/task_actions.go)

## 入口

启动主链路在 [`runInteractive()`](../cmd/5hagent/main.go#L41-L214)。关键代码是：

```go
systemPrompt, err := utils.Load("prompt", "main")
ag, err := agent.NewAgent(nil, nil, agentConfig)
if err := tools.InitRegistry(taskList, ag.GetSkillManager()); err != nil { ... }
modelWithTools, err := client.GetModel().WithTools(toolInfos)
ag.SetModel(modelWithTools)
ag.SetTools(allTools)
_, err = ag.RunStream(ctx, messageCtx, line, onToken)
```

对应代码：

- [`utils.Load("prompt", "main")`](../cmd/5hagent/main.go#L70-L76)
- [`agent.NewAgent(nil, nil, agentConfig)`](../cmd/5hagent/main.go#L78-L92)
- [`tools.InitRegistry(...)`](../cmd/5hagent/main.go#L94-L98)
- [`client.GetModel().WithTools(toolInfos)`](../cmd/5hagent/main.go#L111-L116)
- [`ag.RunStream(...)`](../cmd/5hagent/main.go#L183-L207)

这里完成三件事：加载主提示词，构造 Agent，给模型绑定工具，然后进入 REPL。

## Agent 结构

`Agent` 本身只持有运行主循环所需的核心依赖，定义在 [`Agent struct`](../internal/agent/agent.go#L17-L33)。

```go
type Agent struct {
    model        model.ToolCallingChatModel
    tools        []tool.BaseTool
    toolMap      map[string]tool.BaseTool
    config       *Config
    state        *State
    ctxManager   *agentctx.Manager
    skillManager *skill.Manager
}
```

初始化逻辑在 [`NewAgent()`](../internal/agent/agent.go#L49-L94)。关键代码是：

```go
toolMap := make(map[string]tool.BaseTool)
for _, t := range tools {
    info, err := t.Info(context.Background())
    if err != nil {
        continue
    }
    toolMap[info.Name] = t
}

skillMgr := skill.NewManager(".5hagent/skills")
if err := skillMgr.LoadSkills(); err != nil { ... }
```

`toolMap` 让后续 `findTool(name)` 变成 O(1) 查找，`skillManager` 则负责首次对话时的 skill 注入。

## Run

> TODO: 已经注释掉了。

非流式主循环在 [`Run()`](../internal/agent/agent.go#L96-L218)。它是最容易读懂的 ReAct 基线实现。

关键代码：

```go
messages, _ := a.ctxManager.GetMessages(messageCtx)
resp, err := a.model.Generate(ctx, messages)

if len(resp.ToolCalls) > 0 {
    assistantMsg := &schema.Message{
        Role:      schema.Assistant,
        Content:   resp.Content,
        ToolCalls: resp.ToolCalls,
    }
    if err := a.ctxManager.AddMessage(messageCtx, assistantMsg); err != nil { ... }
    if err := a.exeTools(ctx, messageCtx, resp.ToolCalls); err != nil { ... }
    continue
}
```

对应代码：

- [`a.ctxManager.GetMessages(messageCtx)`](../internal/agent/agent.go#L156-L160)
- [`resp, err := a.model.Generate(ctx, messages)`](../internal/agent/agent.go#L168-L174)
- [`assistantMsg := &schema.Message{... ToolCalls: resp.ToolCalls}`](../internal/agent/agent.go#L185-L193)
- [`a.exeTools(ctx, messageCtx, resp.ToolCalls)`](../internal/agent/agent.go#L195-L198)

这里的顺序很关键：先把带 `ToolCalls` 的 `assistant` 消息写进上下文，再执行工具。下一轮模型要靠这条消息和后续 `tool` 消息配对。

## RunStream

默认 REPL 走的是 [`RunStream()`](../internal/agent/agent.go#L346-L536)。它比 `Run()` 多了一层“边接收 chunk，边收敛 tool call，再按顺序执行工具”的流式控制。

核心入口代码：

- [`reader, err := a.model.Stream(ctx, messages)`](../internal/agent/agent.go#L414-L420)
- [`chunk, err := reader.Recv()`](../internal/agent/agent.go#L440-L449)

流式路径的关键不是 token 输出，而是把分片 `ToolCall` 收敛成“可执行”的完整调用。当前代码把这块收口进 [`streamToolCollector`](../internal/agent/tool_use.go)：

```go
func (c *streamToolCollector) Add(chunks []schema.ToolCall) []schema.ToolCall {
    for _, tc := range chunks {
        c.merge(tc)
    }

    ready := make([]schema.ToolCall, 0)
    for _, state := range c.states {
        if state.dispatched || !isRunnableToolCall(state.call) {
            continue
        }
        state.dispatched = true
        ready = append(ready, state.call)
    }
    return ready
}
```

对应代码：

- [`streamToolCollector`](../internal/agent/tool_use.go)
- [`if len(chunk.ToolCalls) > 0 { ... }`](../internal/agent/agent.go#L457-L470)

> 注意，由于chunk里包含的toolcall信息可能也不完整

这里的“可执行”不是只看有无 `ToolCall`，还要求 `ID`、`Name` 和一段完整 JSON 参数都已经到位。判断方法: [`isRunnableToolCall()`](../internal/agent/tool_use.go)：

```go
func isRunnableToolCall(tc schema.ToolCall) bool {
    if tc.ID == "" || tc.Function.Name == "" {
        return false
    }
    return isValidJSON(tc.Function.Arguments)
}
```

- stream 还在继续时，工具已经可以开始执行
- 下一轮 ReAct 只有在本轮 stream EOF、assistant 消息落上下文、tool result 全部写回后才会开始
- 流结束后，代码先收回已经执行完的工具结果，再构造最终 `assistant` 消息并写回上下文：

# Tool_Use

工具执行层位于 [`internal/agent/tool_use.go`](../internal/agent/tool_use.go)，核心职能：

1. 流式收集：把 LLM stream 碎片拼成完整 ToolCall
2. 分类调度：只读工具并发、写工具串行
3. 调用适配：`EnhancedInvokableTool` 和 `InvokableTool` 两种接口
4. 结果格式化：ToolResult 压平成字符串 + 错误提示注入

当某个工具调用变成 runnable，`RunStream()` 就把它送进本轮的顺序执行队列：

```go
// 创建带缓冲的 channel，用于异步执行工具调用，缓冲大小为8
toolQueue := make(chan streamToolRequest, 8)

// 启动 goroutine 从队列中消费并执行工具调用
go func() {
    for req := range toolQueue {
        result, execErr := a.executeToolCall(ctx, req.tc, req.idx, req.idx+1, false)
        toolResultCh <- streamToolResult{idx: req.idx, tc: req.tc, result: result, err: execErr}
    }
}()

for _, tc := range collector.Add(chunk.ToolCalls) {
    idx := len(queuedCalls)
    queuedCalls = append(queuedCalls, tc)
    toolQueue <- streamToolRequest{idx: idx, tc: tc}
}
```

对应代码：

- [`toolQueue` / `toolResultCh`](../internal/agent/agent.go#L428-L438)
- [`collector.Add(chunk.ToolCalls)`](../internal/agent/agent.go#L465-L469)
- [`executeToolCall()`](../internal/agent/tool_use.go)

```go
close(toolQueue)

toolCalls := collector.RunnableCalls()
toolResults := make([]streamToolResult, len(queuedCalls))
for res := range toolResultCh {
    toolResults[res.idx] = res
}

finalMessage := &schema.Message{
    Role:      schema.Assistant,
    Content:   content,
    ToolCalls: toolCalls,
}

if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil { ... }
for _, res := range toolResults {
    if err := a.addToolResultToContext(messageCtx, res.tc, res.result, res.err); err != nil { ... }
}
```

对应代码：

- [`close(toolQueue)`](../internal/agent/agent.go#L480-L481)
- [`toolCalls := collector.RunnableCalls()`](../internal/agent/agent.go#L486-L490)
- [`finalMessage := &schema.Message{...}`](../internal/agent/agent.go#L492-L496)
- [`a.ctxManager.AddMessage(messageCtx, finalMessage)`](../internal/agent/agent.go#L507-L509)
- [`a.addToolResultToContext(...)`](../internal/agent/agent.go#L511-L515)

这些字段不是 5hAgent 自己发明的，而是 Eino 框架的 `schema.ToolCall` / `schema.FunctionCall`。框架定义见：

- [`schema.Message`](https://pkg.go.dev/github.com/cloudwego/eino/schema#Message)
- [`schema.ToolCall`](https://pkg.go.dev/github.com/cloudwego/eino/schema#ToolCall)
- [`schema.FunctionCall`](https://pkg.go.dev/github.com/cloudwego/eino/schema#FunctionCall)
- [`schema.ToolMessage`](https://pkg.go.dev/github.com/cloudwego/eino/schema#ToolMessage)

关键定义可以概括成：

```go
type Message struct {
    Role       RoleType
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string
}

type ToolCall struct {
    Index    *int
    ID       string
    Type     string
    Function FunctionCall
}

type FunctionCall struct {
    Name      string
    Arguments string
}
```

对应语义很简单：

- `tc.ID`：这次工具调用的唯一标识，用来和后面的 `tool` 结果消息配对
- `tc.Function.Name`：模型要调用的工具名，比如 `base.read_file`、`base.exec_shell`
- `tc.Function.Arguments`：工具参数，类型是 JSON 字符串，不是已经解析好的 Go struct
- `tc.Index`：框架给流式多工具调用合并预留的位置索引；Eino 文档明确说它在 stream mode 下用于识别分片、辅助合并

也就是说，`RunStream()` 里所谓“合并碎片的 tool call”，本质上是在把多个 chunk 里的 `ToolCall` 片段重新拼回一个完整的：`ID + Name + JSON arguments`。

## ID 从哪里来

当前项目的 LLM 客户端在 [`internal/llm/client.go`](../internal/llm/client.go#L92-L97) 里使用的是 `github.com/cloudwego/eino-ext/components/model/claude` 适配层。所以这里的 `ToolCall` 不是 5hAgent 自己构造的原始协议对象，而是 Claude 适配层先把 provider 响应转成了 Eino 的 `schema.ToolCall`。

> Claude SDK 原始 `tool_use` 结构本身就带 `id`。SDK 定义见 [`anthropic.ToolUseBlock`](https://pkg.go.dev/github.com/anthropics/anthropic-sdk-go#ToolUseBlock)，关键字段是：

```go
type ToolUseBlock struct {
    ID    string
    Input json.RawMessage
    Name  string
    Type  constant.ToolUse
}
```

Eino Claude 适配层会把这几个字段映射成 `schema.ToolCall`。对应实现就在 `claude.go` 的 `toolEvent(...)` 里，核心代码是：

```go
return schema.ToolCall{
    Index: toolIndex,
    ID:    toolCallID,
    Function: schema.FunctionCall{
        Name:      toolName,
        Arguments: arguments,
    },
}
```

这说明两件事：

- `ID` 不是 5hAgent 生成的，是 provider 返回的 tool-use id，经由 Claude 适配层原样放进 `schema.ToolCall.ID`
- `Index` 才是适配层在流式过程中额外维护的辅助字段，用来标识第几个工具调用

所以就当前这条链路来说，`ID` 的来源是“LLM/provider 原始响应”，不是框架临时补的。

## 原始格式到框架格式

在当前 Anthropics/Claude 兼容链路里，可以把格式理解成下面这样。

provider 原始 `tool_use` 大致是：

```json
{
  "type": "tool_use",
  "id": "call_xxx",
  "name": "base.exec_shell",
  "input": { "command": "pwd" }
}
```

进入 Eino 后会变成：

```go
schema.ToolCall{
    ID: "call_xxx",
    Function: schema.FunctionCall{
        Name:      "base.exec_shell",
        Arguments: `{"command":"pwd"}`,
    },
}
```

注意这里 `input` 已经被压成了 JSON 字符串，放在 `Function.Arguments` 里。这也是为什么 5hAgent 在调用工具时，要么自己解析 JSON，要么把整段字符串交给 `InvokableRun(...)`。

## tool 结果为什么要带同一个 ID

工具结果写回上下文时，5hAgent 最终调用的是：

```go
resultMsg := schema.ToolMessage(result, tc.ID)
```

对应代码：

- [`resultMsg := schema.ToolMessage(result, tc.ID)`](../internal/agent/tool_use.go)

Claude 适配层再把这条 `tool` 消息转回 provider 请求时，用的是同一个 id：

```go
anthropic.NewToolResultBlock(message.ToolCallID, message.Content, false)
```

也就是说，链路是闭环的：

- provider 返回 `tool_use.id`
- eino框架的 Claude 适配层映射成 `schema.ToolCall.ID`
- 5hAgent 执行工具后把它写进 `schema.ToolMessage(..., tc.ID)`
- Claude 适配层再把 `ToolCallID` 映射回 provider 的 `tool_result.tool_use_id`

> 为什么消息顺序不能错。模型必须先看到自己的 `tool_use(id=call_xxx)`，下一轮才能接受 `tool_result(tool_use_id=call_xxx)`。

## tool_use

### 流式收集过程

LLM stream 返回的 chunk 里 `ToolCall` 可能是碎片——比如这一帧只有 `ID`，下一帧才有 `Name`，再下一帧才收到完整的 JSON 参数。所以需要一个收集器把碎片拼完整。

收集器结构：

```go
type streamToolState struct {
    call       schema.ToolCall  // 当前累积状态
    dispatched bool             // 是否已发给执行队列
}

type streamToolCollector struct {
    states []*streamToolState   // 所有追踪中的 tool call
    byID   map[string]int     // ID → states 索引，快速查找
}
```

**合并逻辑** `mergeToolCall` 分三种情况把碎片拼进去：

```go
func mergeToolCall(dst *schema.ToolCall, src schema.ToolCall) {
    // 1. ID 只赋值一次
    if dst.ID == "" && src.ID != "" {
        dst.ID = src.ID
    }
    // 2. Name 只赋值一次
    if src.Function.Name != "" && dst.Function.Name == "" {
        dst.Function.Name = src.Function.Name
    }
    // 3. Arguments 追加（streaming 可能分多次收到）
    if src.Function.Arguments != "" {
        dst.Function.Arguments += src.Function.Arguments
    }
}
```

**入队逻辑** `Add` 每次收到新 chunk 时：

1. 对每个 chunk 调用 `merge`——有 ID 则查找或追加，无 ID 则追加到最后一个状态
2. 遍历所有状态，把 `dispatched == false && isRunnableToolCall == true` 的标记为已分发并返回

**可执行判断** `isRunnableToolCall` 要求三元组齐全：

```go
func isRunnableToolCall(tc schema.ToolCall) bool {
    if tc.ID == "" || tc.Function.Name == "" { return false }
    return isValidJSON(tc.Function.Arguments)
}
```

也就是 `ID` 非空 + `Name` 非空 + `Arguments` 是合法 JSON。三项缺一则继续等下一个 chunk。

**一个具体例子**：

假设 LLM streaming 分四帧返回同一个 tool call：

```
frame 1: ToolCall{ID: "call_001"}
frame 2: ToolCall{Function{Name: "read_file"}}
frame 3: ToolCall{Function{Arguments: `{"path"`}
frame 4: ToolCall{Function{Arguments: `{"path":"/tmp/a.txt"}`}
```

- frame 1 入队，`byID["call_001"] = 0`，`states[0].call.ID = "call_001"`
- frame 2 合并到 `states[0]`，`Name` 填上
- frame 3/4 持续追加 `Arguments`，直到 JSON 完整合法
- JSON 一旦 valid，`isRunnableToolCall` 返回 `true`，下一帧 `Add` 就会把它摘出来送进执行队列

流结束后用 `RunnableCalls()` 把剩余已拼完但未分发的 tool call 全部取出。

### 分类：只读 vs 写

`isReadOnlyToolCall` 分两层判断：

```go
func isReadOnlyToolCall(tc schema.ToolCall) bool {
    // 第一层：toolmeta 注册表里的 ReadOnly 标记
    if toolmeta.IsReadOnly(tc.Function.Name) { return true }

    // 第二层：task.task 工具根据 action 参数判断
    if tc.Function.Name == "task.task" || tc.Function.Name == "task" {
        var input struct { Action string `json:"action"` }
        if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
            return false
        }
        return input.Action == "get" || input.Action == "list"
    }
    return false
}
```

结果：

- 只读工具（`read_file` / `glob` / `grep` / `list_dir` 等）：并发执行
- 写工具（`write_file` / `edit` / `exec_shell` 等）：串行执行
- `task.task get` / `list`：并发；其他 action：串行

### 并发执行（只读）

```go
func (a *Agent) exeToolsConcurrent(...) error {
    results := make(chan streamToolResult, len(toolCalls))

    for idx, tc := range toolCalls {
        go func(idx int, tc schema.ToolCall) {
            result, execErr := a.executeToolCall(ctx, tc, idx, len(toolCalls), true)
            results <- streamToolResult{idx: idx, tc: tc, result: result, err: execErr}
        }(idx, tc)
    }

    // 按原始顺序收集结果
    collectedResults := make([]streamToolResult, len(toolCalls))
    for i := 0; i < len(toolCalls); i++ {
        collectedResults[i] = <-results
    }

    // 仍按顺序写回上下文
    for _, res := range collectedResults {
        a.addToolResultToContext(messageCtx, res.tc, res.result, res.err)
    }
}
```

关键约束：**执行并发，但写回有序**。goroutine 谁先完成不确定，但 `collectedResults[res.idx] = res` 保证了第 N 个请求的结果一定落在第 N 位，最终按 ID 顺序写进上下文。

### 串行执行（写）

写工具直接 for 循环顺序调用，每执行完一个立即写回结果再执行下一个：

```go
for idx, tc := range writeCalls {
    result, execErr := a.executeToolCall(ctx, tc, idx, len(writeCalls), false)
    a.addToolResultToContext(messageCtx, tc, result, execErr)
}
```

这样文件修改和任务状态更新严格按 LLM 生成的顺序进行，不会出现先创建再删除的竞争。

### 调用接口适配

`invokeTool` 根据工具实现的接口分叉：

```go
func (a *Agent) invokeTool(ctx context.Context, t tool.BaseTool, tc schema.ToolCall) (string, error) {
    // EnhancedInvokableTool：框架解码 JSON → Go struct
    if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
        toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}
        toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
        return formatToolResult(toolResult), nil
    }

    // InvokableTool：工具自己解析 JSON 字符串
    if invokable, ok := t.(tool.InvokableTool); ok {
        return invokable.InvokableRun(ctx, tc.Function.Arguments)
    }

    return "", fmt.Errorf("tool %s is not invokable", tc.Function.Name)
}
```

两种接口区别：

| 接口                    | 参数传递                          | JSON 解码                     |
| ----------------------- | --------------------------------- | ----------------------------- |
| `EnhancedInvokableTool` | `schema.ToolArgument{Text: json}` | Eino 框架自动解码到 Go struct |
| `InvokableTool`         | 原始 `string` JSON                | 工具自己 `json.Unmarshal`     |

### 结果格式化

`formatToolResult` 把 `schema.ToolResult` 的多部分压成字符串：

```go
func formatToolResult(toolResult *schema.ToolResult) string {
    var parts []string
    for _, part := range toolResult.Parts {
        switch part.Type {
        case schema.ToolPartTypeText:  parts = append(parts, part.Text)
        case schema.ToolPartTypeImage: parts = append(parts, "[Image]")
        case schema.ToolPartTypeAudio: parts = append(parts, "[Audio]")
        case schema.ToolPartTypeVideo: parts = append(parts, "[Video]")
        case schema.ToolPartTypeFile:  parts = append(parts, "[File]")
        }
    }
    return strings.Join(parts, "\n")
}
```

### 错误提示注入

执行失败时，`formatToolExecutionError` 会附加一条针对性的恢复建议：

```go
func toolFailureHint(tc schema.ToolCall) string {
    display := toolmeta.DisplayName(tc.Function.Name)  // 去掉前缀
    switch display {
    case "read_file", "write_file", "edit", "glob", "grep", "list_dir":
        return "check the tool arguments and retry with an absolute path under the workspace..."
    case "exec_shell":
        return "check the shell command, quote paths with spaces..."
    case "task":
        return "use a valid task action: create, update, get, list, or delete..."
    case "skill":
        return "use an existing skill name and set action to enable or disable..."
    }
    if meta, ok := toolmeta.Lookup(name); ok && meta.Category == toolmeta.CategoryMCP {
        return "check the remote tool arguments and server-specific requirements..."
    }
    return "review the tool schema and retry with corrected arguments."
}
```

最终写入上下文的消息格式：

```
tool execution failed: <error>
Suggestion: <hint>
```

### 写回上下文

结果最终由 `addToolResultToContext` 写入，用 `tc.ID` 与原始请求配对：

```go
errMsg := schema.ToolMessage(formatToolExecutionError(tc, execErr), tc.ID)
resultMsg := schema.ToolMessage(result, tc.ID)
a.ctxManager.AddMessage(messageCtx, msg)
```

Claude 适配层收到这条 `tool` 消息后会提取 `ToolCallID` 映射回 provider 的 `tool_result(tool_use_id=xxx)`，因此链路必须严格保持 `assistant(tool_calls) → tool result(tool_use_id)` 的顺序。

## BaseTool 怎么变成模型可调用的 tool

模型本身并不直接认识 Go 函数，它只认识 `schema.ToolInfo`。这一步在入口处完成：

```go
allTools := tools.GetAllTools()
for _, t := range allTools {
    info, err := t.Info(ctx)
    toolInfos = append(toolInfos, info)
}
modelWithTools, err := client.GetModel().WithTools(toolInfos)
```

对应代码：

- [`tools.GetAllTools()`](../cmd/5hagent/main.go#L100-L109)
- [`info, err := t.Info(ctx)`](../cmd/5hagent/main.go#L102-L108)
- [`client.GetModel().WithTools(toolInfos)`](../cmd/5hagent/main.go#L111-L116)

这里的关键接口是 [`schema.ToolInfo`](https://pkg.go.dev/github.com/cloudwego/eino/schema#ToolInfo)。它包含三部分：

```go
type ToolInfo struct {
    Name string
    Desc string
    *ParamsOneOf
}
```

也就是说，模型看到的其实是：工具名、工具描述、参数 schema。模型据此决定要不要调用工具，以及调用时该生成什么 JSON 参数。

## 参数是怎么从模型传到工具实现的

这条链路可以直接看成四步：

1. 工具实现 `Info()`，把参数 schema 暴露给模型
2. `WithTools(toolInfos)` 把这些 schema 绑定到模型
3. 模型返回 `ToolCall{Function.Name, Function.Arguments}`
4. `invokeTool()` 把 `Function.Arguments` 传给真实工具实现

关键代码串起来就是：

```go
info, err := t.Info(ctx)
modelWithTools, err := client.GetModel().WithTools(toolInfos)
result, execErr := a.invokeTool(ctx, t, tc)
return invokable.InvokableRun(ctx, tc.Function.Arguments)
```

对应代码：

- [`t.Info(ctx)`](../cmd/5hagent/main.go#L102-L108)
- [`WithTools(toolInfos)`](../cmd/5hagent/main.go#L111-L116)
- [`result, execErr := a.invokeTool(ctx, t, tc)`](../internal/agent/tool_use.go)
- [`return invokable.InvokableRun(ctx, tc.Function.Arguments)`](../internal/agent/tool_use.go)

所以模型和工具实现之间传递的“参数载体”就是 `tc.Function.Arguments`，它的类型是 JSON 字符串。

例如模型返回：

```go
schema.ToolCall{
    ID: "call_xxx",
    Function: schema.FunctionCall{
        Name:      "base.read_file",
        Arguments: `{"path":"/tmp/a.txt","offset":1,"limit":20}`,
    },
}
```

5hAgent 不会先在 agent 层把它解成统一 struct，而是把这段 JSON 原样交给具体工具，由工具自己决定怎么解析。

## 两种工具实现方式

当前仓库里的工具主要有两种实现风格。

第一种是 `EnhancedInvokableTool`，常见于 `base.read_file`、`base.write_file`、`base.edit`、`base.exec_shell`。这些工具通常通过 [`utils.InferEnhancedTool`](https://pkg.go.dev/github.com/cloudwego/eino/components/tool/utils#InferEnhancedTool) 构造。关键代码长这样：

```go
return utils.InferEnhancedTool(
    "base.read_file",
    "...",
    func(ctx context.Context, input ReadFileInput) (*schema.ToolResult, error) {
        ...
    },
)
```

对应代码：

- [`NewReadFileTool()`](../internal/tools/read_file.go#L28-L119)
- [`NewWriteFileTool()`](../internal/tools/write_file.go#L28-L78)
- [`NewEditTool()`](../internal/tools/edit.go#L29-L106)
- [`NewExecShellTool()`](../internal/tools/exec_shell.go#L27-L88)

这类工具的特点是：输入 struct 决定参数 schema，框架会自动从 Go struct 推导 JSON schema；运行时 `invokeTool()` 会这样调用：

```go
toolArg := &schema.ToolArgument{
    Text: tc.Function.Arguments,
}
toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
return formatToolResult(toolResult), nil
```

对应代码：

- [`toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}`](../internal/agent/tool_use.go)
- [`return formatToolResult(toolResult), nil`](../internal/agent/tool_use.go)

也就是说，这类工具拿到的是一段 JSON 参数，但 JSON 到 `ReadFileInput` / `WriteFileInput` 的解码由 Eino 帮你做了。

第二种是 `InvokableTool`，常见于 `task` 和 `skill`。这类工具自己实现 `Info()` 和 `InvokableRun()`：

```go
func (t *TaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "task",
        Desc: "Manage tasks...",
        ParamsOneOf: schema.NewParamsOneOfByParams(...),
    }, nil
}

func (t *TaskTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
    var input struct {
        Action string `json:"action"`
        ...
    }
    if err := json.Unmarshal([]byte(args), &input); err != nil { ... }
    ...
}
```

对应代码：

- [`TaskTool.Info()`](../internal/tools/task_tool.go#L22-L54)
- [`TaskTool.InvokableRun()`](../internal/tools/task_tool.go#L56-L83)
- [`SkillTool.Info()`](../internal/tools/skill_tool.go#L22-L39)
- [`SkillTool.InvokableRun()`](../internal/tools/skill_tool.go#L41-L75)

这类工具的 schema 是手写的，参数 JSON 也是工具自己 `json.Unmarshal`。

## 几个核心工具是怎么实现的

`read_file` 用 `ReadFileInput` 定义输入：

```go
type ReadFileInput struct {
    Path   string `json:"path" jsonschema:"required,..."`
    Offset int    `json:"offset,omitempty" ...`
    Limit  int    `json:"limit,omitempty" ...`
}
```

对应代码：

- [`ReadFileInput`](../internal/tools/read_file.go#L15-L20)
- [`NewReadFileTool()`](../internal/tools/read_file.go#L28-L119)

模型只会看到 `path` / `offset` / `limit` 这几个参数；运行时工具函数拿到的是已经解好的 `ReadFileInput`，然后自己 `os.Open`、`bufio.Scanner` 读取文件，再返回 `schema.ToolResult`。

`write_file` 同理，用 `WriteFileInput{Path, Content}` 描述参数 schema，然后在实现里 `os.MkdirAll` + `os.WriteFile`。对应代码：

- [`WriteFileInput`](../internal/tools/write_file.go#L15-L19)
- [`NewWriteFileTool()`](../internal/tools/write_file.go#L28-L78)

`edit` 也是 `EnhancedInvokableTool`，只是输入变成 `Path` / `OldString` / `NewString`，执行时先读文件、检查 `old_string` 是否存在，再整体替换写回。对应代码：

- [`EditInput`](../internal/tools/edit.go#L15-L20)
- [`NewEditTool()`](../internal/tools/edit.go#L29-L106)

`exec_shell` 只暴露一个 `command` 参数：

```go
type ExecShellInput struct {
    Command string `json:"command" jsonschema:"required,..."`
}
```

执行时直接：

```go
cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
stdout, err := cmd.Output()
```

对应代码：

- [`ExecShellInput`](../internal/tools/exec_shell.go#L15-L18)
- [`cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)`](../internal/tools/exec_shell.go#L43-L45)

`task.task` 和 `skill.skill` 则是典型的手写 schema + 手写 JSON 解析。模型生成的仍然是标准 JSON，比如：

```json
{ "action": "list" }
```

或：

```json
{ "skill": "debugging", "action": "enable" }
```

工具收到后自己 `json.Unmarshal`，再调用 `TaskList` 或 `skill.Manager`。

## 注册表怎么把这些工具收集起来

所有工具最终都会进入 [`InitRegistry()`](../internal/tools/registry.go#L15-L52)。关键代码：

```go
tools := []struct {
    meta toolmeta.Meta
    fn   func() (tool.BaseTool, error)
}{
    {meta: toolmeta.Meta{FullName: "base.read_file", DisplayName: "read_file"}, fn: func() (tool.BaseTool, error) { return NewReadFileTool() }},
    {meta: toolmeta.Meta{FullName: "base.exec_shell", DisplayName: "exec_shell"}, fn: func() (tool.BaseTool, error) { return NewExecShellTool() }},
    ...
}

for _, t := range tools {
    tool, err := t.fn()
    registry = append(registry, tool)
}
```

然后在主入口：

```go
allTools := tools.GetAllTools()
for _, t := range allTools {
    info, err := t.Info(ctx)
    toolInfos = append(toolInfos, info)
}
```

这就是从“Go 里的 tool instance”到“模型能理解的 tool schema”的桥。

## invokeTool

真正的接口适配在 [`invokeTool()`](../internal/agent/tool_use.go)。关键代码：

```go
if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
    toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}
    toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
    return formatToolResult(toolResult), nil
}

if invokable, ok := t.(tool.InvokableTool); ok {
    return invokable.InvokableRun(ctx, tc.Function.Arguments)
}
```

对应代码：

- [`toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}`](../internal/agent/tool_use.go)
- [`return formatToolResult(toolResult), nil`](../internal/agent/tool_use.go)
- [`return invokable.InvokableRun(ctx, tc.Function.Arguments)`](../internal/agent/tool_use.go)

也就是说，对话层最终只关心字符串结果；即使工具内部返回的是 richer 的 `ToolResult`，也会先被 `formatToolResult()` 压平成文本。

## 写回上下文

工具结果最终由 [`addToolResultToContext()`](../internal/agent/tool_use.go) 写回。关键代码：

```go
errMsg := schema.ToolMessage(
    fmt.Sprintf("tool execution failed: %v", execErr),
    tc.ID,
)

resultMsg := schema.ToolMessage(result, tc.ID)
```

对应代码：

- [`schema.ToolMessage(..., tc.ID)` error path](../internal/agent/tool_use.go)
- [`resultMsg := schema.ToolMessage(result, tc.ID)`](../internal/agent/tool_use.go)

这里的 `tc.ID` 就是消息配对键。上一条 `assistant` 消息里是 `ToolCall.ID`，这一条 `tool` 消息里是 `ToolCallID`。下一轮模型再次看到整段上下文时，才能知道哪个工具已经执行完。

## TaskList

任务持久化在 [`TaskList`](../internal/task/tasklist.go#L40-L44)。入口是 [`NewTaskList()`](../internal/task/tasklist.go#L51-L62)：

```go
list := &TaskList{path: path, tasks: map[string]*Task{}}
if err := list.load(); err != nil { ... }
if err := list.save(); err != nil { ... }
```

创建和更新的关键点都在"改内存后立刻持久化"：

```go
l.tasks[id] = task
if err := l.saveLocked(); err != nil { ... }
```

对应代码：

- [`CreateTask()`](../internal/task/tasklist.go#L72-L88)
- [`UpdateTaskStatus()`](../internal/task/tasklist.go#L90-L103)
- [`DeleteTask()`](../internal/task/tasklist.go#L132-L143)
- [`save()`](../internal/task/tasklist.go#L267-L271)

任务文件路径默认是项目根 `task.md`，由 [`runInteractive()`](../cmd/5hagent/main.go#L46-L53) 初始化。`TaskList` 会把任务持久化到 `task.md` 的受管 markdown 区块里，并在每次公开读写前重新从磁盘加载，确保人工修改能立即被 `/task` 和 `task.task` 看到。

