package alert

import (
	"slices"
	"testing"
)

func TestNotifySendArgs(t *testing.T) {
	t.Run("with urgency", func(t *testing.T) {
		got := notifySendArgs("rotki-sync", "done", UrgencyCritical)
		want := []string{"--app-name=rotki-sync", "--urgency=critical", "rotki-sync", "done"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("without urgency", func(t *testing.T) {
		got := notifySendArgs("rotki-sync", "done", "")
		want := []string{"--app-name=rotki-sync", "rotki-sync", "done"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestEnabled(t *testing.T) {
	t.Setenv(webhookEnvVar, "")
	if Enabled() {
		t.Error("Enabled() should be false when webhook env is empty")
	}
	t.Setenv(webhookEnvVar, "https://example.com/hook")
	if !Enabled() {
		t.Error("Enabled() should be true when webhook env is set")
	}
}
