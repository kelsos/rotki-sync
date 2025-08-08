package async

import (
	"github.com/kelsos/rotki-sync/internal/models"
)

// Client provides a clean API for async operations
type Client struct {
	manager *TaskManager
}

// NewClient creates a new async client with the given task manager
func NewClient(manager *TaskManager) *Client {
	return &Client{
		manager: manager,
	}
}

// Get performs an async GET request with typed response
func Get[T any](c *Client, endpoint string) (*models.APIResponse[T], error) {
	return ExecuteAsync[T](c.manager, "GET", endpoint, nil)
}

// Post performs an async POST request with typed response
func Post[T any](c *Client, endpoint string, body interface{}) (*models.APIResponse[T], error) {
	return ExecuteAsync[T](c.manager, "POST", endpoint, body)
}

// Put performs an async PUT request with typed response
func Put[T any](c *Client, endpoint string, body interface{}) (*models.APIResponse[T], error) {
	return ExecuteAsync[T](c.manager, "PUT", endpoint, body)
}

// Patch performs an async PATCH request with typed response
func Patch[T any](c *Client, endpoint string, body interface{}) (*models.APIResponse[T], error) {
	return ExecuteAsync[T](c.manager, "PATCH", endpoint, body)
}
