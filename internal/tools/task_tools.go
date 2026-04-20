package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/miniAgent/internal/agent"
)

// TaskCreateInput defines the input for task_create tool
type TaskCreateInput struct {
	ID          string `json:"id" jsonschema:"required,description=Unique task ID"`
	Title       string `json:"title" jsonschema:"required,description=Task title"`
	Description string `json:"description" jsonschema:"required,description=Task description"`
}

// TaskUpdateInput defines the input for task_update tool
type TaskUpdateInput struct {
	ID     string `json:"id" jsonschema:"required,description=Task ID to update"`
	Status string `json:"status" jsonschema:"required,description=New status: pending, in_progress, completed, failed"`
}

// TaskGetInput defines the input for task_get tool
type TaskGetInput struct {
	ID string `json:"id" jsonschema:"required,description=Task ID to retrieve"`
}

// TaskListInput defines the input for task_list tool
type TaskListInput struct {
	Status string `json:"status,omitempty" jsonschema:"description=Filter by status: pending, in_progress, completed, failed (optional)"`
}

// TaskDeleteInput defines the input for task_delete tool
type TaskDeleteInput struct {
	ID string `json:"id" jsonschema:"required,description=Task ID to delete"`
}

// Global task list instance
var globalTaskList *agent.TaskList

// InitTaskList initializes the global task list
func InitTaskList(filePath string) error {
	tl, err := agent.NewTaskList(filePath)
	if err != nil {
		return err
	}
	globalTaskList = tl
	return nil
}

// NewTaskCreateTool creates a tool for creating tasks
func NewTaskCreateTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"task_create",
		"Create a new task in the task list. Returns the created task with ID, title, description, and status.",
		func(ctx context.Context, input TaskCreateInput) (*schema.ToolResult, error) {
			if globalTaskList == nil {
				return nil, fmt.Errorf("task list not initialized")
			}

			task, err := globalTaskList.CreateTask(input.ID, input.Title, input.Description)
			if err != nil {
				return nil, err
			}

			resultJSON, _ := json.Marshal(task)
			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{Type: schema.ToolPartTypeText, Text: string(resultJSON)},
				},
			}, nil
		},
	)
}

// NewTaskUpdateTool creates a tool for updating task status
func NewTaskUpdateTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"task_update",
		"Update a task's status. Valid statuses: pending, in_progress, completed, failed.",
		func(ctx context.Context, input TaskUpdateInput) (*schema.ToolResult, error) {
			if globalTaskList == nil {
				return nil, fmt.Errorf("task list not initialized")
			}

			status := agent.TaskStatus(input.Status)
			if err := globalTaskList.UpdateTaskStatus(input.ID, status); err != nil {
				return nil, err
			}

			task, _ := globalTaskList.GetTask(input.ID)
			resultJSON, _ := json.Marshal(task)
			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{Type: schema.ToolPartTypeText, Text: string(resultJSON)},
				},
			}, nil
		},
	)
}

// NewTaskGetTool creates a tool for getting a task
func NewTaskGetTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"task_get",
		"Get details of a specific task by ID. Returns task information including status, timestamps, and description.",
		func(ctx context.Context, input TaskGetInput) (*schema.ToolResult, error) {
			if globalTaskList == nil {
				return nil, fmt.Errorf("task list not initialized")
			}

			task, err := globalTaskList.GetTask(input.ID)
			if err != nil {
				return nil, err
			}

			resultJSON, _ := json.Marshal(task)
			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{Type: schema.ToolPartTypeText, Text: string(resultJSON)},
				},
			}, nil
		},
	)
}

// NewTaskListTool creates a tool for listing tasks
func NewTaskListTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"task_list",
		"List all tasks or filter by status. Returns array of tasks with their details and progress summary.",
		func(ctx context.Context, input TaskListInput) (*schema.ToolResult, error) {
			if globalTaskList == nil {
				return nil, fmt.Errorf("task list not initialized")
			}

			var tasks []*agent.Task
			if input.Status != "" {
				tasks = globalTaskList.ListTasksByStatus(agent.TaskStatus(input.Status))
			} else {
				tasks = globalTaskList.ListTasks()
			}

			// Get progress
			total, pending, inProgress, completed, failed := globalTaskList.GetProgress()

			result := map[string]interface{}{
				"tasks": tasks,
				"progress": map[string]int{
					"total":       total,
					"pending":     pending,
					"in_progress": inProgress,
					"completed":   completed,
					"failed":      failed,
				},
			}

			resultJSON, _ := json.Marshal(result)
			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{Type: schema.ToolPartTypeText, Text: string(resultJSON)},
				},
			}, nil
		},
	)
}

// NewTaskDeleteTool creates a tool for deleting tasks
func NewTaskDeleteTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"task_delete",
		"Delete a task from the task list by ID. This action cannot be undone.",
		func(ctx context.Context, input TaskDeleteInput) (*schema.ToolResult, error) {
			if globalTaskList == nil {
				return nil, fmt.Errorf("task list not initialized")
			}

			if err := globalTaskList.DeleteTask(input.ID); err != nil {
				return nil, err
			}

			result := map[string]interface{}{
				"success": true,
				"message": fmt.Sprintf("Task %s deleted successfully", input.ID),
			}

			resultJSON, _ := json.Marshal(result)
			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{Type: schema.ToolPartTypeText, Text: string(resultJSON)},
				},
			}, nil
		},
	)
}
