package async

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
)

type TaskManager struct {
	client        *client.APIClient
	activeTasks   map[models.TaskID]chan<- models.APIResponse[json.RawMessage]
	mu            sync.RWMutex
	pollInterval  time.Duration
	stopPolling   chan struct{}
	pollingActive bool
}

func NewTaskManager(apiClient *client.APIClient) *TaskManager {
	return &TaskManager{
		client:       apiClient,
		activeTasks:  make(map[models.TaskID]chan<- models.APIResponse[json.RawMessage]),
		pollInterval: time.Second,
		stopPolling:  make(chan struct{}),
	}
}

func (tm *TaskManager) RegisterTask(taskID models.TaskID) <-chan models.APIResponse[json.RawMessage] {
	resultChan := make(chan models.APIResponse[json.RawMessage], 1)

	tm.mu.Lock()
	tm.activeTasks[taskID] = resultChan

	if !tm.pollingActive {
		tm.pollingActive = true
		// Recreate stopPolling channel if it was closed from previous stop
		tm.stopPolling = make(chan struct{})
		go tm.pollTasks()
	}
	tm.mu.Unlock()

	logger.Debug("Registered task %d for monitoring", taskID)
	return resultChan
}

func (tm *TaskManager) pollTasks() {
	ticker := time.NewTicker(tm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopPolling:
			tm.mu.Lock()
			tm.pollingActive = false
			tm.mu.Unlock()
			return
		case <-ticker.C:
			tm.checkTasks()
		}
	}
}

func (tm *TaskManager) checkTasks() {
	tm.mu.RLock()
	if len(tm.activeTasks) == 0 {
		tm.mu.RUnlock()
		tm.Stop()
		return
	}
	tm.mu.RUnlock()

	var tasksResponse models.APIResponse[models.TasksResponse]
	if err := tm.client.Get("/tasks", &tasksResponse); err != nil {
		logger.Error("Failed to fetch tasks status: %v", err)
		return
	}

	for _, completedTaskID := range tasksResponse.Result.Completed {
		tm.mu.RLock()
		resultChan, exists := tm.activeTasks[completedTaskID]
		tm.mu.RUnlock()

		if exists {
			tm.fetchTaskResult(completedTaskID, resultChan)
		}
	}
}

func (tm *TaskManager) fetchTaskResult(taskID models.TaskID, resultChan chan<- models.APIResponse[json.RawMessage]) {
	endpoint := fmt.Sprintf("/tasks/%d", taskID)
	var taskResult models.APIResponse[models.TaskResult]

	if err := tm.client.Get(endpoint, &taskResult); err != nil {
		logger.Error("Failed to fetch result for task %d: %v", taskID, err)
		resultChan <- models.APIResponse[json.RawMessage]{
			Message: fmt.Sprintf("Failed to fetch task result: %v", err),
		}
	} else {
		if taskResult.Result.Status == models.TaskStatusNotFound {
			logger.Error("Task %d not found", taskID)
			resultChan <- models.APIResponse[json.RawMessage]{
				Message: fmt.Sprintf("Task %d not found", taskID),
			}
		} else {
			resultChan <- models.APIResponse[json.RawMessage]{
				Result:  taskResult.Result.Outcome,
				Message: taskResult.Message,
			}
		}
	}

	close(resultChan)

	tm.mu.Lock()
	delete(tm.activeTasks, taskID)
	tm.mu.Unlock()

	logger.Debug("Task %d completed and removed from monitoring", taskID)
}

func (tm *TaskManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.pollingActive {
		close(tm.stopPolling)
		tm.pollingActive = false
	}
}

// prepareAsyncEndpoint adds async_query=true parameter to GET endpoints
func prepareAsyncEndpoint(endpoint string) string {
	asyncEndpoint := endpoint
	if endpoint != "" && endpoint[len(endpoint)-1] != '?' {
		asyncEndpoint += "?"
	}
	asyncEndpoint += "async_query=true"
	return asyncEndpoint
}

// prepareRequestBody converts body to map and adds async_query=true
func prepareRequestBody(body interface{}) (map[string]interface{}, error) {
	var requestBody map[string]interface{}
	if body != nil {
		bodyBytes, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", marshalErr)
		}
		if unmarshalErr := json.Unmarshal(bodyBytes, &requestBody); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to unmarshal request body: %w", unmarshalErr)
		}
	} else {
		requestBody = make(map[string]interface{})
	}
	requestBody["async_query"] = true
	return requestBody, nil
}

// executeHTTPRequest performs the actual HTTP request based on method
func executeHTTPRequest(tm *TaskManager, method, endpoint string, requestBody map[string]interface{}) (*models.APIResponse[models.AsyncTaskResponse], error) {
	var asyncResponse models.APIResponse[models.AsyncTaskResponse]
	var err error

	switch method {
	case "POST":
		err = tm.client.Post(endpoint, requestBody, &asyncResponse)
	case "PUT":
		err = tm.client.Put(endpoint, requestBody, &asyncResponse)
	case "PATCH":
		err = tm.client.Patch(endpoint, requestBody, &asyncResponse)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}

	return &asyncResponse, err
}

// waitForTaskResult waits for async task completion and unmarshals result
func waitForTaskResult[T any](tm *TaskManager, taskID models.TaskID) (*models.APIResponse[T], error) {
	resultChan := tm.RegisterTask(taskID)
	rawResult := <-resultChan

	var finalResponse models.APIResponse[T]
	if err := json.Unmarshal(rawResult.Result, &finalResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task result: %w", err)
	}

	return &finalResponse, nil
}

func ExecuteAsync[T any](
	tm *TaskManager,
	method string,
	endpoint string,
	body interface{},
) (*models.APIResponse[T], error) {
	var asyncResponse *models.APIResponse[models.AsyncTaskResponse]
	var err error

	switch method {
	case "GET":
		asyncEndpoint := prepareAsyncEndpoint(endpoint)
		var response models.APIResponse[models.AsyncTaskResponse]
		err = tm.client.Get(asyncEndpoint, &response)
		asyncResponse = &response
	case "POST", "PUT", "PATCH":
		requestBody, prepErr := prepareRequestBody(body)
		if prepErr != nil {
			return nil, prepErr
		}
		asyncResponse, err = executeHTTPRequest(tm, method, endpoint, requestBody)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initiate async request: %w", err)
	}

	return waitForTaskResult[T](tm, asyncResponse.Result.TaskID)
}
