package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var log zerolog.Logger

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
