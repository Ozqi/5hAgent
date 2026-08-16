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

func TestReadMDReplacesAndDeletesSection(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	content := "# Intro\n\nstart\n\n## Install\n\nold\n\n### Detail\n\nold detail\n\n## Usage\n\nrun\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	candidate, err := NewReadMDTool(root)
	if err != nil {
		t.Fatal(err)
	}

	replace := `{"action":"replace_section","path":"guide.md","heading":"Install","content":"## Install\n\nnew\n"}`
	if _, err := candidate.InvokableRun(context.Background(), &schema.ToolArgument{Text: replace}); err != nil {
		t.Fatalf("replace section error = %v", err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "## Install\n\nnew\n\n## Usage") || strings.Contains(string(updated), "old detail") {
		t.Fatalf("updated Markdown = %q", updated)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode after replace = %v, want 0640", info.Mode().Perm())
	}

	deleteInput := `{"action":"delete_section","path":"guide.md","heading":"Install"}`
	if _, err := candidate.InvokableRun(context.Background(), &schema.ToolArgument{Text: deleteInput}); err != nil {
		t.Fatalf("delete section error = %v", err)
	}
	updated, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), "## Install") || !strings.Contains(string(updated), "## Usage") {
		t.Fatalf("Markdown after delete = %q", updated)
	}
}
