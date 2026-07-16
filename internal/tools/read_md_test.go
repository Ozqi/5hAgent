package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestReadMDListsHeadingsAndReadsSection(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	content := "# Intro\n\nstart\n\n## Install\n\nstep 1\n\n```go\n# not heading\n```\n\n### Detail\n\nmore\n\n## Usage\n\nrun\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	candidate, err := NewReadMDTool(root)
	if err != nil {
		t.Fatalf("NewReadMDTool() error = %v", err)
	}

	listResult, err := candidate.InvokableRun(context.Background(), &schema.ToolArgument{Text: `{"action":"list_headings","path":"guide.md"}`})
	if err != nil {
		t.Fatalf("list headings error = %v", err)
	}
	var listOutput ReadMDOutput
	if err := json.Unmarshal([]byte(listResult.Parts[0].Text), &listOutput); err != nil {
		t.Fatalf("Unmarshal list error = %v", err)
	}
	if len(listOutput.Headings) != 4 {
		t.Fatalf("headings = %+v, want 4 headings excluding fenced heading", listOutput.Headings)
	}
	if listOutput.Headings[1].Title != "Install" || listOutput.Headings[1].Level != 2 {
		t.Fatalf("second heading = %+v, want Install h2", listOutput.Headings[1])
	}

	sectionResult, err := candidate.InvokableRun(context.Background(), &schema.ToolArgument{Text: `{"action":"read_section","path":"guide.md","heading":"Install"}`})
	if err != nil {
		t.Fatalf("read section error = %v", err)
	}
	var sectionOutput ReadMDOutput
	if err := json.Unmarshal([]byte(sectionResult.Parts[0].Text), &sectionOutput); err != nil {
		t.Fatalf("Unmarshal section error = %v", err)
	}
	if sectionOutput.Heading != "Install" {
		t.Fatalf("heading = %q, want Install", sectionOutput.Heading)
	}
	for _, want := range []string{"## Install", "step 1", "### Detail", "more"} {
		if !strings.Contains(sectionOutput.Content, want) {
			t.Fatalf("section content = %q, want %q", sectionOutput.Content, want)
		}
	}
	if strings.Contains(sectionOutput.Content, "## Usage") {
		t.Fatalf("section content = %q, should stop before Usage", sectionOutput.Content)
	}
}
