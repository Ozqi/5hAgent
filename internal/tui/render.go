// tui_render.go - TUI 布局和状态栏渲染
// 功能：渲染主布局、输入栏、运行状态和底部 metadata。
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lzq/5hAgent/internal/tools"
)

func renderMainPane(m *AppModel) string {
	width := max(20, m.width)
	snapshot := m.snapshot()
	header := renderTopStatus(snapshot, m.modelName, m.agentName, width)
	conversationHeight := max(1, m.viewport.Height)
	conversation := lipgloss.NewStyle().Height(conversationHeight).Render(renderViewportPane(m.viewport, m.viewText))
	inputBlock := renderInputBar(m.input.View(), width)
	slashHint := m.renderSlashHint(max(12, width-4))
	footer := renderInputFooter(snapshot, m.sessionID, width)
	blocks := []string{
		conversation,
		header,
	}
	if slashHint != "" {
		blocks = append(blocks, slashHint)
	}
	blocks = append(blocks, inputBlock, footer)
	content := lipgloss.JoinVertical(lipgloss.Left, blocks...)
	return mainViewStyle.Width(width).Render(content)
}

// renderInputBar 使用参考 tmux 对话窗口的上下深灰线样式包住输入行。
// 参数：inputView 是 textarea 当前输出；width 是终端主列宽度。
func renderInputBar(inputView string, width int) string {
	width = max(12, width)
	line := lipgloss.NewStyle().Foreground(colorInputBg).Render(strings.Repeat("▄", width))
	bottom := lipgloss.NewStyle().Foreground(colorInputBg).Render(strings.Repeat("▀", width))
	body := inputShellStyle.Width(width).Render(compactInputView(inputView))
	return strings.Join([]string{line, body, bottom}, "\n")
}

func compactInputView(inputView string) string {
	lines := strings.Split(inputView, "\n")
	for len(lines) > 1 && isEmptyInputPromptLine(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func isEmptyInputPromptLine(line string) bool {
	plain := strings.TrimSpace(stripANSI(line))
	return plain == "" || plain == ">"
}

// renderTopStatus 渲染输入框上方的高频运行状态。
// 参数：snapshot 为运行快照；modelName/agentName 来自 AppModel；width 为当前主列宽度。
func renderTopStatus(snapshot statusSnapshot, modelName string, _ string, width int) string {
	meta := snapshot.Runtime
	if width < 72 {
		parts := []string{
			lipgloss.NewStyle().Foreground(colorCommand).Render(truncateMiddle(fallback(modelName, "-"), 22)),
			renderState(meta.State, meta.Busy),
		}
		if meta.Turn > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("turn %d", meta.Turn)))
		}
		if meta.ToolCallsTotal > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("tools %d", meta.ToolCallsTotal)))
		}
		if meta.ContextTokens > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(renderTokenStatus(meta)))
		}
		return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
	}
	parts := []string{
		lipgloss.NewStyle().Foreground(colorCommand).Render(truncateMiddle(fallback(modelName, "-"), max(16, width/3))),
		renderState(meta.State, meta.Busy),
	}
	if meta.Turn > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("turn %d", meta.Turn)))
	}
	if meta.ToolCallsTotal > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("tools %d", meta.ToolCallsTotal)))
	}
	if meta.LastToolName != "" {
		last := truncateMiddle(fallback(tools.DisplayName(meta.LastToolName), meta.LastToolName), 24)
		parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render("last "+last))
	}
	if meta.ContextTokens > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(renderTokenStatus(meta)))
	}
	return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
}

func renderTokenStatus(meta runtimeMeta) string {
	context := fmt.Sprintf("ctx %d tokens", meta.ContextTokens)
	if meta.ContextWindow > 0 {
		context = fmt.Sprintf("ctx %d/%d tokens", meta.ContextTokens, meta.ContextWindow)
	}
	return fmt.Sprintf("%s · total %d tokens · spent $--", context, meta.SessionTokens)
}

// renderInputFooter 渲染输入框下方的低频上下文状态。
// 参数：snapshot 为运行快照；width 为当前主列宽度。
func renderInputFooter(snapshot statusSnapshot, _ string, width int) string {
	meta := snapshot.Runtime
	if width < 72 {
		parts := make([]string, 0, 4)
		if meta.Workdir != "" && meta.Workdir != "-" {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorWhite).Render(truncateMiddle(meta.Workdir, 24)))
		}
		if meta.Git.Repo {
			branch := truncateMiddle(fallback(meta.Git.Branch, "detached"), 12)
			if meta.Git.Dirty {
				branch += "*"
			}
			if meta.Git.Worktree {
				parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render("worktree "+branch))
			} else {
				parts = append(parts, lipgloss.NewStyle().Foreground(colorBlue).Render("git "+branch))
			}
		}
		if meta.TotalTasks > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorCommand).Render(fmt.Sprintf("tasks %d active / %d total", meta.ActiveTasks, meta.TotalTasks)))
		}
		if meta.ScrollPercent < 100 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("scroll %d%%", meta.ScrollPercent)))
		}
		return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
	}
	parts := make([]string, 0, 8)
	if meta.Workdir != "" && meta.Workdir != "-" {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorWhite).Render(truncateMiddle(meta.Workdir, 36)))
	}
	if meta.Git.Repo {
		branch := truncateMiddle(fallback(meta.Git.Branch, "detached"), 18)
		prefix := "git "
		color := colorBlue
		if meta.Git.Worktree {
			prefix = "worktree "
			color = colorGreen
		}
		if meta.Git.Dirty {
			branch += "*"
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(color).Render(prefix+branch))
		if meta.Git.Shortstat != "" {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorError).Render("diff "+truncateMiddle(meta.Git.Shortstat, 20)))
		}
	}
	if meta.ContextMessages > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("msgs %d", meta.ContextMessages)))
	}
	if meta.ContextSummaries > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("sum %d", meta.ContextSummaries)))
	}
	if meta.TotalTasks > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorCommand).Render(fmt.Sprintf("tasks %d active / %d total", meta.ActiveTasks, meta.TotalTasks)))
	}
	if len(snapshot.EnabledSkills) > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render(skillSummary(snapshot.EnabledSkills)))
	}
	if len(snapshot.HighlightedTaskLine) > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorCommand).Render("focus "+truncateMiddle(strings.Join(snapshot.HighlightedTaskLine, ","), 48)))
	}
	if meta.ScrollPercent < 100 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("scroll %d%%", meta.ScrollPercent)))
	}
	return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
}
