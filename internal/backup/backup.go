package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
)

func GetDataHome() (string, error) {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return dataHome, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
			return appData, nil
		}
		return filepath.Join(homeDir, "AppData", "Local"), nil
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support"), nil
	default:
		return filepath.Join(homeDir, ".local", "share"), nil
	}
}

// GetDefaultRotkiDataDir returns the default Rotki data directory in an OS-independent way
func GetDefaultRotkiDataDir() (string, error) {
	homeDir, err := GetDataHome()
	if err != nil {
		return "", fmt.Errorf("failed to get data directory: %w", err)
	}

	dataDir := filepath.Join(homeDir, "rotki", "data")
	return dataDir, nil
}

// GetDefaultBackupDir returns the default backup directory
func GetDefaultBackupDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	backupDir := filepath.Join(homeDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	return backupDir, nil
}

// CreateBackup creates a backup of the Rotki data directory
func CreateBackup(dataDir, backupDir string) (string, error) {
	if dataDir == "" {
		var err error
		dataDir, err = GetDefaultRotkiDataDir()
		if err != nil {
			return "", fmt.Errorf("failed to get default Rotki data directory: %w", err)
		}
	}

	if backupDir == "" {
		var err error
		backupDir, err = GetDefaultBackupDir()
		if err != nil {
			return "", fmt.Errorf("failed to get default backup directory: %w", err)
		}
	}

	timestamp := filepath.Base(time.Now().Format("20060102_150405"))
	backupFile := filepath.Join(backupDir, fmt.Sprintf("rotki_backup_%s.zip", timestamp))

	// Create a new zip file
	zipFile, err := os.Create(backupFile)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the data directory and add files to the zip
	err = filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		return AddToZip(path, info, err, dataDir, zipWriter)
	})

	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	logger.Info("Backup created successfully: %s", backupFile)
	return backupFile, nil
}

func AddToZip(path string, info os.FileInfo, err error, dataDir string, zipWriter *zip.Writer) error {
	if err != nil {
		return err
	}

	if path == dataDir {
		return nil
	}

	relPath, err := filepath.Rel(dataDir, path)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}

	if !ShouldIncludeInBackup(relPath, info.IsDir()) {
		if info.IsDir() {
			logger.Debug("Skipping directory: %s", relPath)
			return filepath.SkipDir
		}
		logger.Debug("Skipping file: %s", relPath)
		return nil
	}

	if info.IsDir() {
		_, err = zipWriter.Create(relPath + "/")
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("failed to create file header: %w", err)
	}

	header.Name = relPath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create file in zip: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	logger.Debug("Added file to backup: %s", relPath)
	return nil
}

// ShouldIncludeInBackup checks if a file or directory should be included in the backup
func ShouldIncludeInBackup(relPath string, isDir bool) bool {
	components := strings.Split(relPath, string(filepath.Separator))
	if len(components) == 0 {
		return false
	}

	switch components[0] {
	case "app", "images":
		return true
	case "global":
		if isDir {
			return true
		}
		return strings.HasSuffix(relPath, ".db")
	case "users":
		if len(components) < 2 {
			// Include the users directory itself
			return true
		}
		if len(components) == 2 && isDir {
			// Include user subdirectories
			return true
		}
		if len(components) == 3 && !isDir {
			filename := components[2]
			return filename == "dbinfo.json" || filename == "rotkehlchen.db" || filename == "rotkehlchen_transient.db"
		}
		return false
	default:
		return false
	}
}
