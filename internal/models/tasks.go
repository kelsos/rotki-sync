package models

import "encoding/json"

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusNotFound  TaskStatus = "not-found"
)

type TaskID int

type TasksResponse struct {
	Pending   []TaskID `json:"pending"`
	Completed []TaskID `json:"completed"`
}

type TaskResult struct {
	Status  TaskStatus      `json:"status"`
	Outcome json.RawMessage `json:"outcome"`
}

type AsyncTaskResponse struct {
	TaskID TaskID `json:"task_id"`
}
