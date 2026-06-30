# 5hAgent UI 组件复用笔记

本文不是通用前端教程，而是给当前 `internal/cli/tui.go` 演进时做边界判断用的工作笔记。重点是：在 Bubble Tea + Lip Gloss 这套终端 UI 里，哪些内容值得复用，应该以什么粒度复用，避免为了“像前端组件化”而把 TUI 代码拆得更难维护。

## 1. 当前 UI 结构

当前主文件：[internal/cli/tui.go](/Users/bytedance/Proj/5hAgent/internal/cli/tui.go)

当前代码里已经存在的天然分层：

- 状态模型：
  - `AppModel`
  - `conversationEntry`
  - `statusSnapshot`
- 布局层：
  - `View`
  - `renderSidebar`
  - `renderMainPane`
  - `renderStatusPanel`
  - `renderBottomStatusBar`
- 内容块层：
  - `renderSlashHint`
  - `renderThinkingEntry`
  - `renderMessageBlock`
  - `renderToolEntry`
  - `renderToolCompactEntry`
  - `renderToolUnknownEntry`

这说明当前代码已经不是“一坨逻辑”，而是处在“函数级组件化”阶段。下一轮布局会删除左侧 sidebar 和右侧状态栏，但这些函数里的状态数据仍然有价值，应先迁移到输入框上方/下方的状态区，再删除旧布局。

## 2. 适合复用的组件类型

在这个仓库里，优先复用下面三类，而不是盲目追求 React 式组件树。

### 2.1 样式 token

典型内容：

- 颜色
- 边框样式
- padding / margin
- 固定宽度
- 高亮态 / 普通态

当前已有基础：

- `colorBg`
- `colorSurface`
- `colorGreen`
- `colorBlue`
- `colorGray`
- `inputShellStyle`
- `slashHintStyle`
- `messageBoxStyle`

建议：

- 样式 token 可以继续集中维护。
- 不急着拆新包，先保证“同类视觉元素用同一组 token”。
- 如果新增 token，优先命名为语义色或语义 style，不要命名成页面局部色，例如少用 `chatHeaderBlue`，多用 `colorAccentInfo` 一类的抽象语义名。

### 2.2 渲染块

这类是最值得复用的当前粒度。

例子：

- 带标题的消息块
- tool 调用块
- thinking 块
- 状态行 `label + value`
- slash hint 行

判断标准：

- 同一块在 2 个以上地方重复出现
- 宽度、边框、前缀、颜色规则一致
- 未来有单测价值

这类复用通常保持成 `renderXxx(...) string` 就够了，不一定要上 struct/component。

### 2.3 视图快照

TUI 和前端很像的一点是：渲染层不应该直接乱读 runtime 内部状态。

当前已有一个好起点：

- `statusSnapshot`

建议继续沿这个方向：

- 输入框附近的状态区优先消费 snapshot 或轻量 view model。
- 不要让 `View` 或 `render*` 深入读取 `taskList`、`ctxManager`、`agent` 的复杂逻辑。
- 如果某个面板需要很多派生字段，优先先造快照 struct，再考虑渲染。

## 3. 不要急着抽成“组件”的部分

### 3.1 只有一次使用、逻辑很薄的 render 函数

如果一个函数：

- 只在一个地方用
- 逻辑只有几行
- 抽出去不会减少认知负担

那就先留在原地。过早拆分会让 `tui.go` 变成很多来回跳的薄包装。

### 3.2 带强状态耦合的交互逻辑

例如：

- `Update`
- `submit`
- `runAgent`
- session 切换
- tool event 进入 entries 的逻辑

这些是控制流，不是展示组件。它们更适合通过“消息类型明确、状态转换清楚”来维护，而不是组件化。

### 3.3 即将废弃的布局结构

sidebar 和右侧 status panel 会被移除，不要再围绕它们新增抽象。需要保留的是其中的数据：模型、状态、token、消息数、工具调用、技能和任务焦点。

## 4. 适合本项目的组件化顺序

### 阶段 1：统一 token 和命名

目标：

- 颜色、边框、宽度、状态名更统一
- `render*` 家族命名保持同一尺度

输出物：

- 少量常量整理
- 少量 render 函数重命名
- 不动文件结构

### 阶段 2：沉淀可复用渲染块

目标：

- 把消息块、tool 块、状态行这类高复用区域收敛为稳定函数
- 给这些函数补最小测试

适合抽象的对象：

- message block
- entry header / prefix
- `label-value` 行
- hint/list 小面板

### 阶段 3：必要时再拆文件

只有在下面情况成立时再拆：

- `tui.go` 的单一文件体积已经明显影响修改效率
- 某一类 render 函数内部强相关，独立阅读更轻松
- 测试和实现一起移动后边界更清楚

可选拆分方向：

- `tui_render_layout.go`
- `tui_render_entry.go`
- `tui_render_status.go`

但这一步不是当前前提。

## 5. 下一轮布局目标

目标是让 TUI 回到单主列结构：

- 上方是对话和工具/thinking 流。
- 输入框上方显示当前 Agent 状态、模型、token、速度、最近工具和任务焦点。
- 输入框使用偏亮灰色背景，和深色对话区拉开层级。
- 输入框下方放低频状态，例如 session、enabled skills、快捷键和 slash hint。
- 不保留左侧 sidebar 和右侧 status panel。

实现时优先复用 `statusSnapshot` 和现有 entry 渲染函数，不要把运行状态直接散落到 `View` 里。

## 6. 当前可以复用的“组件目录”

如果 UI 分支开始积累稳定经验，`.doc/tui/` 可以持续存放：

- 布局规则
- 宽度/换行问题案例
- 颜色与层级规则
- 通用渲染块清单
- 真实截图或问题记录

建议文档主题：

1. `ui-component-reuse-notes.md`
2. `layout-and-width-rules.md`
3. `tool-entry-patterns.md`
4. `terminal-visual-regression-notes.md`

其中第 1 份是原则文档，后几份更适合沉淀具体现象和经验。

## 7. UI 设计时的判断清单

每次准备新增一个“组件”前，先问这几个问题：

1. 这是样式复用、渲染复用，还是状态复用？
2. 这个抽象是否减少重复，还是只是换了个名字包一层？
3. 它是否能形成稳定输入输出，适合写测试？
4. 它会不会让 `Update -> refreshView -> render*` 这条主链更难追？
5. 这个复用是否只服务当前一个页面？如果是，保留在 `tui.go` 往往更稳。

## 8. 对当前代码的直接建议

基于现在的 `tui.go`，我建议优先复用这些块：

- `renderRow` 这一类状态行，但目标位置从右侧面板迁移到输入框附近
- `renderMessageBlock` 这一类带 header 的内容块
- 各类 entry 渲染函数的公共 header/body/prefix 逻辑
- slash hint 的匹配与渲染规则

我不建议当前就做这些事：

- 把每一种 entry 都做成独立复杂对象
- 为了“组件化”引入额外 interface
- 把交互状态拆散到很多文件

## 9. 和普通前端组件复用的差异

终端 UI 和 Web UI 最大的不同在于：

- 布局受终端宽度变化影响更直接
- ANSI 样式会影响宽度计算
- 中文、emoji、代码块、表格更容易出现 wrap 偏差
- 很多问题不是“组件没复用”，而是“宽度和文本测量不稳定”

所以在 5hAgent 里，UI 复用的优先级通常是：

1. 宽度与布局规则稳定
2. 渲染块稳定
3. 样式 token 稳定
4. 文件级拆分

顺序不要反过来。
