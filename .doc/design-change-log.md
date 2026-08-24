# 设计影响日志

本文件按 git commit 粒度记录：哪些用户提示词、设计原则和开发约束影响了对应 diff。

用途：后续 agent 接手时，先读本文件，理解“为什么这批改动会长这样”，不要只看代码结果。

## 记录格式

```json
{
  "commit": "pending | <commit-hash>",
  "summary": "这次 diff 的一句话目的",
  "user_prompts": [
    "直接影响设计或代码的用户原话"
  ],
  "design_principles": [
    "由提示词沉淀出的原则"
  ],
  "diff_scope": [
    "受影响的文件或模块"
  ],
  "effect": [
    "这些原则如何改变了本次 diff"
  ],
  "review_notes": [
    "后续 review 时要特别注意的点"
  ]
}
```

## Entries

```json
{
  "commit": "pending",
  "summary": "把 walle 的开发原则、TODO 设计记录、Runtime 内部接入文档和部分 TUI/CLI 行为收敛到 Lazy/Simple-is-Beauty 方向。",
  "user_prompts": [
    "注意原则不要过度设计, 要足够Lazy 简单就是美",
    "这个文档只是设计，不要乱指导实际的实施过程。",
    "先更新一下我们开发过程中的提示词... 阅读别人的架构设计：世界是个草台班子... 新增设计：简单就是美... 写代码：Lazy原则... 调研思考：持续且深入",
    "你现在写的文档太臭太长了, 架构信息要弄成json结构, 再用json结构生成mermind ,(这两步都有skill) 然后具体的接口和使用方法 在后面写清楚(就是详细信息用少量文字+源码跳转链接表示) 把这一句.doc更新规范 固化到agent.md",
    "对于TODO里的HOOK功能, 应该支持在tool use前有prehook",
    "新增一个功能, debug 可以打印出发给LLM的请求的所有内容",
    "再新增todo，就是... Terminal工具，所有的都默认起Tmux来跑。",
    "新建一个代码蒸馏skill 包含这些代码行数分析, 模块分析; 寻找死代码，过时代码, 低复用的代码",
    "新的todo... walle可以加一些命令行工具... walle ps... walle attach xxx... tab还能提示出可选的 target",
    "do fix 这个用量统计的tokens 单位得写上, 然后spent 单位用美元"
  ],
  "design_principles": [
    "设计文档只记录问题、边界、取舍和风险，不替实际实施排优先级。",
    "开发 agent 阅读架构时不信复杂宣称，先看真实调用和真实行为。",
    "新增设计先用最小架构表达第一个真实需求，不提前做完整框架。",
    "写代码少写少改，但给不熟上下文的人留下必要注释。",
    ".doc 是内部开发文档；架构信息先写 JSON 拓扑事实源，再由 JSON 派生成 Mermaid，接口细节用短文字和源码链接表达。",
    "prehook 是 tool use 前的同步 gate；runtime hooks/tool_start/tool_end/tool_error 是事后事件通知。",
    "调试可观测性优先复用现有 --debug/logger，不先新增 tracing 系统。",
    "Terminal/tmux 承载先聚焦长命令可观察和可恢复，不引入完整 job system。",
    "代码蒸馏先统计和分级，标出可删除/待核验/保留，不直接重构。",
    "用量显示不能把 token 数伪装成美元，token 数必须标单位。"
  ],
  "diff_scope": [
    "AGENTS.md",
    "CLAUDE.md",
    ".doc/约定.md",
    ".doc/runtime-interface.md",
    ".TODO/0_TUI-fix.md",
    ".TODO/2_元数据.md",
    ".TODO/3_Tools相关.md",
    ".TODO/5_功能扩展与最小Runtime.md",
    ".TODO/6_代码蒸馏.md",
    ".TODO/7_Runtime对外接口.md",
    ".TODO/codex账户额度.md",
    ".TODO/验证skill加载.md",
    ".walle/skills/code-distillation/SKILL.md",
    "cmd/walle/main.go",
    "internal/cli/tui.go",
    "internal/cli/tui_commands.go",
    "internal/cli/tui_render.go",
    "internal/cli/tui_test.go",
    "internal/agent/agent.go",
    "internal/agent/callbacks.go",
    "internal/utils/tokenBudget.go",
    "internal/runtime/runtime.go",
    "internal/llm/client.go",
    "internal/context/ctx.go",
    "internal/tools/context_tool.go",
    "prompt/main.md",
    "prompt/tui.md"
  ],
  "effect": [
    "AGENTS.md 和 CLAUDE.md 增加开发判断原则，约束后续 agent 的设计、代码和调研方式。",
    ".doc/runtime-interface.md 从长文改为 JSON 拓扑事实源 + Mermaid 派生图 + 接口索引。",
    ".TODO 文件从实施路线改为设计记录，强调边界和待核验项。",
    "code-distillation skill 使代码规模统计、模块分析、死代码/过时代码/低复用扫描可复用。",
    "TUI 用量显示改为显式 tokens 单位，spent 使用美元占位而不是复用 token 数。",
    "部分 prompt/TUI/CLI 改动来自调试和可见性诉求，review 时应确认是否仍符合设计阶段边界。"
  ],
  "review_notes": [
    "本 entry 覆盖的是当前未提交 diff，commit 后应把 commit 字段改成真实 hash 或追加新 entry。",
    ".gitignore 已放行 .doc/ 和 .TODO/，内部设计与任务文件可正常进入 review。",
    "当前工作树有多处既有修改，review 时不要把所有 diff 都默认归因于同一个提示词。",
    "如果后续 agent 只做设计审阅，应先在 chat 给建议，等用户确认后再改文件。"
  ]
}
```


```json
{
  "commit": "pending",
  "summary": "按从简原则压缩 runtime/daemon 内部设计文档、整理 TODO 入口、固化中文注释规则，并接通最小 stop 控制。",
  "user_prompts": [
    "一切从简, 能用一句话讲清楚的事情绝对不用两句话 , 能在图里一图流展示一个模块,就不要写好几段来描述, 代码也是如此 , 检查一切, 整理一切",
    "另外注释都用中文, 我看不懂英文喵"
  ],
  "design_principles": [
    "内部设计文档先给一句话和一图流，再给最小表格。",
    "TODO 入口按主线、待核验、设计备忘分组，避免散落。",
    "新增或修改代码注释使用中文，保留代码标识符和外部 API 原文。"
  ],
  "diff_scope": [
    ".doc/0_入口.md",
    ".doc/约定.md",
    ".doc/runtime-daemon-boundary.md",
    ".doc/runtime-interface.md",
    ".TODO/0_入口.md",
    ".TODO/5_功能扩展与最小Runtime.md",
    ".TODO/6_代码蒸馏.md",
    ".TODO/7_Runtime对外接口.md",
    ".TODO/为啥会有tui.md",
    "AGENTS.md",
    "CLAUDE.md",
    "internal/**/*.go comments",
    "internal/systemd/control.go",
    "internal/runtime/daemon_session.go",
    "internal/tui/remote.go",
    "internal/tui/commands.go",
    "internal/systemd/control_test.go",
    "internal/tui/app_test.go"
  ],
  "effect": [
    "runtime/daemon 设计收敛为 runtime 独立工作、walled 管理 N 个 runtime。",
    "runtime-interface 从长接口文档压缩为拓扑事实源、一图流、接口表和边界。",
    "控制协议保持 list / attach / input / stop；detach 只作为 attached TUI 断开内部消息。",
    "attached TUI 的 /stop 通过 ProcessClient.Stop 发送到 daemon session，取消当前 Agent run context。",
    "代码注释审计后，当前 internal/cmd 的注释无纯英文残留。"
  ],
  "review_notes": [
    ".gitignore 已放行 .doc/ 和 .TODO/，新内部文档可正常进入 review。",
    "go test -count=1 ./...、go build -o walle ./cmd/walle、git diff --check 已通过。",
    "tmux 冒烟覆盖 attached TUI /provider、/model、/stop；捕获目录见 .traces/tui-runtime-stop-20260805-083014/。",
    "provider/model 行为改动仍需重点 review，尤其是远端 model catalog、DeepSeek 默认值和 ConfiguredProviders。"
  ]
}
```
