package cli

import (
	"strings"
	"testing"
)

func TestRenderMarkdownPreservesListIndent(t *testing.T) {
	input := "- one\n  - child\n  1. ordered"
	rendered := stripANSI(renderMarkdownForTerminal(input, true))
	lines := strings.Split(rendered, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %#v, want 3 list lines", lines)
	}
	if lines[0] != "- one" {
		t.Fatalf("line 0 = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  - child") {
		t.Fatalf("line 1 = %q, want nested bullet indentation", lines[1])
	}
	if !strings.HasPrefix(lines[2], "  1. ordered") {
		t.Fatalf("line 2 = %q, want nested ordered indentation", lines[2])
	}
}

func TestRenderMarkdownPreservesQuoteMarkers(t *testing.T) {
	input := "> first\n> > nested"
	rendered := stripANSI(renderMarkdownForTerminal(input, true))
	lines := strings.Split(rendered, "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %#v, want 2 quote lines", lines)
	}
	if lines[0] != "> first" {
		t.Fatalf("line 0 = %q", lines[0])
	}
	if lines[1] != "> > nested" {
		t.Fatalf("line 1 = %q", lines[1])
	}
}

func TestRenderMarkdownTableKeepsPipeShape(t *testing.T) {
	input := "| Name | Value |\n| --- | --- |\n| a | 1 |\n| longer | 20 |"
	rendered := stripANSI(renderMarkdownForTerminal(input, true))
	lines := strings.Split(rendered, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %#v, want header separator and rows", lines)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			t.Fatalf("table line = %q, want markdown pipe shape", line)
		}
	}
	if !strings.Contains(lines[1], "---") {
		t.Fatalf("separator = %q, want markdown separator", lines[1])
	}
}
