# CLI - 命令行界面工具

## 概述

CLI 模块提供两种输出模式：
1. **简单输出模式** (ui.go) - 基础的 stdout 输出函数（legacy，保留但未使用）
2. **TUI 模式** (tui.go) - 基于 Bubble Tea 的交互式终端界面（当前主模式）

---

## TUI 模块 (`internal/cli/tui.go`)

### 依赖

- `github.com/charmbracelet/bubbletea` - TUI 框架
- `github.com/charmbracelet/bubbles` - 组件库（textarea、viewport）
- `github.com/charmbracelet/lipgloss` - 终端样式

### 核心类型

```go
// 对话条目
type conversationEntry struct {
    Role    string  // "user" | "assistant" | "tool" | "system"
    Content string
}

// 状态快照（用于右侧状态面板）
type statusSnapshot struct {
    Busy                bool
    CurrentState        string      // "idle" | "thinking" | "streaming" | "error"
    TokenUsed           int
    TokenLimit          int
    ContextMessages     int
    ContextSummaries    int
    ToolCallsTotal      int
    LastToolName        string
    EnabledSkills       []string
    TaskTotal           int
    TaskInProgress      int
    HighlightedTaskLine []string    // 当前/最近的 3 个任务
}

// 主应用模型
type AppModel struct {
    program    *tea.Program
    ag         *agent.Agent
    modelName  string
    agentName  string
    taskList   *task.TaskList
    skillMgr   *skill.Manager
    ctxManager *agentctx.Manager
    messageCtx *agentctx.Context
    ctx        context.Context

    width       int
    height      int
    statusWidth int
    busy        bool

    viewport  viewport.Model   // 对话区域
    input     textarea.Model   // 底部输入框
    entries   []conversationEntry
    toolCalls int
    lastTool  string

    currentAssistant int      // 当前正在流式输出的 assistant 条目索引
    currentStatus    string
    spinnerFrame     int      // 转盘动画帧
    escPending       bool
    lastEscAt        time.Time
}
```

### 布局结构

```
┌─────────────────────────────────────┬──────────┐
│                                     │  status  │
│         对话区域 (viewport)          │  busy    │
│                                     │  tokens  │
│   You: 用户输入                      │  context│
│   Agent: AI 响应 (Markdown渲染)      │  tools   │
│   tool: 工具调用                     │  skills  │
│                                     │  tasks   │
├─────────────────────────────────────┴──────────┤
│ > 输入消息...                                  │
│ model: claude-3-5 | agent: Agent              │
└────────────────────────────────────────────────┘
```

- 默认 100x30 终端尺寸
- 宽度 ≥120 时状态面板 34 字符，<80 时 24 字符
- 对话区域宽度 = 总宽度 - 状态面板 - 4

### 内部消息类型

```go
type assistantTokenMsg struct { token string }   // AI 流式输出 token
type assistantDoneMsg struct{}                   // AI 输出完成
type assistantErrorMsg struct { err error }      // AI 执行错误
type toolEventMsg struct { event logger.ToolEvent }  // 工具调用事件
type spinnerTickMsg struct{}                     // 转盘动画 tick
```

### 主要函数

| 函数 | 说明 |
|------|------|
| `NewAppModel(...)` | 创建 AppModel 实例 |
| `LaunchTUI(...)` | 启动 TUI 程序入口 |
| `Init()` | 初始化光标闪烁 |
| `Update(msg)` | 处理消息更新状态 |
| `View()` | 渲染整个 TUI 界面 |
| `submit()` | 提交用户输入 |
| `runAgent()` | 在 goroutine 中运行 agent |
| `resize()` | 计算布局尺寸 |
| `refreshView()` | 刷新对话区域内容 |
| `snapshot()` | 生成状态快照 |

### 内置命令

| 命令 | 说明 |
|------|------|
| `/skill <args>` | 调用 skill 管理命令 |
| `/task <args>` | 调用 task 管理命令 |
| `/compress` | 压缩上下文并打印压缩后的当前 ctx |

命令处理在 `submit()` 中完成，不走 agent。

### 快捷键

| 键 | 功能 |
|----|------|
| `Enter` | 发送消息 |
| `Ctrl+C` / 双 `Esc` | 退出程序 |
| `PgUp` / `Ctrl+B` | 上滚动对话 |
| `PgDown` / `Ctrl+F` | 下滚动对话 |

### 样式约定

- **User**: 绿色 `You:`
- **Assistant**: 青色 `Agent:` + Markdown 渲染
- **Tool**: 灰色 `tool:` + 参数缩进
- **System**: 灰色 `system:`
- Markdown 支持标题、代码块、列表等，颜色编码输出

---

## 简单输出模式 (`internal/cli/ui.go`)

> ⚠️ **Legacy**: 以下函数已注释未使用，保留 API 兼容

### PrintError(err)

打印红色错误消息到 stdout。

```go
cli.PrintError(fmt.Errorf("file not found"))
// 输出: Error: file not found
```

---

## Markdown 流式渲染 (`internal/cli/markdown_stream.go`)

### splitReadyMarkdown(input string, force bool)

分割已就绪的 Markdown 文本。在代码块外部，遇到空行时分割，支持流式输出。

```go
ready, rest := splitReadyMarkdown("Hello\n\nworld\n", false)
// ready = "Hello", rest = "world"
```

### renderMarkdownForTerminal(input string, color bool)

将 Markdown 渲染为终端 ANSI 颜色字符串。支持：
- 标题 (`#` → 粗体)
- 代码块 (``` → 高亮背景)
- 行内代码 (`` ` `` → 红色)
- 粗体/斜体
- 链接
- 列表

---

## 测试覆盖 (`internal/cli/tui_test.go`)

| 测试 | 验证内容 |
|------|----------|
| `TestRenderStatusPanelIncludesKeySections` | 状态面板包含所有关键部分 |
| `TestRenderConversationEntryUsesMarkdownRenderer` | 对话条目使用 Markdown 渲染 |
| `TestRenderToolEntryIsDimAndCompact` | 工具条目灰色紧凑 |
| `TestAnimatedStateLabel` | 动画状态标签含 spinner |
| `TestInitOnlyStartsCursorBlink` | Init 仅启动光标闪烁 |
| `TestDoubleEscQuits` | 双 Esc 退出 |
| `TestEnterSubmitsMessage` | Enter 提交消息 |
| `TestAssistantEntryCreatedOnFirstToken` | 首个 token 创建 assistant 条目 |
| `TestViewDoesNotExceedWindowHeight` | 渲染不超过窗口高度 |
| `TestNormalizeAssistantContentCollapsesBlankLines` | 折叠空行 |

---

## 使用示例

```go
// 创建并启动 TUI
ctx := context.Background()
model := NewAppModel(ctx, ag, "claude-3-5-sonnet", taskList, skillMgr, ctxManager, messageCtx)
if err := LaunchTUI(ctx, ag, "claude-3-5-sonnet", taskList, skillMgr, ctxManager, messageCtx); err != nil {
    log.Fatal(err)
}
```

---

## 设计特点

1. **流式输出**: AI token 逐个接收并追加到当前 assistant 条目
2. **Goroutine 并发**: `runAgent()` 在后台 goroutine 执行，不阻塞 UI
3. **双缓冲**: viewport 累积 entries，refreshView 时统一设置内容
4. **自适应布局**: resize() 根据窗口宽度调整状态面板宽度
5. **工具事件桥接**: logger.SetToolEventSink 将日志事件转发到 TUI 消息
