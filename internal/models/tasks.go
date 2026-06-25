package models

import (
	"encoding/json"
	"fmt"
)

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

// TaskOutcome is the envelope a completed async task carries in its outcome
// payload. rotki embeds the real HTTP-style result here: a status_code plus a
// result/message pair. A task can complete (status "completed") while the
// underlying operation failed, so callers must inspect status_code rather than
// assume completion means success.
type TaskOutcome struct {
	Result     json.RawMessage `json:"result"`
	Message    string          `json:"message"`
	StatusCode int             `json:"status_code"`
}

// Err reports a non-nil error when the outcome describes a failed operation.
// It treats a non-2xx status_code as a failure, and also a literal
// result==false paired with a message (some endpoints omit status_code but
// still signal failure this way). A zero status_code is treated as "not
// reported" and ignored.
func (o TaskOutcome) Err() error {
	if o.StatusCode != 0 && (o.StatusCode < 200 || o.StatusCode >= 300) {
		msg := o.Message
		if msg == "" {
			msg = "operation failed"
		}
		return fmt.Errorf("async task failed (status %d): %s", o.StatusCode, msg)
	}

	if o.Message != "" && len(o.Result) > 0 {
		var ok bool
		if err := json.Unmarshal(o.Result, &ok); err == nil && !ok {
			return fmt.Errorf("async task reported failure: %s", o.Message)
		}
	}

	return nil
}
