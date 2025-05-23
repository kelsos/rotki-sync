package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// FetchWithValidation makes an HTTP request and validates the response
func FetchWithValidation[T any](url string, method string, body interface{}) (*T, error) {
	start := time.Now()
	logger.Debug("Starting request to %s", url)

	var requestBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("error marshaling request body: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		elapsed := time.Since(start)
		logger.Error("Request failed after (%s) %v: %v", url, elapsed, err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	logger.Debug("Request to %s completed in %v with status %d", url, elapsed, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Error("%s: HTTP error %d: %s", url, resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("%s: Error decoding response: %v", url, err)
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &result, nil
}
