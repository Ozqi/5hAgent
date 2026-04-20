package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// GrepInput defines the input parameters for grep tool
type GrepInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Search pattern (supports regex)"`
	Path    string `json:"path,omitempty" jsonschema:"description=Directory or file to search in (default: current directory)"`
	Type    string `json:"type,omitempty" jsonschema:"description=File type filter (e.g. 'go', 'py', 'js')"`
}

// GrepMatch represents a single match result
type GrepMatch struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

// GrepOutput defines the output structure for grep tool
type GrepOutput struct {
	Matches []GrepMatch `json:"matches"`
	Count   int         `json:"count"`
}

// NewGrepTool creates a new grep tool for code searching
func NewGrepTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"grep",
		"Search for patterns in files using ripgrep. Supports regex patterns and file type filtering. Returns matching lines with file path, line number, and content.",
		func(ctx context.Context, input GrepInput) (*schema.ToolResult, error) {
			// Check if ripgrep is available
			if _, err := exec.LookPath("rg"); err != nil {
				// Fallback to grep if ripgrep not available
				return grepFallback(ctx, input)
			}

			// Build ripgrep command
			args := []string{
				"--json",           // JSON output
				"--no-heading",     // Don't group by file
				"--line-number",    // Show line numbers
				"--column",         // Show column numbers
				"--smart-case",     // Smart case matching
				"--max-count=100", // Limit matches per file
			}

			// Add type filter if specified
			if input.Type != "" {
				args = append(args, "--type", input.Type)
			}

			// Add pattern
			args = append(args, input.Pattern)

			// Add path
			if input.Path != "" {
				args = append(args, input.Path)
			} else {
				args = append(args, ".")
			}

			// Execute ripgrep
			cmd := exec.CommandContext(ctx, "rg", args...)
			output, err := cmd.CombinedOutput()

			// ripgrep returns exit code 1 when no matches found
			if err != nil && len(output) == 0 {
				return &schema.ToolResult{
					Parts: []schema.ToolOutputPart{
						{
							Type: schema.ToolPartTypeText,
							Text: `{"matches":[],"count":0}`,
						},
					},
				}, nil
			}

			// Parse JSON output
			matches := parseRipgrepJSON(string(output))

			// Build output
			result := GrepOutput{
				Matches: matches,
				Count:   len(matches),
			}

			// Convert to JSON
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal output: %w", err)
			}

			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{
						Type: schema.ToolPartTypeText,
						Text: string(resultJSON),
					},
				},
			}, nil
		},
	)
}

// parseRipgrepJSON parses ripgrep's JSON output
func parseRipgrepJSON(output string) []GrepMatch {
	var matches []GrepMatch

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Only process "match" type entries
		if entry["type"] != "match" {
			continue
		}

		data, ok := entry["data"].(map[string]interface{})
		if !ok {
			continue
		}

		path, _ := data["path"].(map[string]interface{})
		pathText, _ := path["text"].(string)

		lineNum, _ := data["line_number"].(float64)

		submatches, _ := data["submatches"].([]interface{})
		if len(submatches) == 0 {
			continue
		}

		submatch, _ := submatches[0].(map[string]interface{})
		matchText, _ := submatch["match"].(map[string]interface{})
		text, _ := matchText["text"].(string)

		start, _ := submatch["start"].(float64)

		matches = append(matches, GrepMatch{
			File:   pathText,
			Line:   int(lineNum),
			Column: int(start) + 1,
			Text:   text,
		})
	}

	return matches
}

// grepFallback uses standard grep when ripgrep is not available
func grepFallback(ctx context.Context, input GrepInput) (*schema.ToolResult, error) {
	path := input.Path
	if path == "" {
		path = "."
	}

	// Build grep command
	args := []string{
		"-r",           // Recursive
		"-n",           // Line numbers
		"-H",           // Show filename
		"--max-count=100", // Limit matches
		input.Pattern,
		path,
	}

	cmd := exec.CommandContext(ctx, "grep", args...)
	output, err := cmd.CombinedOutput()

	// grep returns exit code 1 when no matches found
	if err != nil && len(output) == 0 {
		return &schema.ToolResult{
			Parts: []schema.ToolOutputPart{
				{
					Type: schema.ToolPartTypeText,
					Text: `{"matches":[],"count":0}`,
				},
			},
		}, nil
	}

	// Parse grep output (format: file:line:text)
	var matches []GrepMatch
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}

		var lineNum int
		fmt.Sscanf(parts[1], "%d", &lineNum)

		matches = append(matches, GrepMatch{
			File:   parts[0],
			Line:   lineNum,
			Column: 0,
			Text:   parts[2],
		})
	}

	result := GrepOutput{
		Matches: matches,
		Count:   len(matches),
	}

	resultJSON, _ := json.Marshal(result)

	return &schema.ToolResult{
		Parts: []schema.ToolOutputPart{
			{
				Type: schema.ToolPartTypeText,
				Text: string(resultJSON),
			},
		},
	}, nil
}
