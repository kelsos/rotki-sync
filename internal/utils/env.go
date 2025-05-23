package utils

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// LoadEnvironment loads environment variables from .env files
// It tries to load from the current directory and from the directory of the executable
func LoadEnvironment() {
	if err := godotenv.Load(); err != nil {
		logger.Info("No .env file found in current directory or error loading it: %v", err)
	} else {
		logger.Info("Successfully loaded .env file from current directory")
	}

	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		envPath := filepath.Join(execDir, ".env")
		if err := godotenv.Load(envPath); err != nil {
			logger.Info("No .env file found in app directory (%s) or error loading it: %v", execDir, err)
		} else {
			logger.Info("Successfully loaded .env file from app directory: %s", execDir)
		}
	} else {
		logger.Info("Could not determine executable path: %v", err)
	}
}
