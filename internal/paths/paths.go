// Package paths resolves the per-user base directory where rotki-sync keeps the
// files it manages: the downloaded rotki-core bundle and the core log. Anchoring
// these to a stable location instead of the current working directory lets a
// binary installed in ~/.local/bin run from any directory.
package paths

import (
	"os"
	"path/filepath"
)

// Home returns rotki-sync's base data directory, resolved in order:
//
//  1. $ROTKI_SYNC_HOME, if set (explicit override)
//  2. $XDG_DATA_HOME/rotki-sync, if XDG_DATA_HOME is set
//  3. ~/.local/share/rotki-sync
//
// As a last resort (no resolvable home directory) it falls back to ".", so the
// behavior degrades to the previous cwd-relative layout rather than failing.
func Home() string {
	if d := os.Getenv("ROTKI_SYNC_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "rotki-sync")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return filepath.Join(home, ".local", "share", "rotki-sync")
}

// BinDir is the directory where the rotki-core bundle is downloaded and looked
// up (<home>/bin).
func BinDir() string {
	return filepath.Join(Home(), "bin")
}

// LogDir is the directory where rotki-core logs are written (<home>/logs).
func LogDir() string {
	return filepath.Join(Home(), "logs")
}
