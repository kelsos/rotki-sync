package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/kelsos/rotki-sync/internal/paths"
)

// defaultLogKeep is the number of most-recent per-run rotki-sync_*.log files
// kept when pruning. Overridable via ROTKI_SYNC_LOG_KEEP.
const defaultLogKeep = 20

var log zerolog.Logger
var logFile *os.File

func Init() {
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	output.FormatLevel = func(i interface{}) string {
		return fmt.Sprintf("[%s]", i)
	}
	output.FormatMessage = func(i interface{}) string {
		return fmt.Sprintf("%s", i)
	}

	log = zerolog.New(output).With().Timestamp().Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	if _, exists := os.LookupEnv("DEBUG"); exists {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

// InitFileOnly initializes the logger to write only to a file (for TUI mode)
func InitFileOnly() error {
	// Create logs directory if it doesn't exist
	logDir := paths.LogDir()
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logPath := filepath.Join(logDir, fmt.Sprintf("rotki-sync_%s.log", timestamp))

	var err error
	logFile, err = os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Use JSON format for file logging (easier to parse)
	log = zerolog.New(logFile).With().Timestamp().Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	if _, exists := os.LookupEnv("DEBUG"); exists {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// A fresh per-run log file is created every run; prune old ones so they do
	// not accumulate unbounded. (rotki-core rotates its own log separately.)
	pruneOldLogs(logDir, logKeepCount())

	Info("Logger initialized in file-only mode: %s", logPath)
	return nil
}

// logKeepCount returns how many recent per-run log files to keep, from
// ROTKI_SYNC_LOG_KEEP when set to a valid non-negative integer, else the
// default. A value of 0 disables pruning.
func logKeepCount() int {
	if v := os.Getenv("ROTKI_SYNC_LOG_KEEP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultLogKeep
}

// pruneOldLogs deletes all but the newest keep rotki-sync_*.log files in dir.
// The timestamped file name sorts chronologically, so a reverse lexical sort
// puts the newest first. It is best-effort: failures are logged at debug and do
// not interrupt the run. keep <= 0 disables pruning.
func pruneOldLogs(dir string, keep int) {
	if keep <= 0 {
		return
	}
	matches, err := filepath.Glob(filepath.Join(dir, "rotki-sync_*.log"))
	if err != nil || len(matches) <= keep {
		return
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	for _, path := range matches[keep:] {
		if err := os.Remove(path); err != nil {
			Debug("Could not prune old log %s: %v", path, err)
		}
	}
}

// Close closes the log file if it's open
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// SetOutput sets the output destination for the logger
func SetOutput(w io.Writer) {
	output := zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
	output.FormatLevel = func(i interface{}) string {
		return fmt.Sprintf("[%s]", i)
	}
	output.FormatMessage = func(i interface{}) string {
		return fmt.Sprintf("%s", i)
	}
	log = zerolog.New(output).With().Timestamp().Logger()
}

// Debug logs a debug message
func Debug(msg string, args ...interface{}) {
	log.Debug().Msgf(msg, args...)
}

// Info logs an info message
func Info(msg string, args ...interface{}) {
	log.Info().Msgf(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...interface{}) {
	log.Warn().Msgf(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...interface{}) {
	log.Error().Msgf(msg, args...)
}

// Fatal logs a fatal message and exits the program
func Fatal(msg string, args ...interface{}) {
	log.Fatal().Msgf(msg, args...)
}
