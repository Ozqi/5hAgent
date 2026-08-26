// Package toolevent 格式化工具调用、结果和错误事件，供终端和 TUI 展示。
package toolevent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Ozqi/walle/internal/logger"
	"github.com/Ozqi/walle/internal/tools"
)

const toolIndent = "  "

type toolCallSummary struct {
	Title  string
	Fields []string
}

type toolResultSummary struct {
	Fields []string
	Lines  []string
}

// ToolEvent 是工具调用、结果或错误的结构化展示事件。
// Text 是可截断的展示摘要；Result 是完整工具结果；Error 是未格式化的原始错误文本。
// 格式化函数只生成 Text，不改写 Result 或 Error。
type ToolEvent struct {
	Kind       string
	Name       string
	Text       string
	Args       string
	Result     string
	Error      string
	Concurrent bool
}

func formatToolCall(indent string, name string, args string, concurrent bool) string {
	mode := ""
	if concurrent {
		mode = " [并发]"
	}

	summary := summarizeToolCall(name, args)
	var b strings.Builder
	b.WriteString("\n● ")
	b.WriteString(logger.Cyan(summary.Title))
	b.WriteString(logger.Gray(mode))
	b.WriteString("\n")
	for _, field := range summary.Fields {
		b.WriteString(indent)
		b.WriteString(logger.Gray(field))
		b.WriteString("\n")
	}
	return b.String()
}

func formatToolResultText(indent string, name string, args string, result string) string {
	var b strings.Builder
	if strings.TrimSpace(result) == "" {
		b.WriteString(indent)
		b.WriteString("⎿ ")
		b.WriteString(logger.Gray("(无输出)"))
		b.WriteString("\n")
		return b.String()
	}

	summary := summarizeToolResult(name, args, result)
	if len(summary.Fields) == 0 && len(summary.Lines) == 0 {
		return formatIndentedLines(indent+"⎿ ", splitDisplayLines(result, 4, 150))
	}

	if len(summary.Fields) > 0 {
		for i, field := range summary.Fields {
			prefix := indent
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
			prefix := indent + "  "
			if len(summary.Fields) == 0 && i == 0 {
				prefix = indent + "⎿ "
			}
			b.WriteString(prefix)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatToolErrorText(indent string, name string, args string, err error) (string, string) {
	summary := summarizeToolCall(name, args)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s⎿ %s %s\n", indent, logger.Red("✗"), logger.Red(summarizeToolError(name, err))))
	if len(summary.Fields) > 0 {
		for _, field := range summary.Fields {
			b.WriteString(indent)
			b.WriteString("  ")
			b.WriteString(logger.Gray(field))
			b.WriteString("\n")
		}
	} else if strings.TrimSpace(args) != "" {
		b.WriteString(indent)
		b.WriteString("  ")
		b.WriteString(logger.Gray("args: " + logger.TruncateString(args, 180)))
		b.WriteString("\n")
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	return b.String(), errText
}

func summarizeToolError(name string, err error) string {
	if err == nil {
		return tools.DisplayName(name) + " failed"
	}

	message := err.Error()
	if idx := strings.LastIndex(message, "err="); idx >= 0 {
		message = message[idx+len("err="):]
	}
	if idx := strings.Index(message, ". Make sure"); idx >= 0 {
		message = message[:idx]
	}
	if idx := strings.Index(message, ". Use absolute paths"); idx >= 0 {
		message = message[:idx]
	}

	path := ""
	if marker := "failed to read file '"; strings.Contains(message, marker) {
		start := strings.Index(message, marker) + len(marker)
		if end := strings.Index(message[start:], "'"); end >= 0 {
			path = message[start : start+end]
		}
	}

	reason := message
	if idx := strings.LastIndex(message, ": "); idx >= 0 && idx+2 < len(message) {
		reason = message[idx+2:]
	}

	displayName := tools.DisplayName(name)
	if path != "" {
		return fmt.Sprintf("%s failed: %s (%s)", displayName, shortenPath(path), reason)
	}
	return fmt.Sprintf("%s failed: %s", displayName, logger.TruncateString(strings.TrimSpace(message), 180))
}

// PrintToolCall 将工具调用直接打印到 stdout。
func PrintToolCall(name string, args string, concurrent bool) {
	fmt.Print(FormatToolCall(name, args, concurrent))
}

// FormatToolCall 生成工具调用文本。
func FormatToolCall(name string, args string, concurrent bool) string {
	return formatToolCall(toolIndent, name, args, concurrent)
}

// PrintToolResult 将工具结果直接打印到 stdout。
func PrintToolResult(name string, args string, result string) {
	fmt.Print(FormatToolResult(name, args, result))
}

// FormatToolResult 生成工具结果文本。
func FormatToolResult(name string, args string, result string) string {
	return formatToolResultText(toolIndent, name, args, result)
}

// PrintToolError 将工具错误直接打印到 stdout。
func PrintToolError(name string, args string, err error) {
	text, _ := FormatToolError(name, args, err)
	fmt.Print(text)
}

// FormatToolError 生成展示文本和原始错误文本。
func FormatToolError(name string, args string, err error) (string, string) {
	return formatToolErrorText(toolIndent, name, args, err)
}

func summarizeToolCall(name string, args string) toolCallSummary {
	displayName := tools.DisplayName(name)
	summary := toolCallSummary{Title: displayName}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		if strings.TrimSpace(args) != "" {
			summary.Fields = append(summary.Fields, "args: "+logger.TruncateString(args, 100))
		}
		return summary
	}

	switch displayName {
	case "read_file":
		summary.Fields = append(summary.Fields,
			formatField("path", shortenPath(stringValue(raw["path"]))),
			formatField("lines", formatLineRange(intValue(raw["offset"]), intValue(raw["limit"]))),
		)
	case "grep":
		summary.Fields = append(summary.Fields,
			formatField("pattern", stringValue(raw["pattern"])),
			formatField("path", shortenPath(stringValue(raw["path"]))),
		)
		if v := stringValue(raw["type"]); v != "" {
			summary.Fields = append(summary.Fields, formatField("type", v))
		}
	case "glob":
		summary.Fields = append(summary.Fields,
			formatField("pattern", stringValue(raw["pattern"])),
			formatField("path", shortenPath(stringValue(raw["path"]))),
		)
	case "list_dir":
		summary.Fields = append(summary.Fields,
			formatField("path", shortenPath(stringValue(raw["path"]))),
		)
		if boolValue(raw["recursive"]) {
			summary.Fields = append(summary.Fields, formatField("recursive", "true"))
		}
	case "exec_shell":
		summary.Fields = append(summary.Fields, formatField("command", logger.TruncateString(stringValue(raw["command"]), 120)))
	default:
		keys := make([]string, 0, len(raw))
		for key := range raw {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			summary.Fields = append(summary.Fields, formatField(key, logger.TruncateString(valueString(raw[key]), 100)))
		}
	}

	summary.Fields = compactFields(summary.Fields)
	return summary
}

func summarizeToolResult(name string, args string, result string) toolResultSummary {
	displayName := tools.DisplayName(name)
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
			lines = append(lines, fmt.Sprintf("%s:%d  %s", file, line, logger.TruncateString(text, 80)))
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
			fields = append(fields, formatField("stderr", logger.TruncateString(first, 100)))
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
		lines = append(lines, logger.TruncateString(line, maxWidth))
	}
	if len(parts) > limit {
		lines = append(lines, logger.Gray("..."))
	}
	return lines
}

func linesWithEllipsis(lines []string, total int, shown int) []string {
	if total > shown {
		return append(lines, logger.Gray("..."))
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
	s, _ := v.(string)
	return s
}

func boolValue(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func intValue(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
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
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func asObjects(v interface{}) []map[string]interface{} {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if ok {
			result = append(result, obj)
		}
	}
	return result
}

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
