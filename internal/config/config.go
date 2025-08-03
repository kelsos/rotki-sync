package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	// Server settings
	Port            int
	BinPath         string
	DataDir         string
	APIReadyTimeout int

	// Retry settings
	MaxRetries int
	RetryDelay time.Duration

	// API settings
	BaseURL string

	// Backup settings
	BackupDir string
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		Port:            59001,
		BinPath:         "bin/rotki-core",
		APIReadyTimeout: 30,
		MaxRetries:      10,
		RetryDelay:      2 * time.Second,
		BackupDir:       "~/backups",
	}
}

// LoadFromEnvironment loads configuration from environment variables
func (c *Config) LoadFromEnvironment() {
	if port := os.Getenv("ROTKI_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.Port = p
		}
	}

	if binPath := os.Getenv("ROTKI_BIN_PATH"); binPath != "" {
		c.BinPath = binPath
	}

	if dataDir := os.Getenv("ROTKI_DATA_DIR"); dataDir != "" {
		c.DataDir = dataDir
	}

	if timeout := os.Getenv("ROTKI_API_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			c.APIReadyTimeout = t
		}
	}

	if retries := os.Getenv("ROTKI_MAX_RETRIES"); retries != "" {
		if r, err := strconv.Atoi(retries); err == nil {
			c.MaxRetries = r
		}
	}

	if delay := os.Getenv("ROTKI_RETRY_DELAY"); delay != "" {
		if d, err := strconv.Atoi(delay); err == nil {
			c.RetryDelay = time.Duration(d) * time.Millisecond
		}
	}

	if backupDir := os.Getenv("ROTKI_BACKUP_DIR"); backupDir != "" {
		c.BackupDir = backupDir
	}
}

// SetBaseURL sets the base URL based on the configured port
func (c *Config) SetBaseURL() {
	c.BaseURL = fmt.Sprintf("http://localhost:%d", c.Port)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Port < 1024 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535, got: %d", c.Port)
	}

	if c.BinPath == "" {
		return fmt.Errorf("binary path cannot be empty")
	}

	if c.APIReadyTimeout <= 0 {
		return fmt.Errorf("API ready timeout must be positive, got: %d", c.APIReadyTimeout)
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries must be non-negative, got: %d", c.MaxRetries)
	}

	return nil
}
