package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lzq/5hAgent/internal/agent"
)

// HandleTask 处理 /task 命令
func HandleTask(cmd string, list *agent.TaskList) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return "", fmt.Errorf("usage: /task <list|create|update|get|delete> [args...]")
	}

	action := parts[1]
	switch action {
	case "list":
		status := ""
		if len(parts) > 2 {
			status = parts[2]
		}
		return listTasks(list, status)
	case "create":
		if len(parts) < 5 {
			return "", fmt.Errorf("usage: /task create <id> <title> <description>")
		}
		return createTask(list, parts[2], parts[3], strings.Join(parts[4:], " "))
	case "update":
		if len(parts) < 4 {
			return "", fmt.Errorf("usage: /task update <id> <status>")
		}
		return updateTask(list, parts[2], parts[3])
	case "get":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /task get <id>")
		}
		return getTask(list, parts[2])
	case "delete":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /task delete <id>")
		}
		return deleteTask(list, parts[2])
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func listTasks(list *agent.TaskList, status string) (string, error) {
	var tasks []*agent.Task

	if status != "" {
		tasks = list.ListTasksByStatus(agent.TaskStatus(status))
	} else {
		tasks = list.ListTasks()
	}

	if len(tasks) == 0 {
		return "No tasks found", nil
	}

	var sb strings.Builder
	sb.WriteString("Tasks:\n")
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("  [%s] %s - %s (%s)\n", t.ID, t.Title, t.Status, t.CreatedAt.Format("2006-01-02")))
	}

	total, pending, inProgress, completed, failed := list.GetProgress()
	sb.WriteString(fmt.Sprintf("\nProgress: %d total, %d pending, %d in_progress, %d completed, %d failed\n",
		total, pending, inProgress, completed, failed))

	return sb.String(), nil
}

func createTask(list *agent.TaskList, id, title, desc string) (string, error) {
	task, err := list.CreateTask(id, title, desc)
	if err != nil {
		return "", err
	}

	result, _ := json.MarshalIndent(task, "", "  ")
	return fmt.Sprintf("Task created:\n%s", string(result)), nil
}

func updateTask(list *agent.TaskList, id, status string) (string, error) {
	if err := list.UpdateTaskStatus(id, agent.TaskStatus(status)); err != nil {
		return "", err
	}

	task, _ := list.GetTask(id)
	return fmt.Sprintf("Task '%s' updated to status: %s", task.ID, task.Status), nil
}

func getTask(list *agent.TaskList, id string) (string, error) {
	task, err := list.GetTask(id)
	if err != nil {
		return "", err
	}

	result, _ := json.MarshalIndent(task, "", "  ")
	return string(result), nil
}

func deleteTask(list *agent.TaskList, id string) (string, error) {
	if err := list.DeleteTask(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Task '%s' deleted", id), nil
}
