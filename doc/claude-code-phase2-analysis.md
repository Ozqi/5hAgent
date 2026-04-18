# Claude Code Phase 2 关键实现分析

> 针对 miniAgent Phase 2 需求：流式输出、上下文管理、工具扩展  
> 分析对象：/home/lzq/Proj/claude-code  
> 当前 miniAgent 状态：Phase 1 完成（基础 ReAct 循环、3个工具、260行 agent.go）

---

## 一、当前 miniAgent 的实现状态

### 1.1 已有代码结构

```
internal/agent/agent.go (562行)
├── Agent 结构体
│   ├── model: LLM 模型
│   ├── tools: 工具列表
│   ├── toolMap: 工具映射表（O(1)查找）
│   ├── config: 配置（SystemPrompt、MaxTurns）
│   ├── state: 运行状态
│   └── ctxManager: 上下文管理器
├── Run(): 同步执行
├── RunStream(): 流式执行（已实现）
└── exeTools(): 工具执行
```

**关键特点**：
- ✅ 已实现流式输出（RunStream）
- ✅ 已有上下文管理器（ctxManager）
- ✅ 工具映射表优化
- ❌ 上下文注入机制不完善（只有 SystemPrompt）
- ❌ 缺少 git 状态、项目规则、memory 注入
- ❌ 缺少上下文压缩

---

## 二、Claude Code 的 QueryEngine 核心设计

### 2.1 QueryEngine 类结构（1295行）

```typescript
export class QueryEngine {
  // 配置（不可变）
  private config: QueryEngineConfig
  
  // 会话状态（可变）
  private mutableMessages: Message[]          // 完整对话历史
  private abortController: AbortController    // 中断控制
  private permissionDenials: SDKPermissionDenial[]  // 权限拒绝记录
  private totalUsage: NonNullableUsage        // token 统计
  private readFileState: FileStateCache       // 文件读取缓存
  private discoveredSkillNames: Set<string>   // 技能发现追踪
  private loadedNestedMemoryPaths: Set<string> // 嵌套 memory 路径
  
  constructor(config: QueryEngineConfig)
  async *submitMessage(prompt, options): AsyncGenerator<SDKMessage>
}
```

**关键设计点**：

1. **会话级状态机**：`mutableMessages` 持久化整个对话历史
2. **流式生成器**：`async *submitMessage()` 返回 AsyncGenerator
3. **文件缓存**：`readFileState` 避免重复读取
4. **中断控制**：`abortController` 支持取消
5. **使用统计**：`totalUsage` 累积 token 消耗

---

## 三、上下文注入机制（context.ts）

### 3.1 两层上下文结构

Claude Code 将上下文分为两层：

```typescript
// System Context（系统层，缓存整个会话）
export const getSystemContext = memoize(async (): Promise<{[k: string]: string}> => {
  const gitStatus = await getGitStatus()  // git 状态
  return {
    ...(gitStatus && { gitStatus }),
  }
})

// User Context（用户层，缓存整个会话）
export const getUserContext = memoize(async (): Promise<{[k: string]: string}> => {
  const claudeMd = getClaudeMds(await getMemoryFiles())  // CLAUDE.md + memory
  return {
    ...(claudeMd && { claudeMd }),
    currentDate: `Today's date is ${getLocalISODate()}.`,
  }
})
```

### 3.2 Git 状态注入（getGitStatus）

```typescript
const [branch, mainBranch, status, log, userName] = await Promise.all([
  getBranch(),                    // 当前分支
  getDefaultBranch(),             // 主分支
  execFile('git', ['status', '--short']),  // 状态
  execFile('git', ['log', '--oneline', '-n', '5']),  // 最近5次提交
  execFile('git', ['config', 'user.name']),  // 用户名
])

// 截断过长的 status（最多 2000 字符）
const truncatedStatus = status.length > MAX_STATUS_CHARS
  ? status.substring(0, MAX_STATUS_CHARS) + '\n... (truncated)'
  : status

return [
  `This is the git status at the start of the conversation.`,
  `Current branch: ${branch}`,
  `Main branch (you will usually use this for PRs): ${mainBranch}`,
  `Git user: ${userName}`,
  `Status:\n${truncatedStatus || '(clean)'}`,
  `Recent commits:\n${log}`,
].join('\n\n')
```

**关键点**：
- 并行执行 5 个 git 命令（Promise.all）
- 截断过长输出（避免 token 爆炸）
- 明确说明"快照时间"（不会更新）
- 使用 memoize 缓存（整个会话只执行一次）

### 3.3 CLAUDE.md 和 Memory 注入

```typescript
const claudeMd = getClaudeMds(filterInjectedMemoryFiles(await getMemoryFiles()))
```

**加载逻辑**：
1. 扫描 `.claude/` 目录下的 memory 文件
2. 过滤已注入的文件（避免重复）
3. 读取 `CLAUDE.md`（项目规则）
4. 合并为一个字符串注入

---

## 四、流式输出实现对比

### 4.1 miniAgent 当前实现（agent.go:215-404）

```go
func (a *Agent) RunStream(ctx context.Context, messageCtx *agentctx.Context, 
                          input string, onToken TokenCallback) (string, error) {
    // 1. 注入 SystemPrompt
    // 2. 添加用户消息
    // 3. ReAct 循环
    for turn := 0; turn < a.config.MaxTurns; turn++ {
        // 调用 LLM 流式生成
        reader, err := a.model.Stream(ctx, messages)
        
        // 收集完整响应
        var fullContent string
        toolCallsMap := make(map[string]*schema.ToolCall)
        
        // 读取流式响应
        for {
            chunk, err := reader.Recv()
            if err == io.EOF { break }
            
            // 累积内容
            fullContent += chunk.Content
            if onToken != nil {
                onToken(chunk.Content)  // 回调输出
            }
            
            // 合并 ToolCalls（按索引）
            for i, tc := range chunk.ToolCalls {
                key := fmt.Sprintf("_index_%d", i)
                if existing, ok := toolCallsMap[key]; ok {
                    // 合并字段
                    existing.Function.Arguments += tc.Function.Arguments
                } else {
                    toolCallsMap[key] = &tc
                }
            }
        }
        
        // 构造最终消息
        finalMessage := &schema.Message{
            Role:      schema.Assistant,
            Content:   fullContent,
            ToolCalls: extractValidToolCalls(toolCallsMap),
        }
        
        // 检查工具调用
        if len(finalMessage.ToolCalls) > 0 {
            a.ctxManager.AddMessage(messageCtx, finalMessage)
            a.exeTools(ctx, messageCtx, finalMessage.ToolCalls)
            continue
        }
        
        return fullContent, nil
    }
}
```

**问题**：
- ✅ 流式输出已实现
- ✅ ToolCalls 合并逻辑正确
- ❌ 缺少进度追踪（无法知道当前轮数）
- ❌ 缺少 usage 统计
- ❌ 缺少中断控制

### 4.2 Claude Code 的流式实现（QueryEngine.ts:675-800）

```typescript
for await (const message of query({
  messages,
  systemPrompt,
  userContext,
  systemContext,
  canUseTool: wrappedCanUseTool,
  toolUseContext: processUserInputContext,
  fallbackModel,
  querySource: 'sdk',
  maxTurns,
  taskBudget,
})) {
  // 记录消息到历史
  if (message.type === 'assistant' || message.type === 'user') {
    messages.push(message)
    if (persistSession) {
      await recordTranscript(messages)  // 持久化
    }
  }
  
  // 统计 token 使用
  if (message.event.type === 'message_start') {
    currentMessageUsage = EMPTY_USAGE
    currentMessageUsage = updateUsage(currentMessageUsage, message.event.message.usage)
  }
  if (message.event.type === 'message_delta') {
    currentMessageUsage = updateUsage(currentMessageUsage, message.event.delta.usage)
    lastStopReason = message.event.delta.stop_reason
  }
  
  // 累积总使用量
  this.totalUsage = accumulateUsage(this.totalUsage, currentMessageUsage)
  
  // 流式输出
  yield* normalizeMessage(message)
}
```

**关键特性**：
1. **会话持久化**：每条消息都写入 transcript
2. **Usage 统计**：实时累积 token 使用
3. **Stop Reason 追踪**：记录停止原因
4. **Generator 嵌套**：`yield*` 转发内部生成器

---

## 五、miniAgent Phase 2 具体实施建议

### 5.1 优先级 P1：上下文注入机制

**目标**：让 Agent 天然感知项目状态

**实现步骤**：

#### Step 1: 创建 context 包（internal/context/inject.go）

```go
package context

import (
    "fmt"
    "os/exec"
    "strings"
    "time"
)

// SystemContext 系统层上下文（git 状态）
type SystemContext struct {
    GitStatus string
}

// UserContext 用户层上下文（项目规则、memory）
type UserContext struct {
    ClaudeMD    string
    CurrentDate string
}

// GetSystemContext 获取系统上下文（缓存）
func GetSystemContext() (*SystemContext, error) {
    gitStatus, err := getGitStatus()
    if err != nil {
        return nil, err
    }
    return &SystemContext{GitStatus: gitStatus}, nil
}

// getGitStatus 获取 git 状态（并行执行）
func getGitStatus() (string, error) {
    // 检查是否是 git 仓库
    if !isGitRepo() {
        return "", nil
    }
    
    // 并行执行 git 命令
    type gitResult struct {
        name  string
        value string
        err   error
    }
    
    commands := []struct {
        name string
        args []string
    }{
        {"branch", []string{"branch", "--show-current"}},
        {"mainBranch", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}},
        {"status", []string{"status", "--short"}},
        {"log", []string{"log", "--oneline", "-n", "5"}},
        {"user", []string{"config", "user.name"}},
    }
    
    results := make(chan gitResult, len(commands))
    for _, cmd := range commands {
        go func(name string, args []string) {
            out, err := exec.Command("git", args...).Output()
            results <- gitResult{name, strings.TrimSpace(string(out)), err}
        }(cmd.name, cmd.args)
    }
    
    // 收集结果
    data := make(map[string]string)
    for i := 0; i < len(commands); i++ {
        r := <-results
        if r.err == nil {
            data[r.name] = r.value
        }
    }
    
    // 截断过长的 status
    status := data["status"]
    if len(status) > 2000 {
        status = status[:2000] + "\n... (truncated)"
    }
    
    // 格式化输出
    return fmt.Sprintf(`This is the git status at the start of the conversation.

Current branch: %s
Main branch: %s
Git user: %s

Status:
%s

Recent commits:
%s`, 
        data["branch"],
        extractMainBranch(data["mainBranch"]),
        data["user"],
        status,
        data["log"],
    ), nil
}

// GetUserContext 获取用户上下文（CLAUDE.md + memory）
func GetUserContext() (*UserContext, error) {
    claudeMD, err := readClaudeMD()
    if err != nil {
        return nil, err
    }
    
    return &UserContext{
        ClaudeMD:    claudeMD,
        CurrentDate: time.Now().Format("2006-01-02"),
    }, nil
}

// readClaudeMD 读取 CLAUDE.md 和 memory 文件
func readClaudeMD() (string, error) {
    // 1. 读取 CLAUDE.md
    // 2. 读取 .claude/memory/*.md
    // 3. 合并返回
    // TODO: 实现
    return "", nil
}
```

#### Step 2: 修改 Agent.Run/RunStream 注入上下文

```go
func (a *Agent) Run(ctx context.Context, messageCtx *agentctx.Context, input string) (string, error) {
    // 1. 首次对话时注入完整上下文
    messages, _ := a.ctxManager.GetMessages(messageCtx)
    if len(messages) == 0 {
        // 注入 System Context
        sysCtx, err := context.GetSystemContext()
        if err == nil && sysCtx.GitStatus != "" {
            systemMsg := &schema.Message{
                Role:    schema.System,
                Content: fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>", sysCtx.GitStatus),
            }
            a.ctxManager.AddMessage(messageCtx, systemMsg)
        }
        
        // 注入 User Context
        userCtx, err := context.GetUserContext()
        if err == nil {
            if userCtx.ClaudeMD != "" {
                claudeMDMsg := &schema.Message{
                    Role:    schema.System,
                    Content: fmt.Sprintf("<claudeMd>\n%s\n</claudeMd>", userCtx.ClaudeMD),
                }
                a.ctxManager.AddMessage(messageCtx, claudeMDMsg)
            }
            
            dateMsg := &schema.Message{
                Role:    schema.System,
                Content: fmt.Sprintf("<currentDate>\n%s\n</currentDate>", userCtx.CurrentDate),
            }
            a.ctxManager.AddMessage(messageCtx, dateMsg)
        }
        
        // 注入 SystemPrompt
        if a.config.SystemPrompt != "" {
            systemMsg := &schema.Message{
                Role:    schema.System,
                Content: a.config.SystemPrompt,
            }
            a.ctxManager.AddMessage(messageCtx, systemMsg)
        }
    }
    
    // 2. 添加用户消息
    // 3. ReAct 循环
    // ... 其余逻辑不变
}
```

**收益**：
- Agent 自动感知 git 状态（分支、提交、变更）
- Agent 读取项目规则（CLAUDE.md）
- Agent 知道当前日期
- 无需修改 LLM 调用逻辑

---

### 5.2 优先级 P2：Usage 统计和进度追踪

**目标**：追踪 token 使用和执行进度

#### Step 1: 扩展 Agent.State

```go
type State struct {
    CurrentTurn int
    IsRunning   bool
    
    // 新增字段
    TotalTokens      int     // 总 token 数
    InputTokens      int     // 输入 token
    OutputTokens     int     // 输出 token
    CachedTokens     int     // 缓存 token
    TotalCostUSD     float64 // 总成本
    LastStopReason   string  // 停止原因
}
```

#### Step 2: 在 RunStream 中统计

```go
func (a *Agent) RunStream(ctx context.Context, messageCtx *agentctx.Context, 
                          input string, onToken TokenCallback) (string, error) {
    // ... 前面逻辑不变
    
    for turn := 0; turn < a.config.MaxTurns; turn++ {
        a.state.CurrentTurn = turn + 1
        
        reader, err := a.model.Stream(ctx, messages)
        
        for {
            chunk, err := reader.Recv()
            if err == io.EOF { break }
            
            // 统计 token（如果 chunk 包含 usage 信息）
            if chunk.Usage != nil {
                a.state.InputTokens += chunk.Usage.InputTokens
                a.state.OutputTokens += chunk.Usage.OutputTokens
                a.state.CachedTokens += chunk.Usage.CachedTokens
                a.state.TotalTokens = a.state.InputTokens + a.state.OutputTokens
            }
            
            // 记录停止原因
            if chunk.StopReason != "" {
                a.state.LastStopReason = chunk.StopReason
            }
            
            // 流式输出
            if chunk.Content != "" {
                fullContent += chunk.Content
                if onToken != nil {
                    onToken(chunk.Content)
                }
            }
        }
    }
}
```

#### Step 3: 提供查询接口

```go
// GetUsage 获取 token 使用统计
func (a *Agent) GetUsage() *UsageStats {
    return &UsageStats{
        TotalTokens:  a.state.TotalTokens,
        InputTokens:  a.state.InputTokens,
        OutputTokens: a.state.OutputTokens,
        CachedTokens: a.state.CachedTokens,
        CostUSD:      a.state.TotalCostUSD,
    }
}
```

---

### 5.3 优先级 P3：文件读取缓存

**目标**：避免重复读取同一文件

#### 实现：在 Agent 中添加文件缓存

```go
type Agent struct {
    // ... 现有字段
    
    // 文件缓存
    fileCache map[string]fileCacheEntry
    cacheMu   sync.RWMutex
}

type fileCacheEntry struct {
    Content   string
    ModTime   time.Time
    ReadCount int
}

// 在 read_file 工具中使用缓存
func (t *ReadFileTool) InvokableRun(ctx context.Context, args string) (string, error) {
    // 1. 检查缓存
    if cached, ok := t.agent.getFileCache(path); ok {
        // 检查文件是否修改
        stat, _ := os.Stat(path)
        if stat.ModTime().Equal(cached.ModTime) {
            logger.Debug("Using cached file: %s", path)
            return cached.Content, nil
        }
    }
    
    // 2. 读取文件
    content, err := os.ReadFile(path)
    if err != nil {
        return "", err
    }
    
    // 3. 更新缓存
    stat, _ := os.Stat(path)
    t.agent.setFileCache(path, fileCacheEntry{
        Content: string(content),
        ModTime: stat.ModTime(),
        ReadCount: 1,
    })
    
    return string(content), nil
}
```

---

### 5.4 优先级 P4：中断控制

**目标**：支持取消长时任务

#### 实现：使用 context.Context

```go
func (a *Agent) RunStream(ctx context.Context, messageCtx *agentctx.Context, 
                          input string, onToken TokenCallback) (string, error) {
    for turn := 0; turn < a.config.MaxTurns; turn++ {
        // 检查是否取消
        select {
        case <-ctx.Done():
            return "", fmt.Errorf("agent cancelled: %w", ctx.Err())
        default:
        }
        
        // 调用 LLM（传递 ctx）
        reader, err := a.model.Stream(ctx, messages)
        
        // ... 其余逻辑
    }
}
```

**使用示例**：

```go
// 创建可取消的 context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

// 运行 Agent
result, err := agent.RunStream(ctx, messageCtx, input, onToken)
if err == context.DeadlineExceeded {
    fmt.Println("Agent timeout")
}
```

---

## 六、实施优先级总结

| 优先级 | 任务 | 工作量 | 收益 |
|--------|------|--------|------|
| P1 | 上下文注入（git + CLAUDE.md） | 2-3h | ⭐⭐⭐⭐⭐ |
| P2 | Usage 统计 | 1h | ⭐⭐⭐⭐ |
| P3 | 文件缓存 | 1h | ⭐⭐⭐ |
| P4 | 中断控制 | 0.5h | ⭐⭐⭐ |

**建议顺序**：P1 → P2 → P3 → P4

**Phase 2 完成标准**：
- ✅ Agent 自动注入 git 状态
- ✅ Agent 读取 CLAUDE.md
- ✅ 实时统计 token 使用
- ✅ 文件读取缓存生效
- ✅ 支持中断控制

---

## 七、关键代码参考

### 7.1 必读文件

| 文件 | 行数 | 关键内容 |
|------|------|----------|
| src/context.ts | 190 | git 状态、CLAUDE.md 注入 |
| src/QueryEngine.ts | 1295 | 会话状态机、流式输出 |
| src/utils/fileStateCache.ts | ~200 | 文件缓存实现 |

### 7.2 可以跳过的部分

- Feature flag 体系（太复杂）
- Coordinator 模式（Phase 3 再看）
- MCP/Plugin 系统（Phase 3 再看）
- 权限系统（Phase 3 再看）

---

## 八、避免的陷阱

1. **不要过度设计**：先实现基础功能，不要一次性搬运所有特性
2. **注意 token 控制**：git status 要截断，CLAUDE.md 要限制大小
3. **缓存失效策略**：文件缓存要检查 ModTime
4. **并发安全**：文件缓存要加锁
5. **错误处理**：git 命令失败不应阻塞 Agent 启动
