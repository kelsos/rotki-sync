package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TimestampData represents the structure of the timestamp data stored in the file
type TimestampData struct {
	LastToTimestamp int64 `json:"last_to_timestamp"`
	UpdatedAt       int64 `json:"updated_at"`
}

// GetAppDataDir returns the application data directory
func GetAppDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	appDataDir := filepath.Join(homeDir, ".rotki-sync")
	if err := os.MkdirAll(appDataDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create app data directory: %w", err)
	}

	return appDataDir, nil
}

// GetTimestampFilePath returns the path to the timestamp file for a specific chain
func GetTimestampFilePath(chain string) (string, error) {
	appDataDir, err := GetAppDataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(appDataDir, fmt.Sprintf("%s_timestamp.json", chain)), nil
}

// SaveTimestamp saves the timestamp to a file
func SaveTimestamp(chain string, timestamp int64) error {
	filePath, err := GetTimestampFilePath(chain)
	if err != nil {
		return err
	}

	data := TimestampData{
		LastToTimestamp: timestamp,
		UpdatedAt:       time.Now().Unix(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal timestamp data: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write timestamp file: %w", err)
	}

	return nil
}

// GetLastTimestamp gets the last timestamp from a file
func GetLastTimestamp(chain string) (int64, error) {
	filePath, err := GetTimestampFilePath(chain)
	if err != nil {
		return 0, err
	}

	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		return 0, nil
	}

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read timestamp file: %w", err)
	}

	var data TimestampData
	if err := json.Unmarshal(fileData, &data); err != nil {
		return 0, fmt.Errorf("failed to unmarshal timestamp data: %w", err)
	}

	return data.LastToTimestamp, nil
}
