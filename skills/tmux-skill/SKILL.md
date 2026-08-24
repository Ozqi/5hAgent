---
name: tmux-skill
description: 使用 tmux 承载长任务、运行和检查 TUI、捕获真实终端画面，并管理可恢复的 session/window/pane。用户提到 tmux、长时间命令、后台保活、终端截图、TUI 验收或 SSH 断线恢复时使用。
---

# tmux 自动化

tmux 只用于需要保活、交互或真实终端画面的任务。秒级非交互命令直接执行，不为简单任务创建 session。

## 前置检查

```bash
tmux -V
tmux list-sessions
tmux list-panes -a -F '#{session_name}:#{window_index}.#{pane_index} #{pane_id} #{pane_current_command} #{pane_current_path}'
```

缺少 tmux 时直接报告，不自动安装。不要假设 prefix、插件或用户配置。

## 最小工作流

1. 确认命令、工作目录、预计耗时、日志和停止方式。
2. 优先复用 Agent 所在或用户正在查看的 session；需要隔离时才新建。
3. 创建固定尺寸的默认 shell，再用 `send-keys` 启动命令，便于人工接管。
4. 用 `capture-pane` 检查状态；不要高频轮询。
5. 报告 session、pane、日志和 attach 命令；按用户要求保留或清理。

```bash
tmux new-session -d -s <session> -n <window> -x 120 -y 40 -c <workdir>
tmux send-keys -t <session>:<window>.0 '<command>' Enter
tmux capture-pane -t <session>:<window>.0 -p -S -120
tmux attach -t <session>
```

## TUI 验收

至少检查正常尺寸和窄尺寸，观察边框、换行、输入框、状态区和滚动行为。

```bash
tmux new-session -d -s <name> -x 100 -y 28 -c <workdir> '<command>'
tmux capture-pane -t <name> -p -S -80
tmux resize-window -t <name> -x 80 -y 24
tmux capture-pane -t <name> -p -S -80
tmux send-keys -t <name> '<keys>'
```

## 安全边界

- 不操作无法确认归属的 session/window/pane。
- 不默认启用 `synchronize-panes`。
- 不用 tmux 绕过审批、认证或高风险操作确认。
- `capture-pane` 只是终端快照；需要审计时显式写日志。
- SSH、多主机写操作和 destructive git 命令仍需确认。
