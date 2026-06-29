// Package alert sends run-failure notifications to an external channel so a
// silent, green-looking cron run can never again hide a broken sync.
package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// Urgency maps to notify-send's --urgency levels for desktop notifications.
type Urgency string

const (
	UrgencyNormal   Urgency = "normal"
	UrgencyCritical Urgency = "critical"
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

// Desktop shows a best-effort desktop notification via notify-send (libnotify),
// suited to the local --user timer. It is a no-op when notify-send is not
// installed; delivery failures are logged, never returned, so a notification can
// never affect the run's outcome.
func Desktop(title, body string, urgency Urgency) {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		logger.Debug("Desktop notifications unavailable (notify-send not found)")
		return
	}

	// #nosec G204 - path comes from LookPath; args are app-controlled, not user input
	cmd := exec.Command(path, notifySendArgs(title, body, urgency)...)
	if err := cmd.Run(); err != nil {
		logger.Debug("Failed to send desktop notification: %v", err)
		return
	}
	logger.Debug("Desktop notification sent: %s", title)
}

// notifySendArgs builds the notify-send argument list (separated for testing).
func notifySendArgs(title, body string, urgency Urgency) []string {
	args := []string{"--app-name=rotki-sync"}
	if urgency != "" {
		args = append(args, "--urgency="+string(urgency))
	}
	return append(args, title, body)
}
