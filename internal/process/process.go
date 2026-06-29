package process

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// logDirName and logFileName locate the rotki-core log file, written relative
// to the working directory. LogFilePath is the single source of truth for this
// path so the launcher and the log-tailing progress tracker stay in sync.
const (
	logDirName  = "logs"
	logFileName = "rotki-core.log"
)

// LogFilePath returns the path rotki-core is configured to log to.
func LogFilePath() string {
	return filepath.Join(logDirName, logFileName)
}

// RotkiProcess represents a running rotki-core process
type RotkiProcess struct {
	Cmd     *exec.Cmd
	Process *os.Process
	Port    int
	BinPath string
}

// StartRotkiCore starts the rotki-core process and returns a RotkiProcess
func StartRotkiCore(binPath string, port int, apiReadyTimeout int, dataDir string) (*RotkiProcess, error) {
	if port < 1024 || port > 65535 {
		return nil, fmt.Errorf("port must be between 1024 and 65535, got: %d", port)
	}

	logger.Info("Starting rotki-core at port %d...", port)

	if !filepath.IsAbs(binPath) {
		absPath, err := filepath.Abs(binPath)
		if err != nil {
			return nil, fmt.Errorf("invalid binary path: %v", err)
		}
		binPath = absPath
	}

	binPath = filepath.Clean(binPath)
	var args []string
	args = append(args, "--rest-api-port", strconv.Itoa(port), "--disable-task-manager")

	if dataDir != "" {
		args = append(args, "--data-dir", dataDir)
	}

	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logDirName, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Configure rotki-core to log to file in the logs directory
	logPath := LogFilePath()
	args = append(args, "--logtarget", "file", "--logfile", logPath)

	logger.Info("rotki-core logs will be written to: %s", logPath)

	// #nosec G204 - Parameters have been validated and sanitized above
	cmd := exec.Command(binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start rotki-core: %w", err)
	}

	rotki := &RotkiProcess{
		Cmd:     cmd,
		Process: cmd.Process,
		Port:    port,
		BinPath: binPath,
	}

	logger.Info("rotki-core process started successfully")
	return rotki, nil
}

// Stop terminates the rotki-core process. Returns nil if the process is not
// running.
func (r *RotkiProcess) Stop() error {
	if r.Process == nil {
		return nil
	}
	logger.Info("Stopping rotki-core...")
	return r.Process.Kill()
}

// WaitForExit waits for the rotki-core process to exit or for a signal to terminate it
func (r *RotkiProcess) WaitForExit() error {
	// Set up a channel to handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Set up a channel to handle process exit
	done := make(chan error, 1)
	go func() {
		done <- r.Cmd.Wait()
	}()

	// Wait for a signal or for the process to exit
	select {
	case sig := <-sigChan:
		logger.Info("Received signal %v, terminating rotki-core...", sig)
		if r.Process != nil {
			err := r.Process.Kill()
			if err != nil {
				return err
			}
		}
		return fmt.Errorf("process terminated by signal: %v", sig)
	case err := <-done:
		if err != nil {
			logger.Error("rotki-core exited with error: %v", err)
			return fmt.Errorf("process exited with error: %w", err)
		} else {
			logger.Info("rotki-core exited successfully")
			return nil
		}
	}
}
