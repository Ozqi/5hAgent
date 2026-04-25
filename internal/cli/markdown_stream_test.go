package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderMarkdownCompactsParagraphs(t *testing.T) {
	got := renderMarkdownForTerminal("Hello\nworld\n\nNext para\nline two", true)

	if !strings.Contains(got, "Hello world") {
		t.Fatalf("expected compact paragraph, got %q", got)
	}
	if strings.Contains(got, "Hello\nworld") {
		t.Fatalf("expected single newline inside paragraph to collapse, got %q", got)
	}
	if !strings.Contains(got, "Next para line two") {
		t.Fatalf("expected second paragraph to be compacted, got %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Fatalf("expected blank lines to collapse to a single newline, got %q", got)
	}
}

func TestRenderMarkdownStylesMarkdown(t *testing.T) {
	got := renderMarkdownForTerminal("# Title\n\n- item with `code`\n\n```go\nfmt.Println(1)\n```", true)

	if !strings.Contains(got, "Title") || !strings.Contains(got, "item with") || !strings.Contains(got, "fmt.Println(1)") {
		t.Fatalf("expected markdown content to remain visible, got %q", got)
	}
	if !strings.Contains(got, "\033[") {
		t.Fatalf("expected ansi colors in rendered markdown, got %q", got)
	}
}

func TestMarkdownStreamRendererBuffersParagraphUntilBoundary(t *testing.T) {
	var out bytes.Buffer
	r := NewMarkdownStreamRenderer(&out, "Assistant: ")

	r.WriteToken("Hello\n")
	r.WriteToken("world")
	if out.Len() != 0 {
		t.Fatalf("expected no output before paragraph boundary, got %q", out.String())
	}

	r.WriteToken("\n\n")
	got := out.String()
	if !strings.Contains(got, "Assistant: Hello world") {
		t.Fatalf("expected buffered paragraph to flush with prefix, got %q", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected flushed output not to preserve double newline, got %q", got)
	}
}

func TestMarkdownStreamRendererFlushesRemainingContent(t *testing.T) {
	var out bytes.Buffer
	r := NewMarkdownStreamRenderer(&out, "Assistant: ")

	r.WriteToken("# Title")
	r.Flush()

	got := out.String()
	if !strings.Contains(got, "Assistant: ") || !strings.Contains(got, "Title") {
		t.Fatalf("expected flush to emit remaining content, got %q", got)
	}
}
