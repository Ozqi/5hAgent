// tui_tool_render.go - TUI 工具事件和工具块渲染
// 功能：处理 ToolEvent 状态回填，并渲染历史工具调用块。
package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/tools"
)

func (m *AppModel) applyToolEvent(event logger.ToolEvent) {
	displayName := fallback(tools.DisplayName(event.Name), event.Name)
	key := toolEventKey(event.Name, event.Args)
	summary := formatToolArgsSummary(event.Args)

	// result/error 依赖 tool name + args 回填最近的 running 行；找不到时补一条完成记录，避免丢事件。
	switch event.Kind {
	case "call":
		m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "running", ToolOutput: "running..."})
	case "result":
		idx := m.findRunningToolEntry(key, event.Name)
		output := summarizeToolEventOutput(event)
		if idx < 0 {
			m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "done", ToolOutput: output})
			return
		}
		m.entries[idx].ToolState = "done"
		m.entries[idx].ToolOutput = output
	case "error":
		idx := m.findRunningToolEntry(key, event.Name)
		output := summarizeToolEventOutput(event)
		if idx < 0 {
			m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "error", ToolOutput: output})
			return
		}
		m.entries[idx].ToolState = "error"
		m.entries[idx].ToolOutput = output
	default:
		m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "done", ToolOutput: strings.TrimSpace(stripANSI(event.Text))})
	}
}

func (m *AppModel) applyRunToolEvent(event logger.ToolEvent) {
	displayName := fallback(tools.DisplayName(event.Name), event.Name)
	switch event.Kind {
	case "call":
		m.ensureRunTableHeader()
		m.runLines = append(m.runLines, formatRunToolCall(displayName, event.Args))
	case "result":
		output := summarizeToolEventOutput(event)
		if len(m.runLines) == 0 {
			m.ensureRunTableHeader()
			m.runLines = append(m.runLines, formatRunToolCall(displayName, event.Args))
		}
		m.runLines[len(m.runLines)-1] += "  " + padCell("ok", 6) + "  " + truncateInline(output, 64)
	case "error":
		output := summarizeToolEventOutput(event)
		if len(m.runLines) == 0 {
			m.ensureRunTableHeader()
			m.runLines = append(m.runLines, formatRunToolCall(displayName, event.Args))
		}
		m.runLines[len(m.runLines)-1] += "  " + padCell("error", 6) + "  " + truncateInline(output, 64)
	}
	m.updateRunEntryContent("")
}

func (m *AppModel) ensureRunTableHeader() {
	if len(m.runLines) > 0 {
		return
	}
	m.runLines = append(m.runLines,
		padCell("Tool", 12)+"  "+padCell("Action", 10)+"  "+padCell("Target", 34)+"  "+padCell("State", 6)+"  Result",
		strings.Repeat("-", 12)+"  "+strings.Repeat("-", 10)+"  "+strings.Repeat("-", 34)+"  "+strings.Repeat("-", 6)+"  "+strings.Repeat("-", 24),
	)
}

func formatRunToolCall(name string, args string) string {
	action, target := toolActionTarget(args)
	return padCell(name, 12) + "  " + padCell(action, 10) + "  " + padCell(target, 34)
}

func toolActionTarget(args string) (string, string) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(args)), &raw); err != nil {
		return "-", truncateInline(args, 34)
	}
	action := "-"
	if v, ok := raw["action"]; ok {
		action = toolArgValue(v)
	}
	targetParts := make([]string, 0, 3)
	for _, key := range []string{"id", "status", "path", "pattern"} {
		if v, ok := raw[key]; ok {
			targetParts = append(targetParts, key+"="+toolArgValue(v))
		}
	}
	if len(targetParts) == 0 {
		for key, value := range raw {
			if key == "action" {
				continue
			}
			targetParts = append(targetParts, key+"="+toolArgValue(value))
		}
		sort.Strings(targetParts)
	}
	if len(targetParts) == 0 {
		return action, "-"
	}
	return action, strings.Join(targetParts, " ")
}

func padCell(text string, width int) string {
	text = truncateInline(text, width)
	padding := width - lipgloss.Width(text)
	if padding > 0 {
		text += strings.Repeat(" ", padding)
	}
	return text
}

func (m *AppModel) finishRunEntry(summary string) {
	m.updateRunEntryContent(summary)
	m.runEntry = -1
	m.runLines = nil
}

func (m *AppModel) updateRunEntryContent(summary string) {
	if m.runEntry < 0 || m.runEntry >= len(m.entries) {
		return
	}
	lines := append([]string{}, m.runLines...)
	if strings.TrimSpace(summary) != "" {
		lines = append(lines, compactRunSummary(summary)...)
	}
	if len(lines) == 0 {
		lines = append(lines, "running...")
	}
	m.entries[m.runEntry].Content = strings.Join(lines, "\n")
}

func compactRunSummary(summary string) []string {
	lines := strings.Split(strings.TrimSpace(summary), "\n")
	result := make([]string, 0, 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, line)
		if len(result) >= 3 {
			break
		}
	}
	return result
}

func (m *AppModel) findRunningToolEntry(key string, name string) int {
	displayName := tools.DisplayName(name)
	for i := len(m.entries) - 1; i >= 0; i-- {
		entry := m.entries[i]
		if entry.Role != roleHint || entry.ToolState != "running" {
			continue
		}
		if key != "" && entry.ToolKey == key {
			return i
		}
		if entry.ToolName == name || entry.ToolName == displayName {
			return i
		}
	}
	return -1
}

func toolEventKey(name string, args string) string {
	return name + "\x00" + strings.TrimSpace(args)
}

func formatToolArgsSummary(args string) string {
	args = strings.TrimSpace(args)
	if args == "" || args == "{}" {
		return ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return "[args=" + truncateMiddle(args, 180) + "]"
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+truncateMiddle(toolArgValue(raw[key]), 120))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func toolArgValue(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		return fmt.Sprintf("%g", vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func summarizeToolEventOutput(event logger.ToolEvent) string {
	clean := strings.TrimSpace(stripANSI(event.Text))
	// 兼容 logger 旧文本块格式（●/⎿/✗），也兼容当前 ToolEvent.Text 的短摘要。
	entry := parseToolBlock(clean)
	switch event.Kind {
	case "result":
		fields := append([]string{}, entry.Result...)
		if len(fields) == 0 {
			fields = strings.Split(strings.TrimSpace(strings.TrimPrefix(clean, "⎿ ")), "\n")
		}
		return compactOutputLines(fields, 4)
	case "error":
		if entry.Error != "" {
			return compactOutputLines([]string{entry.Error}, 2)
		}
		if event.Error != "" {
			return "error: " + truncateMiddle(event.Error, 180)
		}
		return compactOutputLines(strings.Split(clean, "\n"), 2)
	default:
		return compactOutputLines(strings.Split(clean, "\n"), 3)
	}
}

func compactOutputLines(lines []string, limit int) string {
	result := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "⎿ "))
		if line == "" {
			continue
		}
		result = append(result, truncateMiddle(line, 160))
		if len(result) >= limit {
			break
		}
	}
	if len(result) == 0 {
		return "(no output)"
	}
	if len(lines) > len(result) {
		result = append(result, "...")
	}
	return strings.Join(result, "\n")
}

func truncateInline(text string, maxLen int) string {
	text = strings.Join(strings.Fields(text), " ")
	return truncateMiddle(text, maxLen)
}
func (m *AppModel) renderToolHintEntry(entry conversationEntry, width int) string {
	stateIcon := "▮"
	stateColor := colorBlue
	switch entry.ToolState {
	case "running":
		stateIcon = spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		stateColor = colorBlue
	case "error":
		stateIcon = "▮"
		stateColor = colorError
	}

	name := fallback(entry.ToolName, "tool")
	args := strings.TrimSpace(entry.ToolArgs)
	if entry.ToolState != "running" {
		stateIcon = "▮"
	}
	header := lipgloss.NewStyle().Foreground(stateColor).Bold(true).Render(stateIcon) + " " + lipgloss.NewStyle().Foreground(stateColor).Render(name)
	if args != "" {
		header += " " + logger.Gray(args)
	}

	output := strings.TrimSpace(entry.ToolOutput)
	if output == "" {
		return header
	}
	lineWidth := max(8, width-4)
	lines := wrapVisibleText(compactToolOutputForWidth(output, lineWidth), lineWidth)
	if entry.ToolState == "error" {
		lines = loggerColorLines(lines, colorError)
	} else {
		lines = loggerColorLines(lines, colorResult)
	}
	return header + "\n" + indentLines(lines, "  └ ", "    ")
}

func loggerColorLines(text string, color lipgloss.Color) string {
	style := lipgloss.NewStyle().Foreground(color)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

func compactToolOutputForWidth(output string, width int) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, truncateMiddle(line, max(24, width)))
	}
	return strings.Join(result, "\n")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// toolEntry 表示解析后的工具条目
type toolEntry struct {
	Name   string
	Args   []string
	Result []string
	Error  string
}

func parseToolBlock(text string) toolEntry {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(text, ""))
	if clean == "" {
		return toolEntry{}
	}

	lines := strings.Split(clean, "\n")
	entry := toolEntry{}
	seenResult := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "● ") {
			rest := strings.TrimPrefix(line, "● ")
			rest = strings.TrimSuffix(rest, " [并发]")
			entry.Name = extractToolNameFromCall(rest)
			continue
		}
		if strings.HasPrefix(line, "⎿ ") {
			seenResult = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "⎿ "))
		}
		if strings.HasPrefix(line, "✗") {
			entry.Error = strings.TrimSpace(line)
			seenResult = true
			continue
		}
		if !seenResult {
			entry.Args = append(entry.Args, line)
			continue
		}
		entry.Result = append(entry.Result, line)
	}
	return entry
}

// extractToolName 从工具调用行提取工具名称
func extractToolNameFromCall(text string) string {
	// 格式: ToolName(args...) 或 ToolName
	text = strings.TrimSpace(text)
	// 找到第一个括号的位置
	if idx := strings.Index(text, "("); idx > 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}

func renderToolEntry(content string, width int) string {
	entry := parseToolBlock(content)
	if entry.Name == "" && len(entry.Args) == 0 && len(entry.Result) == 0 && entry.Error == "" {
		return renderToolUnknownEntry(content, width)
	}
	return renderToolCompactEntry(entry, width)
}

func renderToolCompactEntry(entry toolEntry, width int) string {
	var b strings.Builder
	icon := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("▮")
	name := lipgloss.NewStyle().Foreground(colorBlue).Render(fallback(entry.Name, "tool"))
	b.WriteString(icon + " " + name)

	if len(entry.Args) > 0 {
		joined := strings.Join(entry.Args, "   ·   ")
		wrapped := wrapVisibleLines(joined, max(8, width-2))
		b.WriteString("\n")
		for _, wl := range wrapped {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Faint(true).Render("  │ " + wl))
			b.WriteString("\n")
		}
	}

	if entry.Error != "" {
		b.WriteString("\n")
		for _, wl := range wrapVisibleLines(entry.Error, max(8, width-2)) {
			b.WriteString(lipgloss.NewStyle().Foreground(colorError).Render("  └ " + wl))
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}

	if len(entry.Result) > 0 {
		showLines := entry.Result
		truncated := false
		if len(showLines) > 2 {
			showLines = showLines[:2]
			truncated = true
		}
		if len(entry.Args) == 0 {
			b.WriteString("\n")
		}
		for _, line := range showLines {
			wrapped := wrapVisibleLines(truncateMiddle(line, max(24, width+12)), max(8, width-2))
			for _, wl := range wrapped {
				b.WriteString(lipgloss.NewStyle().Foreground(colorResult).Render("  └ " + wl))
				b.WriteString("\n")
			}
		}
		if truncated {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  └ ..."))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderToolUnknownEntry 渲染未知工具条目
func renderToolUnknownEntry(content string, width int) string {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(content, ""))
	if clean == "" {
		return lipgloss.NewStyle().Foreground(colorGray).Render("▮ tool\n  └ (empty)")
	}
	maxLen := width - 10
	if maxLen < 20 {
		maxLen = 20
	}
	return lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("▮") + " " + lipgloss.NewStyle().Foreground(colorGray).Render(truncateMiddle(clean, maxLen))
}
