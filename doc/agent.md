# Agent

```text
cmd/5hagent/main.go
  -> agent.NewAgent(...)
  -> tools.InitRegistry(...)
  -> Agent.RunStream(...)
     -> model.Stream()
     -> streamToolCollector / executeToolCall()
     -> TaskList / Context updates
```

## 位置

- [`cmd/5hagent/main.go`](../cmd/5hagent/main.go)
- [`internal/agent/agent.go`](../internal/agent/agent.go)
- [`internal/agent/tool_executor.go`](../internal/agent/tool_executor.go)
- [`internal/agent/tasklist.go`](../internal/agent/tasklist.go)

## 入口

启动主链路在 [`runInteractive()`](../cmd/5hagent/main.go#L41-L214)。关键代码是：

```go
systemPrompt, err := prompt.Load("prompt", "main")
ag, err := agent.NewAgent(nil, nil, agentConfig)
if err := tools.InitRegistry(taskList, ag.GetSkillManager()); err != nil { ... }
modelWithTools, err := client.GetModel().WithTools(toolInfos)
ag.SetModel(modelWithTools)
ag.SetTools(allTools)
_, err = ag.RunStream(ctx, messageCtx, line, onToken)
```

对应代码：

- [`prompt.Load("prompt", "main")`](../cmd/5hagent/main.go#L70-L76)
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

```go
reader, err := a.model.Stream(ctx, messages)
for {
    chunk, err := reader.Recv()
    if err == io.EOF {
        break
    }
    ...
}
```

对应代码：

- [`reader, err := a.model.Stream(ctx, messages)`](../internal/agent/agent.go#L414-L420)
- [`chunk, err := reader.Recv()`](../internal/agent/agent.go#L440-L449)

流式路径的关键不是 token 输出，而是把分片 `ToolCall` 收敛成“可执行”的完整调用。当前代码把这块收口进 [`streamToolCollector`](../internal/agent/agent.go#L224-L330)：

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

- [`streamToolCollector`](../internal/agent/agent.go#L224-L330)
- [`if len(chunk.ToolCalls) > 0 { ... }`](../internal/agent/agent.go#L457-L470)

这里的“可执行”不是只看有无 `ToolCall`，还要求 `ID`、`Name` 和一段完整 JSON 参数都已经到位。判断在 [`isRunnableToolCall()`](../internal/agent/agent.go#L312-L318)：

```go
func isRunnableToolCall(tc schema.ToolCall) bool {
    if tc.ID == "" || tc.Function.Name == "" {
        return false
    }
    return isValidJSON(tc.Function.Arguments)
}
```

一旦某个调用变成 runnable，`RunStream()` 就把它送进本轮的顺序执行队列：

```go
toolQueue := make(chan streamToolRequest, 8)
go func() {
    for req := range toolQueue {
        result, execErr := a.executeToolCall(ctx, req.tc, req.idx, req.idx+1)
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
- [`executeToolCall()`](../internal/agent/agent.go#L332-L344)

这个设计的语义是：

- stream 还在继续时，工具已经可以开始执行
- 但工具仍然按发现顺序进入单线程 executor，不会把写工具并发掉
- 下一轮 ReAct 只有在本轮 stream EOF、assistant 消息落上下文、tool result 全部写回后才会开始

流结束后，代码先收回已经执行完的工具结果，再构造最终 `assistant` 消息并写回上下文：

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

顺序约束仍然没变：上下文里一定先写 `assistant(tool_calls)`，再写对应的 `tool` 结果。区别只是工具执行本身可以在 stream 过程中提前开始，结果延迟到 assistant 消息落库后再统一写回。这样既保住 provider 要求的消息顺序，也避免整段响应结束后才开始跑工具。

## ToolCall 是什么

`RunStream()` 里打印的这几个字段：

```go
logger.DebugTag("STREAM", "  [%d] id='%s' name='%s' args='%s'",
    i, tc.ID, tc.Function.Name, tc.Function.Arguments)
```

对应代码：

- [`logger.DebugTag("STREAM", ... tc.ID, tc.Function.Name, tc.Function.Arguments)`](../internal/agent/agent.go#L460-L462)

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
- `tc.Function.Name`：模型要调用的工具名，比如 `read_file`、`exec_shell`
- `tc.Function.Arguments`：工具参数，类型是 JSON 字符串，不是已经解析好的 Go struct
- `tc.Index`：框架给流式多工具调用合并预留的位置索引；Eino 文档明确说它在 stream mode 下用于识别分片、辅助合并

也就是说，`RunStream()` 里所谓“合并碎片的 tool call”，本质上是在把多个 chunk 里的 `ToolCall` 片段重新拼回一个完整的：`ID + Name + JSON arguments`。

## ID 从哪里来

当前项目的 LLM 客户端在 [`internal/llm/client.go`](../internal/llm/client.go#L92-L97) 里使用的是 `github.com/cloudwego/eino-ext/components/model/claude` 适配层。所以这里的 `ToolCall` 不是 5hAgent 自己构造的原始协议对象，而是 Claude 适配层先把 provider 响应转成了 Eino 的 `schema.ToolCall`。

Claude SDK 原始 `tool_use` 结构本身就带 `id`。SDK 定义见 [`anthropic.ToolUseBlock`](https://pkg.go.dev/github.com/anthropics/anthropic-sdk-go#ToolUseBlock)，关键字段是：

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
  "name": "exec_shell",
  "input": {"command": "pwd"}
}
```

进入 Eino 后会变成：

```go
schema.ToolCall{
    ID: "call_xxx",
    Function: schema.FunctionCall{
        Name:      "exec_shell",
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

- [`resultMsg := schema.ToolMessage(result, tc.ID)`](../internal/agent/tool_executor.go#L258-L264)

Claude 适配层再把这条 `tool` 消息转回 provider 请求时，用的是同一个 id：

```go
anthropic.NewToolResultBlock(message.ToolCallID, message.Content, false)
```

也就是说，链路是闭环的：

- provider 返回 `tool_use.id`
- Claude 适配层映射成 `schema.ToolCall.ID`
- 5hAgent 执行工具后把它写进 `schema.ToolMessage(..., tc.ID)`
- Claude 适配层再把 `ToolCallID` 映射回 provider 的 `tool_result.tool_use_id`

这也是为什么消息顺序不能错。模型必须先看到自己的 `tool_use(id=call_xxx)`，下一轮才能接受 `tool_result(tool_use_id=call_xxx)`。

## tool_executor

工具执行入口是 [`exeTools()`](../internal/agent/tool_executor.go#L26-L72)。核心代码：

```go
for _, tc := range toolCalls {
    if tc.Function.Name == "" {
        continue
    }
    if isReadOnlyToolCall(tc) {
        readOnlyCalls = append(readOnlyCalls, tc)
    } else {
        writeCalls = append(writeCalls, tc)
    }
}

if len(readOnlyCalls) > 0 {
    if err := a.exeToolsConcurrent(ctx, messageCtx, readOnlyCalls); err != nil { ... }
}
for idx, tc := range writeCalls {
    if err := a.executeSingleTool(ctx, messageCtx, tc, idx, len(writeCalls), false); err != nil { ... }
}
```

这里先分类，再决定并发还是串行。分类规则在 [`isReadOnlyToolCall()`](../internal/agent/tool_executor.go#L314-L332)。基础名单来自 [`readOnlyTools`](../internal/agent/tool_executor.go#L16-L24)，统一 `task` 工具还会继续解析 `action`，其中 `get` / `list` 走只读，`create` / `update` / `delete` 走写路径。

只读工具走 [`exeToolsConcurrent()`](../internal/agent/tool_executor.go#L74-L130)。关键代码：

```go
go func(idx int, tc schema.ToolCall) {
    t := a.findTool(tc.Function.Name)
    result, execErr := a.invokeTool(ctx, t, tc)
    results <- toolResult{idx: idx, tc: tc, result: result, err: execErr}
}(idx, tc)

for _, res := range collectedResults {
    if err := a.addToolResultToContext(messageCtx, res.tc, res.result, res.err); err != nil { ... }
}
```

对应代码：

- [`t := a.findTool(tc.Function.Name)`](../internal/agent/tool_executor.go#L102-L107)
- [`result, execErr := a.invokeTool(ctx, t, tc)`](../internal/agent/tool_executor.go#L109-L111)
- [`a.addToolResultToContext(...)`](../internal/agent/tool_executor.go#L122-L127)

注意这里是“执行并发，写回顺序稳定”。工具完成得再快，也会按原始请求顺序写回上下文。

写工具走 [`executeSingleTool()`](../internal/agent/tool_executor.go#L160-L198)。关键代码：

```go
t := a.findTool(tc.Function.Name)
result, execErr := a.invokeTool(ctx, t, tc)
return a.addToolResultToContext(messageCtx, tc, result, execErr)
```

串行路径更简单，重点是保持文件修改和任务状态更新的顺序语义。

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
- [`result, execErr := a.invokeTool(ctx, t, tc)`](../internal/agent/tool_executor.go#L193-L197)
- [`return invokable.InvokableRun(ctx, tc.Function.Arguments)`](../internal/agent/tool_executor.go#L228-L231)

所以模型和工具实现之间传递的“参数载体”就是 `tc.Function.Arguments`，它的类型是 JSON 字符串。

例如模型返回：

```go
schema.ToolCall{
    ID: "call_xxx",
    Function: schema.FunctionCall{
        Name:      "read_file",
        Arguments: `{"path":"/tmp/a.txt","offset":1,"limit":20}`,
    },
}
```

5hAgent 不会先在 agent 层把它解成统一 struct，而是把这段 JSON 原样交给具体工具，由工具自己决定怎么解析。

## 两种工具实现方式

当前仓库里的工具主要有两种实现风格。

第一种是 `EnhancedInvokableTool`，常见于 `read_file`、`write_file`、`edit`、`exec_shell`。这些工具通常通过 [`utils.InferEnhancedTool`](https://pkg.go.dev/github.com/cloudwego/eino/components/tool/utils#InferEnhancedTool) 构造。关键代码长这样：

```go
return utils.InferEnhancedTool(
    "read_file",
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

- [`toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}`](../internal/agent/tool_executor.go#L214-L220)
- [`return formatToolResult(toolResult), nil`](../internal/agent/tool_executor.go#L224-L225)

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

`task` 和 `skill` 则是典型的手写 schema + 手写 JSON 解析。模型生成的仍然是标准 JSON，比如：

```json
{"action":"list"}
```

或：

```json
{"skill":"debugging","action":"enable"}
```

工具收到后自己 `json.Unmarshal`，再调用 `TaskList` 或 `skill.Manager`。

## 注册表怎么把这些工具收集起来

所有工具最终都会进入 [`InitRegistry()`](../internal/tools/registry.go#L15-L52)。关键代码：

```go
tools := []struct {
    name string
    fn   func() (tool.BaseTool, error)
}{
    {"read_file", func() (tool.BaseTool, error) { return NewReadFileTool() }},
    {"exec_shell", func() (tool.BaseTool, error) { return NewExecShellTool() }},
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

真正的接口适配在 [`invokeTool()`](../internal/agent/tool_executor.go#L200-L236)。关键代码：

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

- [`toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}`](../internal/agent/tool_executor.go#L214-L220)
- [`return formatToolResult(toolResult), nil`](../internal/agent/tool_executor.go#L224-L225)
- [`return invokable.InvokableRun(ctx, tc.Function.Arguments)`](../internal/agent/tool_executor.go#L228-L231)

也就是说，对话层最终只关心字符串结果；即使工具内部返回的是 richer 的 `ToolResult`，也会先被 `formatToolResult()` 压平成文本。

## 写回上下文

工具结果最终由 [`addToolResultToContext()`](../internal/agent/tool_executor.go#L238-L265) 写回。关键代码：

```go
errMsg := schema.ToolMessage(
    fmt.Sprintf("tool execution failed: %v", execErr),
    tc.ID,
)

resultMsg := schema.ToolMessage(result, tc.ID)
```

对应代码：

- [`schema.ToolMessage(..., tc.ID)` error path](../internal/agent/tool_executor.go#L247-L255)
- [`resultMsg := schema.ToolMessage(result, tc.ID)`](../internal/agent/tool_executor.go#L258-L264)

这里的 `tc.ID` 就是消息配对键。上一条 `assistant` 消息里是 `ToolCall.ID`，这一条 `tool` 消息里是 `ToolCallID`。下一轮模型再次看到整段上下文时，才能知道哪个工具已经执行完。

## TaskList

任务持久化在 [`TaskList`](../internal/agent/tasklist.go#L34-L39)。入口是 [`NewTaskList()`](../internal/agent/tasklist.go#L41-L61)：

```go
tl := &TaskList{
    tasks:    make(map[string]*Task),
    filePath: filePath,
}
if _, err := os.Stat(filePath); err == nil {
    if err := tl.load(); err != nil { ... }
}
```

创建和更新的关键点都在“改内存后立刻持久化”：

```go
tl.tasks[id] = task
if err := tl.save(); err != nil { ... }
```

对应代码：

- [`CreateTask()`](../internal/agent/tasklist.go#L63-L97)
- [`UpdateTaskStatus()`](../internal/agent/tasklist.go#L116-L146)
- [`DeleteTask()`](../internal/agent/tasklist.go#L181-L202)
- [`save()`](../internal/agent/tasklist.go#L204-L226)

任务文件路径默认是 `.5hagent/tasks.json`，由 [`runInteractive()`](../cmd/5hagent/main.go#L50-L57) 初始化。
