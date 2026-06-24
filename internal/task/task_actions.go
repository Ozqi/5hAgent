// task_actions.go - 任务操作执行器
// 功能：根据 action 参数分发到 TaskList 的具体操作（create/update/get/list/delete/archive/reopen）
// 主要类型：TaskActionRequest, TaskActionResult, TaskProgress
// 导出函数：ExecuteTaskAction
package task

import (
	"fmt"
	"path/filepath"
	"strings"
)

type TaskActionRequest struct {
	Action      string
	ID          string
	Title       string
	Description string
	Status      string
}

type TaskProgress struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Blocked    int `json:"blocked"`
	Completed  int `json:"completed"`
	Archived   int `json:"archived"`
	Failed     int `json:"failed"`
}

type TaskActionResult struct {
	Source   string       `json:"source,omitempty"`
	Path     string       `json:"path,omitempty"`
	Task     *Task        `json:"task,omitempty"`
	Tasks    []*Task      `json:"tasks,omitempty"`
	Progress TaskProgress `json:"progress,omitempty"`
	Message  string       `json:"message,omitempty"`
}

func ExecuteTaskAction(list *TaskList, req TaskActionRequest) (*TaskActionResult, error) {
	if list == nil {
		return nil, fmt.Errorf("task list is required")
	}

	result := &TaskActionResult{
		Source: filepath.Base(list.Path()),
		Path:   list.Path(),
	}

	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	switch req.Action {
	case "create":
		if req.ID == "" || req.Title == "" || req.Description == "" {
			return nil, fmt.Errorf("id, title, and description are required")
		}
		task, err := list.CreateTask(req.ID, req.Title, req.Description)
		if err != nil {
			return nil, err
		}
		result.Task = task
		result.Message = fmt.Sprintf("task %q created", task.ID)
		return result, nil
	case "update":
		if req.ID == "" || req.Status == "" {
			return nil, fmt.Errorf("id and status are required")
		}
		status, err := ParseTaskStatus(req.Status)
		if err != nil {
			return nil, err
		}
		if err := list.UpdateTaskStatus(req.ID, status); err != nil {
			return nil, err
		}
		task, err := list.GetTask(req.ID)
		if err != nil {
			return nil, err
		}
		result.Task = task
		result.Message = fmt.Sprintf("task %q updated", task.ID)
		return result, nil
	case "get":
		if req.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		task, err := list.GetTask(req.ID)
		if err != nil {
			return nil, err
		}
		result.Task = task
		return result, nil
	case "list":
		if req.Status != "" {
			status, err := ParseTaskStatus(req.Status)
			if err != nil {
				return nil, err
			}
			result.Tasks = list.ListTasksByStatus(status)
		} else {
			result.Tasks = list.ListTasks()
		}
		total, pending, inProgress, blocked, completed, archived, failed := list.GetProgress()
		result.Progress = TaskProgress{Total: total, Pending: pending, InProgress: inProgress, Blocked: blocked, Completed: completed, Archived: archived, Failed: failed}
		if len(result.Tasks) > 0 {
			result.Task = result.Tasks[0]
		}
		return result, nil
	case "delete":
		if req.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		if err := list.DeleteTask(req.ID); err != nil {
			return nil, err
		}
		result.Message = fmt.Sprintf("task %q deleted", req.ID)
		return result, nil
	case "archive":
		if req.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		task, err := list.ArchiveTask(req.ID)
		if err != nil {
			return nil, err
		}
		result.Task = task
		result.Message = fmt.Sprintf("task %q archived", task.ID)
		return result, nil
	case "reopen":
		if req.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		status := StatusPending
		if req.Status != "" {
			parsed, err := ParseTaskStatus(req.Status)
			if err != nil {
				return nil, err
			}
			if parsed != StatusPending && parsed != StatusInProgress {
				return nil, fmt.Errorf("reopen status must be pending or in_progress")
			}
			status = parsed
		}
		task, err := list.ReopenTask(req.ID, status)
		if err != nil {
			return nil, err
		}
		result.Task = task
		result.Message = fmt.Sprintf("task %q reopened", task.ID)
		return result, nil
	default:
		if req.Action == "" {
			return nil, fmt.Errorf("missing action: expected one of create/update/get/list/delete/archive/reopen")
		}
		return nil, fmt.Errorf("unknown action %q: expected one of create/update/get/list/delete/archive/reopen", req.Action)
	}
}
