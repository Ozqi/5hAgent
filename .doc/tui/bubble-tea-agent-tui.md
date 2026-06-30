# Bubble Tea Agent TUI 开发参考

本文记录 5hAgent 后续 TUI 开发可复用的 Bubble Tea 生态知识。它不是新模板生成任务，而是给当前 `internal/cli/tui.go` 继续演进时查阅的参考。

## 当前入口

- 主界面：[internal/cli/tui.go](/Users/bytedance/Proj/5hAgent/internal/cli/tui.go)
- Markdown 渲染：[internal/cli/markdown_stream.go](/Users/bytedance/Proj/5hAgent/internal/cli/markdown_stream.go)
- TUI 测试：[internal/cli/tui_test.go](/Users/bytedance/Proj/5hAgent/internal/cli/tui_test.go)
- 模块说明：[doc/cli.md](/Users/bytedance/Proj/5hAgent/doc/cli.md)

当前实现已经使用：

- Bubble Tea：`tea.Model` 的 `Init / Update / View` 主循环。
- Lip Gloss：颜色、边框、面板、输入框、状态栏样式。
- Bubbles `textarea`：用户输入框。
- Bubbles `viewport`：对话历史滚动区域。

## 推荐生态组合

| 包 | 用途 | 在 5hAgent 中的建议 |
| --- | --- | --- |
| Bubble Tea | TUI 主框架，Elm 架构 | 保持 `AppModel.Update` 只处理消息和状态迁移，避免直接做阻塞 I/O |
| Lip Gloss | 样式和布局 | 继续集中维护颜色和 style 变量，减少散落的字符串拼接样式 |
| Bubbles | 现成组件 | 输入、viewport 已采用；后续模型选择、命令选择可考虑 `list` |
| Glamour | Markdown 渲染 | 如果当前 `markdown_stream.go` 不够用，可评估替换或局部引入 |

官方示例优先级：

- Bubble Tea examples: <https://github.com/charmbracelet/bubbletea/tree/main/examples>
- Chat example: <https://github.com/charmbracelet/bubbletea/tree/main/examples/chat>
- 常用参考目录：`chat`、`list`、`textinput`、`textarea`、`viewport`、`spinner`

## Agent TUI 基本形状

AI Agent 的 TUI 通常不需要先做复杂框架，最小稳定结构是：

```go
type Model struct {
    messages []string
    input    textarea.Model
    viewport viewport.Model
    agent    *Agent
}

func (m Model) Init() tea.Cmd {
    return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // 处理 key、窗口尺寸、agent token、tool event、错误和完成事件。
    return m, nil
}

func (m Model) View() string {
    // 用 Lip Gloss 拼出 conversation、input 上下状态区和 input。
    return ""
}
```

5hAgent 的当前 `AppModel` 已经比这个骨架更完整：它持有 `Agent`、`TaskList`、`SkillManager`、`Context`、session id、tool event 和流式 token 消息。后续开发应优先在这个模型内增量演进，不另起一套 TUI 框架。

## 设计原则

1. `Update` 负责状态，不负责布局。布局尽量放在 `View` 和独立 render 函数中。
2. LLM、工具和文件操作保持异步消息化，使用 `tea.Cmd` 或外部 goroutine 发送 `tea.Msg`。
3. UI 不直接读写 agent 内部状态；需要展示的数据先汇总成 snapshot。
4. 命令提示、模型选择、工具列表这类交互优先复用 Bubbles 组件，而不是手写完整选择器。
5. 任何可能影响终端宽度的内容都要用 `lipgloss.Width` 或现有宽字符处理函数验证。
6. 视觉改动要同时看 ANSI 文本和真实截图；有些颜色在 capture 文本里看不出来。

## Slash 命令提示

当前 slash 命令提示是在输入框附近渲染浅色提示，相关函数位于 `internal/cli/tui.go`：

- `slashCommandHints`
- `slashHintMatches`
- `renderSlashHint`

已覆盖的命令：

- `/task <list|create|update|get|delete|archive|reopen>`
- `/skill <list|enable|disable|show>`
- `/compress [compact|truncate]`
- `/mcp <list|add|remove|enable|disable>`
- `/session <new|list|switch|save|drop>`

后续如果要做更接近 OpenCode 的体验，可以在保持最小改动的前提下升级为：

1. 用户输入 `/` 时显示全部命令。
2. 用户输入 `/m` 时只显示 `/mcp`。
3. Enter 仍发送文本；上下键和 Tab 再考虑是否接入选择器。
4. 若需要完整弹出列表，再引入 Bubbles `list`，不要手写复杂焦点系统。

## Markdown 渲染

当前项目有自定义流式 Markdown 渲染：[internal/cli/markdown_stream.go](/Users/bytedance/Proj/5hAgent/internal/cli/markdown_stream.go)。

适合继续保留自定义渲染的场景：

- 只需要标题、列表、代码块、表格等有限格式。
- 需要对流式 token 做稳定增量显示。
- 希望测试输出更可控。

适合评估 Glamour 的场景：

- 需要更完整的 Markdown 兼容性。
- 希望 AI 回复的代码块、引用、表格有统一主题。
- 当前自定义 renderer 开始承载过多格式分支。

引入 Glamour 前要验证：

- 流式输出时是否频繁重排导致闪烁。
- 终端宽度变化后的 wrap 是否正确。
- ANSI 样式是否和现有 Lip Gloss 主题冲突。

## 截图和识图 Debug

TUI 问题建议分三层排查：

1. 纯文本结构：使用 `tmux capture-pane` 或测试里的 `stripANSI` 查看实际内容是否存在。
2. ANSI 宽度：用 `lipgloss.Width`、现有 wrap 测试验证中文、颜色和表格没有超宽。
3. 真实视觉：用终端截图确认浅色提示、边框、光标、输入框背景和状态栏对比度。

可用方法：

- 用户直接发截图，适合判断颜色、层级、遮挡、对齐。
- 本地运行 `5hagent` 后用 `tmux capture-pane` 捕获文本，适合判断渲染内容是否出现。
- 在 macOS 上用 `screencapture` 截图，但可能受屏幕录制权限影响。
- 对固定视图逻辑写单元测试，例如 `renderSlashHint`、`renderConversationEntry`。

调试时不要只依赖截图：截图能发现视觉问题，但命令状态、焦点状态和消息顺序仍要回到 `Update` 事件流和测试里确认。

## 适合 5hAgent 的增量路线

短期优先级：

1. 继续补足 slash 命令提示和匹配测试。
2. 删除左侧 sidebar 和右侧 status panel，把有用状态迁移到输入框上方和下方。
3. 输入框改为偏亮灰色背景，对颜色和布局做截图验证。
4. 模型切换先使用 CLI `--model/-m` 和配置；TUI 内模型选择等后续再接。

中期可考虑：

1. 用 Bubbles `list` 做 `/models` 或 slash command picker。
2. 用 Bubbles `spinner` 替换手写 spinner frame。
3. 用 Glamour 统一 Markdown 主题。
4. 把输入框附近状态区的数据源保持为 snapshot，避免直接读 runtime 内部对象。

## 验证清单

改 TUI 代码后至少检查：

- `go test ./internal/cli`
- 相关 render 函数有不含 ANSI 的断言。
- 中文、英文长词、ANSI 彩色字符串不会撑爆固定宽度。
- slash 命令、工具事件、assistant token、thinking token 的 entry 顺序正确。
- 如果改启动 wiring，再运行 `go build -o 5hagent cmd/5hagent/main.go`。
- 视觉改动尽量补一张真实终端截图或 tmux capture 记录。

## 参考代码路径

继续开发时优先从这些函数读起：

- `NewAppModel`：初始化 viewport、textarea 和默认状态。
- `Update`：处理窗口尺寸、按键、stream token、tool event、spinner。
- `View`：拼接 conversation、输入框上下状态区和底部状态条。
- `renderConversationEntry`：渲染用户、assistant、thinking、tool hint。
- `renderSlashHint`：slash 命令浅色提示。
- `submit` / `runAgent`：把用户输入转成 agent 执行。
