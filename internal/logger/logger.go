package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

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
	logDir := "logs"
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

	Info("Logger initialized in file-only mode: %s", logPath)
	return nil
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
