// toolprint.go - 工具调用格式化输出
// 功能：ToolCall/ToolResult/ToolError 的终端展示（带颜色和缩进）
package logger

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type toolCallSummary struct {
	Title  string
	Fields []string
}

type toolResultSummary struct {
	Fields []string
	Lines  []string
}

type ToolEvent struct {
	Kind string
	Name string
	Text string
}

var (
	toolEventSink   func(ToolEvent)
	toolEventSinkMu sync.RWMutex
)

func SetToolEventSink(sink func(ToolEvent)) {
	toolEventSinkMu.Lock()
	defer toolEventSinkMu.Unlock()
	toolEventSink = sink
}

func currentToolEventSink() func(ToolEvent) {
	toolEventSinkMu.RLock()
	defer toolEventSinkMu.RUnlock()
	return toolEventSink
}

const toolIndent = "  "

func formatToolCallText(name string, args string, concurrent bool) string {
	mode := ""
	if concurrent {
		mode = " [并发]"
	}

	summary := summarizeToolCall(name, args)
	var b strings.Builder
	b.WriteString("\n● ")
	b.WriteString(Cyan(summary.Title))
	b.WriteString(Gray(mode))
	b.WriteString("\n")
	for _, field := range summary.Fields {
		b.WriteString(toolIndent)
		b.WriteString(Gray(field))
		b.WriteString("\n")
	}
	return b.String()
}

func formatToolResultText(name string, args string, result string) string {
	var b strings.Builder
	if strings.TrimSpace(result) == "" {
		b.WriteString(toolIndent)
		b.WriteString("⎿ ")
		b.WriteString(Gray("(无输出)"))
		b.WriteString("\n")
		return b.String()
	}

	summary := summarizeToolResult(name, args, result)
	if len(summary.Fields) == 0 && len(summary.Lines) == 0 {
		return formatIndentedLines(toolIndent+"⎿ ", splitDisplayLines(result, 4, 150))
	}

	if len(summary.Fields) > 0 {
		for i, field := range summary.Fields {
			prefix := toolIndent
			if i == 0 {
				prefix += "⎿ "
			} else {
				prefix += "  "
			}
			b.WriteString(prefix)
			b.WriteString(field)
			b.WriteString("\n")
		}
	}

	if len(summary.Lines) > 0 {
		for i, line := range summary.Lines {
			prefix := toolIndent + "  "
			if len(summary.Fields) == 0 && i == 0 {
				prefix = toolIndent + "⎿ "
			}
			b.WriteString(prefix)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatIndentedLines(prefix string, lines []string) string {
	var b strings.Builder
	for i, line := range lines {
		linePrefix := "  "
		if i == 0 {
			linePrefix = prefix
		}
		b.WriteString(linePrefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func splitDisplayLines(text string, limit int, maxWidth int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, "\n")
	lines := make([]string, 0, min(limit, len(parts)))
	for i, line := range parts {
		if i >= limit {
			break
		}
		lines = append(lines, TruncateString(line, maxWidth))
	}
	if len(parts) > limit {
		lines = append(lines, Gray("..."))
	}
	return lines
}

func linesWithEllipsis(lines []string, total int, shown int) []string {
	if total > shown {
		return append(lines, Gray("..."))
	}
	return lines
}

func compactFields(fields []string) []string {
	filtered := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			filtered = append(filtered, field)
		}
	}
	return filtered
}

func formatField(key string, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s", key, value)
}

func formatLineRange(offset int, limit int) string {
	if offset <= 0 && limit <= 0 {
		return ""
	}
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		return fmt.Sprintf("from %d", offset)
	}
	return fmt.Sprintf("%d-%d", offset, offset+limit-1)
}

func shortenPath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	if len(parts) <= 4 {
		return clean
	}
	return filepath.Join("...", parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1])
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return valueString(v)
}

func boolValue(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func intValue(v interface{}) int {
	if n, ok := v.(float64); ok {
		return int(n)
	}
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}

func valueString(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		return fmt.Sprintf("%d", int(vv))
	case bool:
		if vv {
			return "true"
		}
		return "false"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// asObjects 转换 interface{} 为 []map[string]interface{}
func asObjects(v interface{}) []map[string]interface{} {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// stringSlice 转换 interface{} 为 []string
func stringSlice(v interface{}) []string {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// PrintToolCall 打印工具调用
func PrintToolCall(name string, args string, concurrent bool) {
	text := formatToolCallText(name, args, concurrent)
	if sink := currentToolEventSink(); sink != nil {
		sink(ToolEvent{Kind: "call", Name: name, Text: text})
		return
	}
	fmt.Print(text)
}

// PrintToolResult 打印工具执行结果
func PrintToolResult(name string, args string, result string) {
	text := formatToolResultText(name, args, result)
	if sink := currentToolEventSink(); sink != nil {
		sink(ToolEvent{Kind: "result", Name: name, Text: text})
		return
	}
	fmt.Print(text)
}

// PrintToolError 打印工具执行错误
func PrintToolError(name string, args string, err error) {
	_ = summarizeToolCall(name, args)
	text := fmt.Sprintf("%s⎿ %s %s\n", toolIndent, Red("✗"), Red(err.Error()))
	if sink := currentToolEventSink(); sink != nil {
		sink(ToolEvent{Kind: "error", Name: name, Text: text})
		return
	}
	fmt.Print(text)
}

// PrintToolStatus 打印工具状态信息
func PrintToolStatus(message string) {
	text := fmt.Sprintf("%s⎿ %s\n", toolIndent, Gray(message))
	if sink := currentToolEventSink(); sink != nil {
		sink(ToolEvent{Kind: "status", Text: text})
		return
	}
	fmt.Print(text)
}

// PrintToolSummary 打印工具执行汇总
func PrintToolSummary(message string) {
	text := fmt.Sprintf("\n%s%s\n", toolIndent, Gray(message))
	if sink := currentToolEventSink(); sink != nil {
		sink(ToolEvent{Kind: "summary", Text: text})
		return
	}
	fmt.Print(text)
}

func summarizeToolCall(name string, args string) toolCallSummary {
	displayName := toolsDisplayName(name)
	summary := toolCallSummary{Title: displayName}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		if strings.TrimSpace(args) != "" {
			summary.Fields = append(summary.Fields, "args: "+TruncateString(args, 100))
		}
		return summary
	}

	switch displayName {
	case "read_file":
		summary.Fields = append(summary.Fields,
			formatField("path", shortenPath(stringValue(raw["path"]))),
			formatField("lines", formatLineRange(intValue(raw["offset"]), intValue(raw["limit"]))))
	case "grep":
		summary.Fields = append(summary.Fields,
			formatField("pattern", stringValue(raw["pattern"])),
			formatField("path", shortenPath(stringValue(raw["path"]))))
		if v := stringValue(raw["type"]); v != "" {
			summary.Fields = append(summary.Fields, formatField("type", v))
		}
	case "glob":
		summary.Fields = append(summary.Fields,
			formatField("pattern", stringValue(raw["pattern"])),
			formatField("path", shortenPath(stringValue(raw["path"]))))
	case "list_dir":
		summary.Fields = append(summary.Fields,
			formatField("path", shortenPath(stringValue(raw["path"]))))
		if boolValue(raw["recursive"]) {
			summary.Fields = append(summary.Fields, formatField("recursive", "true"))
		}
	case "exec_shell":
		summary.Fields = append(summary.Fields, formatField("command", TruncateString(stringValue(raw["command"]), 120)))
	default:
		keys := make([]string, 0, len(raw))
		for key := range raw {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			summary.Fields = append(summary.Fields, formatField(key, TruncateString(valueString(raw[key]), 100)))
		}
	}

	summary.Fields = compactFields(summary.Fields)
	return summary
}

func summarizeToolResult(name string, args string, result string) toolResultSummary {
	displayName := toolsDisplayName(name)
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return toolResultSummary{Lines: splitDisplayLines(result, 4, 150)}
	}

	switch displayName {
	case "read_file":
		content := stringValue(raw["content"])
		fields := []string{formatField("total lines", valueString(raw["total_lines"]))}
		lines := splitDisplayLines(content, 6, 200)
		if len(lines) > 0 {
			fields = append(fields, "content:")
		}
		return toolResultSummary{Fields: compactFields(fields), Lines: lines}
	case "grep":
		matches := asObjects(raw["matches"])
		fields := []string{formatField("matches", fmt.Sprintf("%d", len(matches)))}
		lines := make([]string, 0, 4)
		files := map[string]struct{}{}
		for i, match := range matches {
			if i >= 4 {
				break
			}
			file := shortenPath(stringValue(match["file"]))
			files[file] = struct{}{}
			line := intValue(match["line"])
			text := stringValue(match["text"])
			lines = append(lines, fmt.Sprintf("%s:%d  %s", file, line, TruncateString(text, 80)))
		}
		fields = append(fields, formatField("files", fmt.Sprintf("%d", len(files))))
		return toolResultSummary{Fields: compactFields(fields), Lines: linesWithEllipsis(lines, len(matches), 4)}
	case "glob":
		files := stringSlice(raw["files"])
		shown := make([]string, 0, min(5, len(files)))
		for i, path := range files {
			if i >= 5 {
				break
			}
			shown = append(shown, shortenPath(path))
		}
		return toolResultSummary{
			Fields: compactFields([]string{formatField("matches", fmt.Sprintf("%d", len(files)))}),
			Lines:  linesWithEllipsis(shown, len(files), 5),
		}
	case "list_dir":
		files := asObjects(raw["files"])
		fields := []string{formatField("entries", fmt.Sprintf("%d", len(files)))}
		lines := make([]string, 0, 5)
		for i, file := range files {
			if i >= 5 {
				break
			}
			path := shortenPath(stringValue(file["path"]))
			if boolValue(file["is_dir"]) {
				path += "/"
			}
			lines = append(lines, path)
		}
		return toolResultSummary{Fields: compactFields(fields), Lines: linesWithEllipsis(lines, len(files), 5)}
	case "exec_shell":
		stdout := stringValue(raw["stdout"])
		stderr := stringValue(raw["stderr"])
		fields := []string{formatField("exit code", valueString(raw["returncode"]))}
		if stderr != "" {
			first := stderr
			if idx := strings.Index(stderr, "\n"); idx >= 0 {
				first = stderr[:idx]
			}
			fields = append(fields, formatField("stderr", TruncateString(first, 100)))
		}
		lines := splitDisplayLines(stdout, 5, 160)
		if len(lines) == 0 && stderr != "" {
			lines = splitDisplayLines(stderr, 3, 160)
		}
		return toolResultSummary{Fields: compactFields(fields), Lines: lines}
	default:
		return toolResultSummary{Lines: splitDisplayLines(result, 4, 150)}
	}
}

func toolsDisplayName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}
