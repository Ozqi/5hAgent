// markdown_stream.go - Markdown 流式渲染
// 功能：逐步输出 Markdown（代码块/标题/列表/引用），用于终端着色展示
// 主要类型：MarkdownStreamRenderer
// 导出函数：NewMarkdownStreamRenderer, renderMarkdownForTerminal
package cli

import (
	"regexp"
	"strings"

	"github.com/lzq/5hAgent/internal/logger"
)

/*
type MarkdownStreamRenderer struct {
	w       io.Writer
	prefix  string
	started bool
	pending strings.Builder
}

func NewMarkdownStreamRenderer(w io.Writer, prefix string) *MarkdownStreamRenderer {
	return &MarkdownStreamRenderer{w: w, prefix: prefix}
}

func (r *MarkdownStreamRenderer) WriteToken(token string) {
	if token == "" {
		return
	}
	r.pending.WriteString(token)
	r.flushReady(false)
}

func (r *MarkdownStreamRenderer) Flush() {
	r.flushReady(true)
}

func (r *MarkdownStreamRenderer) flushReady(force bool) {
	ready, rest := splitReadyMarkdown(r.pending.String(), force)
	if ready == "" {
		return
	}
	r.pending.Reset()
	r.pending.WriteString(rest)
	r.write(renderMarkdownForTerminal(ready, true))
}

func (r *MarkdownStreamRenderer) write(text string) {
	if text == "" {
		return
	}
	if !r.started {
		fmt.Fprint(r.w, r.prefix)
		r.started = true
	}
	fmt.Fprint(r.w, text)
}
*/

func splitReadyMarkdown(input string, force bool) (string, string) {
	if force {
		return input, ""
	}

	var lastBoundary int
	inCodeBlock := false
	for i := 0; i < len(input); {
		lineEnd := strings.IndexByte(input[i:], '\n')
		if lineEnd < 0 {
			break
		}
		lineEnd += i + 1
		line := input[i:lineEnd]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			if !inCodeBlock {
				lastBoundary = lineEnd
			}
		}
		if !inCodeBlock && trimmed == "" {
			lastBoundary = lineEnd
		}
		i = lineEnd
	}

	if lastBoundary == 0 {
		return "", input
	}
	return input[:lastBoundary], input[lastBoundary:]
}

func renderMarkdownForTerminal(input string, color bool) string {
	blocks := splitMarkdownBlocks(input)
	if len(blocks) == 0 {
		return ""
	}

	rendered := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		rendered = append(rendered, renderMarkdownBlock(block, color))
	}
	return strings.Join(rendered, "\n")
}

func splitMarkdownBlocks(input string) []string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	blocks := make([]string, 0)
	var current []string
	inCodeBlock := false

	flush := func() {
		if len(current) == 0 {
			return
		}
		blocks = append(blocks, strings.Join(current, "\n"))
		current = nil
	}

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "```") {
			if inCodeBlock {
				current = append(current, line)
				inCodeBlock = false
				flush()
			} else {
				flush()
				inCodeBlock = true
				current = append(current, line)
			}
			continue
		}

		if inCodeBlock {
			current = append(current, line)
			continue
		}

		if trimmedLine == "" {
			flush()
			continue
		}

		if isStandaloneMarkdownLine(trimmedLine) {
			flush()
			blocks = append(blocks, line)
			continue
		}

		current = append(current, line)
	}

	flush()
	return blocks
}

func isStandaloneMarkdownLine(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, ">") || isListLine(line)
}

func isListLine(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	for i := 0; i < len(line); i++ {
		if line[i] < '0' || line[i] > '9' {
			return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
		}
	}
	return false
}

func renderMarkdownBlock(block string, color bool) string {
	trimmed := strings.TrimSpace(block)
	switch {
	case strings.HasPrefix(trimmed, "```"):
		return renderCodeBlock(trimmed, color)
	case strings.HasPrefix(trimmed, "#"):
		return applyInlineMarkdown(colorHeading(trimmed), color)
	case strings.HasPrefix(trimmed, ">"):
		return colorQuote(trimmed, color)
	case isListLine(trimmed):
		return colorListLine(trimmed, color)
	default:
		return applyInlineMarkdown(compactParagraph(trimmed), color)
	}
}

func compactParagraph(text string) string {
	parts := strings.Fields(strings.ReplaceAll(text, "\n", " "))
	return strings.Join(parts, " ")
}

func renderCodeBlock(block string, color bool) string {
	if !color {
		return block
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			lines[i] = logger.Gray(line)
		} else {
			lines[i] = logger.Green(line)
		}
	}
	return strings.Join(lines, "\n")
}

func colorHeading(line string) string {
	trimmed := strings.TrimSpace(line)
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	text := strings.TrimSpace(trimmed[level:])
	return logger.Bold(logger.Cyan(text))
}

func colorQuote(line string, color bool) string {
	trimmed := strings.TrimSpace(line)
	text := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	text = applyInlineMarkdown(text, color)
	if !color {
		return "> " + text
	}
	return logger.Gray(">") + " " + text
}

func colorListLine(line string, color bool) string {
	marker, content := splitListMarker(strings.TrimSpace(line))
	content = applyInlineMarkdown(content, color)
	if !color {
		return marker + content
	}
	return logger.Yellow(marker) + content
}

func splitListMarker(line string) (string, string) {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return line[:2], line[2:]
	}
	for i := 0; i < len(line); i++ {
		if line[i] == '.' && i+1 < len(line) && line[i+1] == ' ' {
			return line[:i+2], line[i+2:]
		}
	}
	return "", line
}

var (
	inlineCodePattern = regexp.MustCompile("`([^`]+)`")
	boldPattern       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	linkPattern       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

func applyInlineMarkdown(text string, color bool) string {
	if !color || text == "" {
		return text
	}
	text = linkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		return logger.Cyan(parts[1]) + logger.Gray(" ("+parts[2]+")")
	})
	text = inlineCodePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := inlineCodePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return logger.Yellow("`" + parts[1] + "`")
	})
	text = boldPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := boldPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return logger.Bold(parts[1])
	})
	return text
}
