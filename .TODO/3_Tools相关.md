# 工具相关


## （设计中）动态工具注册

可以通过生成脚本（输入输出安全，有--help等等...），注册成工具，后续LLM可以直接从工具列表直接调用。

### 确定结论

- 动态工具注册必须走“显式目录 + schema 生成 + 人工审查/启用”，不能让 LLM 写完脚本就自动进入默认工具列表。
- 工具脚本至少需要：可执行入口、`--help`、输入 JSON schema、输出 JSON schema、权限声明。
- 推荐目录放在项目 `.5hagent/tools/`，用户级复用工具再考虑 `~/.5hAgent/tools/`。
- 首版只支持启动时加载，不做运行期热加载；这和当前 skill 生命周期固定的设计一致。

对于动态工具的显示，（或者可以推广到所有工具）调用之前应该有一行话显示当前使用这个工具的指令，参数， 这个是在干嘛。就在工具调用UI块的第一行，

## Terminal 工具默认 tmux 执行

想法：工具里的 Bash/Terminal 能力，也就是当前接近 `base.exec_shell` 的命令执行，不应该只是一次性子进程。默认起 tmux 来跑，方便用户/Agent 之后 attach、capture、继续观察。

原则：简单就是美，先做最小包装，不引入完整 job system。

第一版目标：

- [ ] Terminal/exec_shell 类工具默认把命令放进项目级 tmux session/window/pane 执行。
- [ ] 返回值至少包含 tmux target、启动命令、退出码/当前状态、最近输出片段。
- [ ] 短命令仍要像现在一样能拿到 stdout/stderr；tmux 只是执行承载，不要求用户手动 attach 才能看结果。
- [ ] 长任务可以不断 capture 同一个 target，不因为一次 tool call 返回就丢失进程。
- [ ] session/window 命名保持可预测，例如包含 workspace、task/process id 或时间戳，避免散落不可识别的 tmux pane。
- [ ] 先不做远程 tmux、嵌套 tmux、完整进程调度、权限系统；需要时再扩。

待确认边界：

- [ ] 是否保留一个 `direct` escape hatch，给极短、无状态、需要精确 stdout 的命令使用。
- [ ] TUI/headless/daemon 是否共用同一 tmux 命名策略。

## 完成记录

- 工具调用块首行展示动作意图、工具名和参数：`333f3e5256ab3cab8a35f397b82c571e930d9368`
