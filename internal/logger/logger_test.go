package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogKeepCount(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_LOG_KEEP", "")
		if got := logKeepCount(); got != defaultLogKeep {
			t.Fatalf("logKeepCount() = %d, want %d", got, defaultLogKeep)
		}
	})
	t.Run("honors valid override", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_LOG_KEEP", "5")
		if got := logKeepCount(); got != 5 {
			t.Fatalf("logKeepCount() = %d, want 5", got)
		}
	})
	t.Run("zero disables", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_LOG_KEEP", "0")
		if got := logKeepCount(); got != 0 {
			t.Fatalf("logKeepCount() = %d, want 0", got)
		}
	})
	t.Run("invalid falls back to default", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_LOG_KEEP", "-3")
		if got := logKeepCount(); got != defaultLogKeep {
			t.Fatalf("logKeepCount() = %d, want %d", got, defaultLogKeep)
		}
	})
}

func TestPruneOldLogs(t *testing.T) {
	dir := t.TempDir()
	// Names sort chronologically, oldest first.
	names := []string{
		"rotki-sync_2026-01-01_00-00-00.log",
		"rotki-sync_2026-02-01_00-00-00.log",
		"rotki-sync_2026-03-01_00-00-00.log",
		"rotki-sync_2026-04-01_00-00-00.log",
		"rotki-sync_2026-05-01_00-00-00.log",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// An unrelated file must never be touched.
	other := filepath.Join(dir, "keep-me.txt")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	pruneOldLogs(dir, 2)

	// The two newest survive; the three oldest are gone.
	assertExists(t, filepath.Join(dir, "rotki-sync_2026-05-01_00-00-00.log"))
	assertExists(t, filepath.Join(dir, "rotki-sync_2026-04-01_00-00-00.log"))
	assertGone(t, filepath.Join(dir, "rotki-sync_2026-03-01_00-00-00.log"))
	assertGone(t, filepath.Join(dir, "rotki-sync_2026-02-01_00-00-00.log"))
	assertGone(t, filepath.Join(dir, "rotki-sync_2026-01-01_00-00-00.log"))
	assertExists(t, other)
}

func TestPruneOldLogsDisabledAndNoop(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"rotki-sync_2026-01-01_00-00-00.log", "rotki-sync_2026-02-01_00-00-00.log"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// keep <= 0 disables pruning.
	pruneOldLogs(dir, 0)
	assertExists(t, filepath.Join(dir, "rotki-sync_2026-01-01_00-00-00.log"))

	// keep >= count is a no-op.
	pruneOldLogs(dir, 10)
	assertExists(t, filepath.Join(dir, "rotki-sync_2026-01-01_00-00-00.log"))
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist: %v", path, err)
	}
}

func assertGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to be pruned, stat err = %v", path, err)
	}
}
