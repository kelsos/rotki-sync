package utils

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// WaitForAPIReady waits for the API to become ready by pinging it
func WaitForAPIReady(port int, maxAttempts int, delay time.Duration) bool {
	logger.Info("Checking API readiness...")

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		logger.Info("Checking API readiness (attempt %d/%d)...", attempt, maxAttempts)

		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/1/ping", port))

		if err == nil && resp.StatusCode == http.StatusOK {
			err := resp.Body.Close()
			if err != nil {
				return false
			}
			logger.Info("API is ready!")
			return true
		}

		if resp != nil {
			err := resp.Body.Close()
			if err != nil {
				return false
			}
		}

		time.Sleep(delay)
	}

	logger.Error("API failed to become ready after %d attempts", maxAttempts)
	return false
}
