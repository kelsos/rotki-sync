package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderServiceUnit(t *testing.T) {
	out := renderServiceUnit("/home/kelsos/.local/bin/rotki-sync")
	for _, want := range []string{
		"Type=oneshot",
		"ExecStart=/home/kelsos/.local/bin/rotki-sync --no-tui --yes",
		"After=graphical-session.target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("service unit missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderTimerUnit(t *testing.T) {
	out := renderTimerUnit(defaultSchedule)
	for _, want := range []string{
		"OnCalendar=" + defaultSchedule,
		"Persistent=true",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("timer unit missing %q in:\n%s", want, out)
		}
	}
}

func TestUserUnitDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	if got, want := userUnitDir(), filepath.Join("/cfg", "systemd", "user"); got != want {
		t.Fatalf("userUnitDir() = %q, want %q", got, want)
	}
}
