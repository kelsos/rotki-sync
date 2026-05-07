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
)

// ProgressFunc is called once per file added to a backup, with the running
// count, the total number of files to add, and the file's relative path. It is
// invoked before the file's contents are written.
type ProgressFunc func(current, total int, relPath string)

// ExpandPath expands ~ to the user's home directory
func ExpandPath(path string) (string, error) {
	if path == "" {
		return path, nil
	}

	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		return filepath.Join(homeDir, path[2:]), nil
	}

	return path, nil
}

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

// CreateBackup creates a backup of the Rotki data directory. If progress is
// non-nil it is invoked before writing each included file so the caller can
// render UI feedback.
func CreateBackup(dataDir, backupDir string, progress ProgressFunc) (string, error) {
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
	} else {
		// Expand ~ in the path if present
		expandedPath, err := ExpandPath(backupDir)
		if err != nil {
			return "", fmt.Errorf("failed to expand backup directory path: %w", err)
		}
		backupDir = expandedPath

		// Ensure the backup directory exists
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create backup directory: %w", err)
		}
	}

	totalFiles, err := countBackupFiles(dataDir)
	if err != nil {
		return "", fmt.Errorf("failed to scan data directory: %w", err)
	}

	timestamp := filepath.Base(time.Now().Format("20060102_150405"))
	backupFile := filepath.Join(backupDir, fmt.Sprintf("rotki_backup_%s.zip", timestamp))

	zipFile, err := os.Create(backupFile)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	current := 0
	err = filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		return addToZip(path, info, err, dataDir, zipWriter, func(relPath string) {
			current++
			if progress != nil {
				progress(current, totalFiles, relPath)
			}
		})
	})
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupFile, nil
}

// countBackupFiles returns the number of regular files under dataDir that
// would be included by ShouldIncludeInBackup.
func countBackupFiles(dataDir string) (int, error) {
	count := 0
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dataDir {
			return nil
		}
		relPath, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		if !ShouldIncludeInBackup(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

func addToZip(path string, info os.FileInfo, walkErr error, dataDir string, zipWriter *zip.Writer, onFile func(relPath string)) error {
	if walkErr != nil {
		return walkErr
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
			return filepath.SkipDir
		}
		return nil
	}

	if info.IsDir() {
		_, err = zipWriter.Create(relPath + "/")
		return err
	}

	if onFile != nil {
		onFile(relPath)
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

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

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
