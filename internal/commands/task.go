// task.go - /task 命令处理
// 功能：解析 /task 命令（list/create/update/get/delete/archive/reopen），调用 agent.ExecuteTaskAction
// 导出函数：HandleTask, runTaskAction
package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lzq/5hAgent/internal/task"
	"github.com/mattn/go-runewidth"
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
		sb.WriteString("  " + taskListRow("ID", "Title", "Status", "Created") + "\n")
		sb.WriteString("  " + taskListRow(strings.Repeat("-", 34), strings.Repeat("-", 28), strings.Repeat("-", 12), strings.Repeat("-", 10)) + "\n")
		for _, t := range result.Tasks {
			sb.WriteString("  " + taskListRow(t.ID, t.Title, string(t.Status), t.CreatedAt.Format("2006-01-02")) + "\n")
		}
		p := result.Progress
		sb.WriteString(fmt.Sprintf("\nProgress: %d total, %d pending, %d in_progress, %d blocked, %d completed, %d archived, %d failed\n", p.Total, p.Pending, p.InProgress, p.Blocked, p.Completed, p.Archived, p.Failed))
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

func taskListRow(id string, title string, status string, created string) string {
	return padDisplayCell(id, 34) + "  " + padDisplayCell(title, 28) + "  " + padDisplayCell(status, 12) + "  " + padDisplayCell(created, 10)
}

func padDisplayCell(text string, width int) string {
	text = trimCell(text, width)
	padding := width - runewidth.StringWidth(text)
	if padding > 0 {
		text += strings.Repeat(" ", padding)
	}
	return text
}

func trimCell(text string, maxLen int) string {
	runes := []rune(strings.TrimSpace(text))
	var b strings.Builder
	for _, r := range runes {
		next := b.String() + string(r)
		if runewidth.StringWidth(next) > maxLen {
			break
		}
		b.WriteRune(r)
	}
	result := b.String()
	if result == string(runes) {
		return result
	}
	for runewidth.StringWidth(result+"…") > maxLen && result != "" {
		rs := []rune(result)
		result = string(rs[:len(rs)-1])
	}
	return result + "…"
}
