// Package alert sends run-failure notifications to an external channel so a
// silent, green-looking cron run can never again hide a broken sync.
package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// webhookEnvVar names the environment variable holding the alert webhook URL.
// When unset, alerting is disabled and Notify is a no-op.
const webhookEnvVar = "ROTKI_SYNC_ALERT_WEBHOOK"

// Enabled reports whether an alert destination is configured.
func Enabled() bool {
	return os.Getenv(webhookEnvVar) != ""
}

// Notify posts a failure message to the configured webhook. It is a no-op when
// no webhook is configured. Delivery failures are logged, not returned: alerting
// must never change the run's own exit status.
//
// The payload uses a {"text": ...} shape, which Slack, Mattermost and most
// generic webhook receivers accept.
func Notify(title, body string) {
	webhook := os.Getenv(webhookEnvVar)
	if webhook == "" {
		logger.Debug("Alerting disabled (%s not set); skipping notification", webhookEnvVar)
		return
	}

	payload := map[string]string{"text": fmt.Sprintf("%s\n\n%s", title, body)}
	data, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to marshal alert payload: %v", err)
		return
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	// #nosec G704 G107 -- webhook URL comes from operator-controlled config (env var), not user input
	resp, err := httpClient.Post(webhook, "application/json", bytes.NewReader(data))
	if err != nil {
		logger.Error("Failed to deliver alert to webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.Error("Alert webhook returned status %d", resp.StatusCode)
		return
	}

	logger.Info("Failure alert delivered to webhook")
}
