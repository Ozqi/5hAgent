# Bubble Tea Agent TUI 开发参考

本文记录 walle 后续 TUI 开发可复用的 Bubble Tea 生态知识，用作当前 TUI 演进参考。

## 当前入口

- 主界面：[internal/tui/app.go](/Users/bytedance/Proj/walle/internal/tui/app.go)
- Markdown 渲染：[internal/tui/markdown.go](/Users/bytedance/Proj/walle/internal/tui/markdown.go)
- 模块说明：[doc/interface/cli.md](/Users/bytedance/Proj/walle/doc/interface/cli.md)

当前实现已经使用：

- Bubble Tea：`tea.Model` 的 `Init / Update / View` 主循环。
- Lip Gloss：颜色、边框、面板、输入框、状态栏样式。
- Bubbles `textarea`：用户输入框。
- Bubbles `viewport`：对话历史滚动区域。

## 推荐生态组合

| 包 | 用途 | 在 walle 中的建议 |
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
    submit   func(string) error
}

func (m Model) Init() tea.Cmd {
    return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // 处理 key、窗口尺寸、daemon event、tool event、错误和完成事件。
    return m, nil
}

func (m Model) View() string {
    // 用 Lip Gloss 拼出 conversation、input 上下状态区和 input。
    return ""
}
```

walle 的当前 `AppModel` 已经比这个骨架更完整：它持有 daemon submit/stop 回调、session id、tool event、picker 状态和流式 token 消息。它不持有 `Agent`、`SkillManager` 或 `Context`；后续开发应优先在这个模型内增量演进，不另起一套 TUI 框架。

## 设计原则

1. `Update` 负责状态，不负责布局。布局尽量放在 `View` 和独立 render 函数中。
2. LLM、工具和文件操作留在 daemon/runtime 侧；TUI 只把 daemon 事件异步转成 `tea.Msg`。
3. UI 不直接读写 agent 内部状态；需要展示的数据先汇总成 snapshot。
4. 命令提示、模型选择、工具列表这类交互优先复用 Bubbles 组件，而不是手写完整选择器。
5. 任何可能影响终端宽度的内容都要用 `lipgloss.Width` 或现有宽字符处理函数验证。
6. 视觉改动要同时看 ANSI 文本和真实截图；有些颜色在 capture 文本里看不出来。

## Slash 命令提示

当前 slash 命令提示是在输入框附近渲染浅色提示，相关函数位于 `internal/tui/app.go`：

- `slashCommandHints`
- `slashHintMatches`
- `renderSlashHint`

已覆盖的提示命令：

- `/skill <list|get|reload>`
- `/compress`
- `/mcp <list|add|remove|enable|disable>`
- `/session <new|list|id>`
- `/model <provider/model>`
- `/provider [name]`
- `/stop`

这些提示只负责补全和说明，执行仍由 daemon session 负责；`/detach` 是 TUI 本地退出命令。

后续如果要做更接近 OpenCode 的体验，可以在保持最小改动的前提下升级为：

1. 用户输入 `/` 时显示全部命令。
2. 用户输入 `/m` 时只显示 `/mcp`。
3. Enter 仍发送文本；上下键和 Tab 再考虑是否接入选择器。
4. 若需要完整弹出列表，再引入 Bubbles `list`，不要手写复杂焦点系统。

## Markdown 渲染

当前项目有自定义流式 Markdown 渲染：[internal/tui/markdown.go](/Users/bytedance/Proj/walle/internal/tui/markdown.go)。

适合继续保留自定义渲染的场景：

- 只需要标题、列表、代码块、表格等有限格式。
- 需要对流式 token 做稳定增量显示。
- 希望渲染输出更可控。

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

1. 纯文本结构：使用 `tmux capture-pane` 查看实际内容是否存在。
2. ANSI 宽度：用 `lipgloss.Width` 和真实 capture 验证中文、颜色和表格没有超宽。
3. 真实视觉：用终端截图确认浅色提示、边框、光标、输入框背景和状态栏对比度。

可用方法：

- 用户直接发截图，适合判断颜色、层级、遮挡、对齐。
- 本地运行 `walle` 后用 `tmux capture-pane` 捕获文本，适合判断渲染内容是否出现。
- 鼠标滚轮可用 SGR mouse escape 验证：`tmux send-keys -t <session> Escape '[<64;10;10M'` 上滚，`'[<65;10;10M'` 下滚；不要发送字面量 `WheelUpPane`。
- 在 macOS 上用 `screencapture` 截图，但可能受屏幕录制权限影响。
- 固定视图逻辑通过人工 TUI 操作和 capture 验证。

调试时不要只依赖截图：截图能发现视觉问题，命令状态、焦点状态和消息顺序还要回到 `Update` 事件流确认。

## 适合 walle 的增量路线

短期优先级：

1. 继续打磨 slash 命令提示和匹配体验。
2. 保持左侧 sidebar 和右侧 status panel 删除后的单列结构。
3. 输入框保持参考 tmux 对话窗口的深灰低对比样式，对颜色和布局做截图验证。
4. 模型切换由 daemon/runtime 负责；TUI 只展示 `/model` picker 事件并回传选择。

中期可考虑：

1. 用 Bubbles `list` 做 `/models` 或 slash command picker。
2. 用 Bubbles `spinner` 替换手写 spinner frame。
3. 用 Glamour 统一 Markdown 主题。
4. 把输入框附近状态区的数据源保持为 snapshot，避免直接读 runtime 内部对象。

## 验证清单

改 TUI 代码后至少检查：

- 通过 `tmux capture-pane` 检查纯文本结构。
- 中文、英文长词、ANSI 彩色字符串不会撑爆固定宽度。
- slash 命令、工具事件、assistant token、thinking token 的 entry 顺序正确。
- PgUp/PgDown 与 SGR mouse wheel 上下滚动历史记录时，`scroll xx%` 状态应随之变化。
- 72 列以下进入紧凑状态行和紧凑 slash hint；50x18 应仍能看到提示、输入条、session 和 state。
- 长中文输入停留在输入框内时至少显示两行，不能把 session/footer 挤出屏幕。
- Markdown 标题、列表、代码块和表格在 60 列下不能撑破输入区或底栏；空输入框不能重复显示两行 prompt。
- 空态内容紧贴输入区状态行上方，避免在提示和输入条之间留下无意义空行；输入 `/` 后输入条不应产生明显跳动。
- diff 代码块的 `+` / `-` 行应使用参考窗口的绿/红低对比背景，普通代码行用主文本色，fence 弱化。
- 工具错误态应显示为 `◆ Failed <tool>`，错误内容逐行 `└` 缩进，不能退回旧的 `TOOL_EXEC` 盒子。
- 工具 running 态通过真实 daemon 工具调用验证，避免在 TUI 客户端保留本地 debug 执行路径。
- 未知 slash command 应显示为 `◆ Command /unknown` 错误，不进入 LLM，不增加 context messages。
- slash hint 区域保持固定高度，输入 `/` 前后输入条、session/footer、底栏不能跳动。
- 如果改启动 wiring，再运行 `go build -o walle ./cmd/walle`。
- 视觉改动尽量补一张真实终端截图或 tmux capture 记录。

## tmux 验证矩阵

本轮 TUI 重构用 tmux 实测过这些场景。后续改 TUI 时优先复用同一类场景，不要只看源码：

| 场景 | 尺寸 | 输入/触发 | 观察点 |
| --- | --- | --- | --- |
| 空态 | 100x28 / 80x24 | 启动 TUI | 输入条、session、state、空提示 |
| slash hint | 100x28 / 60x20 / 50x18 | `/` | hint 不撑破输入区，窄屏显示短命令 |
| unknown slash | 80x24 | `/unknown` + Enter | 显示 command 错误，state 保持 idle |
| 长输入 | 80x24 / 50x18 | 长中文不回车 | 输入框至少两行，footer 不被挤掉 |
| 真实 tool call | 100x30 | 让模型调用 `base.read_file` | `◆ Ran`、结果缩进、最终回答 |
| tool running | 100x30 | 让 daemon 触发一次真实工具调用 | running spinner、done 后回 idle |
| tool error | 100x30 | 真实错误工具调用 | `◆ Failed`、错误逐行 `└` |
| thinking | 100x30 | 含 `reasoning_content` 的 session | `◆ thinking`、正文弱化 |
| 长历史 | 100x30 | 多轮历史 session | PgUp/PgDown、`scroll xx%` |
| 鼠标滚轮 | 100x30 | SGR wheel escape | wheel up/down 后 `scroll xx%` 改变 |
| Markdown | 100x32 / 60x24 | 标题、列表、代码块、表格 | 不撑破输入区，diff 行有红/绿背景 |
| slash 长输出 | 100x30 | `/session list`、`/skill list` | `◆ Command` 标题，长输出省略 |

`/session list` 捕获必须先于矩阵脚本创建临时 sample session，避免 `tui-capture-*` 污染列表；`audit.txt` 会检查这一点。

常用命令片段：

```bash
/Users/bytedance/.trae/skills/tui-development/scripts/tui_capture_matrix.sh
# 生成 .traces/tui/<timestamp>/，并写 audit.txt 与 ansi-backgrounds.txt。
# 默认自动检查 replacement char、重复空 prompt、明显超长裸行、旧亮色输入背景和黑色背景块。

tmux new-session -d -s walle-tui-check -x 100 -y 28 -c /Users/bytedance/Proj/5hWorkSpace '/Users/bytedance/Proj/walle/walle'
tmux capture-pane -t walle-tui-check -p -S -80
tmux send-keys -t walle-tui-check '/'
tmux send-keys -t walle-tui-check Escape '[<64;10;10M'
tmux send-keys -t walle-tui-check Escape '[<65;10;10M'
tmux kill-session -t walle-tui-check 2>/dev/null || true
```

## 参考代码路径

继续开发时优先从这些函数读起：

- `NewAppModel`：初始化 viewport、textarea 和默认状态。
- `Update`：处理窗口尺寸、按键、stream token、tool event、spinner。
- `View`：拼接 conversation、输入框上下状态区和底部状态条。
- `renderConversationEntry`：渲染用户、assistant、thinking、tool hint。
- `renderSlashHint`：slash 命令浅色提示。
- `submit`：把用户输入转成 daemon control 请求。
