# Agent Systemd 调试说明

> 由 GPT-5.5 于 2026-07-02 更新。本文只保留当前项目的手工调试入口；旧 L0-L4 分层测试规范已废弃。

## 摘要

Agent Systemd 当前是最小 daemon 调度骨架：监听 Project 下 `.5hagent/task.md`，把 `pending/in_progress` 任务转换为 `task.created`，再通过 `Runtime.RunProcess` 启动 AgentProcess。日常验证以真实 TUI/headless/daemon 交互为主，优先模拟用户操作，而不是维护一套冗长的分层测试矩阵。

```mermaid
flowchart LR
    task[".5hagent/task.md"] --> source["TaskFileEventSource"]
    source --> sys["AgentSystemd.Run"]
    sys --> runner["Runtime.RunProcess"]
    runner --> agent["Agent.RunStream"]
    agent --> report["process report / worklog"]
```

## 调试入口

代码改动需要快速自检时，在仓库根目录运行：

```bash
go test ./...
go build -o /private/tmp/5hagent-check cmd/5hagent/main.go
git diff --check
```

真实 LLM、headless、daemon、TUI 调试统一使用：

```text
/Users/bytedance/Proj/5hWorkSpace
```

Project 数据目录：

```text
/Users/bytedance/Proj/5hWorkSpace/.5hagent/
```

需要检查的产物：

```text
.5hagent/task.md
.5hagent/reports/
.5hagent/agents/<agent-id>/logs/
~/.5hAgent/logs/
```

## tmux TUI 调试

优先通过 tmux 模拟真人和 TUI 交互。用户常用窗口可以通过：

```bash
tmux list-windows -a
tmux list-panes -a -F '#{session_name}:#{window_index}.#{pane_index} #{window_name} #{pane_current_path} #{pane_current_command}'
```

如果没有现成 debug 窗口，使用固定 workspace 新建一个：

```bash
tmux new-window -n 'five hour agent-debug' -c /Users/bytedance/Proj/5hWorkSpace '/Users/bytedance/Proj/5hAgent/5hagent'
tmux capture-pane -t 'five hour agent-debug' -p -S -120
```

常用交互：

```bash
tmux send-keys -t 'five hour agent-debug' '/task list' Enter
tmux capture-pane -t 'five hour agent-debug' -p -S -120
tmux send-keys -t 'five hour agent-debug' '/run' Enter
tmux capture-pane -t 'five hour agent-debug' -p -S -120
```

调试重点：

- 输入框是否能正常输入和提交。
- slash hint 是否换行正常。
- `/task list` 是否能读当前 workspace 的 `.5hagent/task.md`。
- `/run` 是否能连续处理 `pending/in_progress` 任务。
- 工具调用是否在 TUI 中可见。
- 输出结束后状态是否回到 `idle`。

## Headless / Daemon 调试

```bash
cd /Users/bytedance/Proj/5hWorkSpace
/Users/bytedance/Proj/5hAgent/5hagent run --quiet
/Users/bytedance/Proj/5hAgent/5hagent daemon --poll 1s
```

检查项：

- `.5hagent/task.md` 状态是否更新。
- `.5hagent/reports/` 是否生成报告。
- `.5hagent/agents/<agent-id>/logs/` 是否生成 worklog。
- `~/.5hAgent/logs/` 是否有 runtime 错误。
