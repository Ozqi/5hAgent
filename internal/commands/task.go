// task.go - /task 命令处理
// 功能：解析 /task 命令（list/create/update/get/delete/archive/reopen），调用 agent.ExecuteTaskAction
// 导出函数：HandleTask, runTaskAction
package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lzq/5hAgent/internal/task"
)

// HandleTask 处理 /task 命令
func HandleTask(cmd string, list *task.TaskList) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return "", fmt.Errorf("usage: /task <list|create|update|get|delete|archive|reopen> [args...]")
	}

	action := parts[1]
	switch action {
	case "list":
		status := ""
		if len(parts) > 2 {
			status = parts[2]
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, Status: status})
	case "create":
		if len(parts) < 5 {
			return "", fmt.Errorf("usage: /task create <id> <title> <description>")
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, ID: parts[2], Title: parts[3], Description: strings.Join(parts[4:], " ")})
	case "update":
		if len(parts) < 4 {
			return "", fmt.Errorf("usage: /task update <id> <status>")
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, ID: parts[2], Status: parts[3]})
	case "get":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /task get <id>")
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, ID: parts[2]})
	case "delete":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /task delete <id>")
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, ID: parts[2]})
	case "archive":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /task archive <id>")
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, ID: parts[2]})
	case "reopen":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /task reopen <id> [status]")
		}
		status := ""
		if len(parts) > 3 {
			status = parts[3]
		}
		return runTaskAction(list, task.TaskActionRequest{Action: action, ID: parts[2], Status: status})
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func runTaskAction(list *task.TaskList, req task.TaskActionRequest) (string, error) {
	result, err := task.ExecuteTaskAction(list, req)
	if err != nil {
		return "", err
	}

	if req.Action == "list" {
		if len(result.Tasks) == 0 {
			return fmt.Sprintf("No tasks found in %s", result.Source), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Tasks (%s):\n", result.Source))
		for _, t := range result.Tasks {
			sb.WriteString(fmt.Sprintf("  [%s] %s - %s (%s)\n", t.ID, t.Title, t.Status, t.CreatedAt.Format("2006-01-02")))
		}
		p := result.Progress
		sb.WriteString(fmt.Sprintf("\nProgress: %d total, %d pending, %d in_progress, %d blocked, %d completed, %d archived\n", p.Total, p.Pending, p.InProgress, p.Blocked, p.Completed, p.Archived))
		return sb.String(), nil
	}

	var payload interface{} = result.Task
	if result.Task == nil {
		payload = result
	}
	encoded, _ := json.MarshalIndent(payload, "", "  ")
	if result.Message != "" {
		return fmt.Sprintf("%s:\n%s", result.Message, string(encoded)), nil
	}
	return string(encoded), nil
}
