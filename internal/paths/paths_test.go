package paths

import (
	"path/filepath"
	"testing"
)

func TestHomeResolutionOrder(t *testing.T) {
	t.Run("ROTKI_SYNC_HOME wins", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_HOME", "/explicit/home")
		t.Setenv("XDG_DATA_HOME", "/xdg/data")
		if got := Home(); got != "/explicit/home" {
			t.Fatalf("Home() = %q, want /explicit/home", got)
		}
	})

	t.Run("XDG_DATA_HOME used when no override", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_HOME", "")
		t.Setenv("XDG_DATA_HOME", "/xdg/data")
		if got, want := Home(), filepath.Join("/xdg/data", "rotki-sync"); got != want {
			t.Fatalf("Home() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.local/share/rotki-sync", func(t *testing.T) {
		t.Setenv("ROTKI_SYNC_HOME", "")
		t.Setenv("XDG_DATA_HOME", "")
		home := t.TempDir()
		t.Setenv("HOME", home)
		if got, want := Home(), filepath.Join(home, ".local", "share", "rotki-sync"); got != want {
			t.Fatalf("Home() = %q, want %q", got, want)
		}
	})
}

func TestBinAndLogDirsAreUnderHome(t *testing.T) {
	t.Setenv("ROTKI_SYNC_HOME", "/base")
	if got, want := BinDir(), filepath.Join("/base", "bin"); got != want {
		t.Errorf("BinDir() = %q, want %q", got, want)
	}
	if got, want := LogDir(), filepath.Join("/base", "logs"); got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}
