package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/logger"
)

// APIClient handles all HTTP communication with the Rotki API
type APIClient struct {
	config     *config.Config
	httpClient *http.Client
}

// NewAPIClient creates a new API client with the given configuration
func NewAPIClient(cfg *config.Config) *APIClient {
	return &APIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BuildURL constructs a full URL for the given endpoint
func (c *APIClient) BuildURL(endpoint string) string {
	return fmt.Sprintf("%s/api/1%s", c.config.BaseURL, endpoint)
}

// Get makes a GET request to the specified endpoint
func (c *APIClient) Get(endpoint string, result interface{}) error {
	return c.request(http.MethodGet, endpoint, nil, result)
}

// Post makes a POST request to the specified endpoint
func (c *APIClient) Post(endpoint string, body interface{}, result interface{}) error {
	return c.request(http.MethodPost, endpoint, body, result)
}

// Put makes a PUT request to the specified endpoint
func (c *APIClient) Put(endpoint string, body interface{}, result interface{}) error {
	return c.request(http.MethodPut, endpoint, body, result)
}

// Delete makes a DELETE request to the specified endpoint
func (c *APIClient) Delete(endpoint string, result interface{}) error {
	return c.request(http.MethodDelete, endpoint, nil, result)
}

// Patch makes a PATCH request to the specified endpoint
func (c *APIClient) Patch(endpoint string, body interface{}, result interface{}) error {
	return c.request(http.MethodPatch, endpoint, body, result)
}

// request is the core HTTP request method
func (c *APIClient) request(method, endpoint string, body interface{}, result interface{}) error {
	url := c.BuildURL(endpoint)
	start := time.Now()
	logger.Debug("Starting %s request to %s", method, url)

	var requestBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("error marshaling request body: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		elapsed := time.Since(start)
		logger.Error("Request failed after (%s) %v: %v", url, elapsed, err)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	logger.Debug("Request to %s completed in %v with status %d", url, elapsed, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Error("%s: HTTP error %d: %s", url, resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			logger.Error("%s: Error decoding response: %v", url, err)
			return fmt.Errorf("error decoding response: %w", err)
		}
	}

	return nil
}

// Ping checks if the API is ready
func (c *APIClient) Ping() error {
	url := c.BuildURL("/ping")
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status %d", resp.StatusCode)
	}

	return nil
}

// WaitForAPIReady waits for the API to become ready
func (c *APIClient) WaitForAPIReady() bool {
	logger.Info("Checking API readiness...")

	for attempt := 1; attempt <= c.config.APIReadyTimeout; attempt++ {
		logger.Info("Checking API readiness (attempt %d/%d)...", attempt, c.config.APIReadyTimeout)

		if err := c.Ping(); err == nil {
			logger.Info("API is ready!")
			return true
		}

		time.Sleep(time.Second)
	}

	logger.Error("API failed to become ready after %d attempts", c.config.APIReadyTimeout)
	return false
}

// BuildURLWithParams properly builds a URL with query parameters
func BuildURLWithParams(endpoint string, params map[string]string) string {
	if len(params) == 0 {
		return endpoint
	}

	// Parse the endpoint to check for existing query parameters
	parts := strings.SplitN(endpoint, "?", 2)
	baseURL := parts[0]

	// Parse existing query parameters if any
	values := url.Values{}
	if len(parts) > 1 {
		existingParams, _ := url.ParseQuery(parts[1])
		values = existingParams
	}

	// Add new parameters
	for key, value := range params {
		values.Set(key, value)
	}

	// Build the final URL
	if len(values) > 0 {
		return baseURL + "?" + values.Encode()
	}
	return baseURL
}
