package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/lzq/5hAgent/internal/mcp"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/task"
)

type fakeMCPClient struct {
	toolName  string
	arguments string
}

func (c *fakeMCPClient) CallTool(ctx context.Context, toolName string, arguments string) (string, error) {
	c.toolName = toolName
	c.arguments = arguments
	return `{"ok":true}`, nil
}

func TestRegistryExposesMCPToolSkillToolAndTaskTool(t *testing.T) {
	ctx := context.Background()

	mcpClient := &fakeMCPClient{}
	mcpTool := NewMCPTool("demo", mcpClient, mcp.ToolSpec{
		Name:        "lookup",
		Description: "lookup data",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"required":["query"],
			"properties":{"query":{"type":"string"}}
		}`),
	})
	assertToolName(t, mcpTool, "mcp.demo.lookup")
	mcpResult, err := mcpTool.InvokableRun(ctx, `{"query":"agent"}`)
	if err != nil {
		t.Fatalf("mcp tool invoke failed: %v", err)
	}
	if mcpResult != `{"ok":true}` || mcpClient.toolName != "lookup" || mcpClient.arguments != `{"query":"agent"}` {
		t.Fatalf("mcp tool did not proxy call correctly: result=%s client=%#v", mcpResult, mcpClient)
	}

	skillDir := filepath.Join(t.TempDir(), "skills")
	writeSkill(t, skillDir, "debugging", `---
name: debugging
description: Debugging workflow
---
# Debugging

Use systematic checks.
`)
	skillMgr := skill.NewManager(skillDir)
	if err := skillMgr.LoadSkills(); err != nil {
		t.Fatalf("load skills failed: %v", err)
	}
	skillTool := &SkillTool{mgr: skillMgr}
	assertToolName(t, skillTool, "skill.skill")
	skillResult, err := skillTool.InvokableRun(ctx, `{"action":"get","skill":"debugging"}`)
	if err != nil {
		t.Fatalf("skill tool get failed: %v", err)
	}
	if !strings.Contains(skillResult, "# Skill: debugging") || !strings.Contains(skillResult, "Use systematic checks.") {
		t.Fatalf("skill result = %q, want skill content", skillResult)
	}

	taskList, err := task.NewTaskList(filepath.Join(t.TempDir(), "task.md"))
	if err != nil {
		t.Fatalf("create task list failed: %v", err)
	}
	taskTool := &TaskTool{taskList: taskList}
	assertToolName(t, taskTool, "task.task")
	if _, err := taskTool.InvokableRun(ctx, `{"action":"create","id":"read-code","title":"Read code","description":"Study implementation"}`); err != nil {
		t.Fatalf("task create failed: %v", err)
	}
	if _, err := taskTool.InvokableRun(ctx, `{"action":"update","id":"read-code","status":"completed"}`); err != nil {
		t.Fatalf("task update failed: %v", err)
	}
	createdTask, err := taskList.GetTask("read-code")
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if createdTask.Status != task.StatusCompleted {
		t.Fatalf("task status mismatch: %s", createdTask.Status)
	}
	if err := InitRegistry(taskList, skillMgr); err != nil {
		t.Fatalf("init registry failed: %v", err)
	}
	if got := GetToolByName("task"); got == nil {
		t.Fatalf("task tool was not exposed by registry")
	}
	if got := GetToolByName("skill"); got == nil {
		t.Fatalf("skill tool was not exposed by registry")
	}
	if got := GetToolByName("sys.session"); got != nil {
		t.Fatalf("sys.session should not be exposed by registry")
	}
	if got := GetToolByName("sys.ipc"); got == nil {
		t.Fatalf("sys.ipc tool was not exposed by registry")
	}
	if err := RegisterMCPTools("demo", mcpClient, []mcp.ToolSpec{{
		Name:        "lookup",
		Description: "lookup data",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}); err != nil {
		t.Fatalf("register mcp tools failed: %v", err)
	}
	if got := GetToolByName("mcp.demo.lookup"); got == nil {
		t.Fatalf("mcp tool was not exposed by registry")
	}
}

func assertToolName(t *testing.T, candidate tool.BaseTool, want string) {
	t.Helper()
	info, err := candidate.Info(context.Background())
	if err != nil {
		t.Fatalf("tool info failed: %v", err)
	}
	if info.Name != want {
		t.Fatalf("tool name = %q, want %q", info.Name, want)
	}
}

func writeSkill(t *testing.T, root string, name string, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create skill dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill failed: %v", err)
	}
}
