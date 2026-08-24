# 设计影响日志

> 由 Claude Fable 5 于 2026-08-24 重写。
> 用途：记录当前未提交设计/清理 diff 的来源和 review 关注点，避免旧 pending 记录引用已删除文件。

## pending：文档和运行时边界收敛

```json
{
  "commit": "pending",
  "summary": "把 walle 的开发契约、spec、架构图、默认配置和少量 TUI/daemon 行为收敛到更简单的当前事实。",
  "user_prompts": [
    "注意原则不要过度设计, 要足够Lazy 简单就是美",
    "一切从简, 能用一句话讲清楚的事情绝对不用两句话 , 能在图里一图流展示一个模块,就不要写好几段来描述, 代码也是如此 , 检查一切, 整理一切",
    "另外注释都用中文, 我看不懂英文喵",
    "检查项目中的死代码，不合理的地方，冗余的地方进行更新和删减",
    "我觉得spec目录下的文档写的都很垃圾，图也一般，并且文档也没引用或者使用这些架构图，这个文档整体全都可以重构"
  ],
  "design_principles": [
    "代码和文档都优先少写、少改、少绕路；只保留能指导后续实现或 review 的事实。",
    "已实现行为以代码为真源；planned 能力必须显式标注，不能伪装成已实现。",
    "架构图必须被 spec 正文引用；不用的图和过期 TODO 直接删除。",
    "MCP stdio client 和 RegisterMCPTools 虽然不在默认启动链路中使用，但属于 spec 明确保留的 optional 接入点，不能按普通死代码删除。"
  ],
  "diff_scope": [
    "AGENTS.md",
    "CLAUDE.md",
    ".TODO/0_入口.md",
    ".TODO/10_动态终端工具.md",
    ".TODO/11_上下文掩盖与LLM投影.md",
    ".TODO/1_关于持久化.md",
    ".doc/0_入口.md",
    ".doc/约定.md",
    ".doc/tui/bubble-tea-agent-tui.md",
    ".spec/*.md",
    ".spec/diagrams/*.mmd",
    ".spec/diagrams/walle-architecture-topology.json",
    ".env.example",
    "install.sh",
    "cmd/walle/interactive_command.go",
    "internal/tui/commands.go"
  ],
  "effect": [
    "spec 从长表格堆叠改为短契约：职责、关键文件、稳定边界、禁止事项、验收。",
    "架构图迁移到 .spec/diagrams，并在 README 和对应模块 spec 正文中直接引用。",
    "删除过期 TODO 和旧 .doc 图副本，避免同一架构事实多处漂移。",
    "默认安装配置切到已验证的本地 Ollama ornith:9b，并保留旧 env 迁移逻辑。",
    "TUI 客户端收敛为 attached-only：输入只转发 daemon control，本地不再持有 Agent/Runtime 业务对象。",
    "TUI 修复滚轮 escape 污染、重复 daemon disconnected，并通过渲染缓存和刷新节流降低长历史 streaming 卡顿。",
    "interactive daemon 启动等待期间监听子进程退出，避免 daemon 早退时前台等满超时。"
  ],
  "review_notes": [
    "go vet ./...、go build -o walle ./cmd/walle、git diff --check 已通过。",
    "未运行 TUI tmux 视觉验收；本轮 TUI 改动包含 attached-only 收口、滚轮输入过滤、断线去重和渲染节流。",
    "未提交、未推送。"
  ]
}
```
