package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/kelsos/rotki-sync/internal/backup"
	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/download"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/process"
	"github.com/kelsos/rotki-sync/internal/services"
	"github.com/kelsos/rotki-sync/internal/tui"
	"github.com/kelsos/rotki-sync/internal/utils"
)

// backupProgressPrinter returns a ProgressFunc suitable for the backup
// command. When stderr is a TTY it renders a single overwriting line; on a
// non-interactive stderr it returns nil so output stays clean for logs.
func backupProgressPrinter() backup.ProgressFunc {
	if !isatty.IsTerminal(os.Stderr.Fd()) {
		return nil
	}
	return func(current, total int, relPath string) {
		// Truncate long paths so the line stays on one row.
		const maxPath = 60
		display := relPath
		if len(display) > maxPath {
			display = "…" + display[len(display)-maxPath+1:]
		}
		percent := 0
		if total > 0 {
			percent = current * 100 / total
		}
		fmt.Fprintf(os.Stderr, "\r\033[K[%d/%d %3d%%] %s", current, total, percent, display)
		if current == total {
			fmt.Fprintln(os.Stderr)
		}
	}
}

// runSync wires up rotki-core and runs the sync flow with or without the TUI.
func runSync(cfg *config.Config, disableTUI, skipConfirm bool) {
	if !disableTUI {
		if err := logger.InitFileOnly(); err != nil {
			logger.Init()
			logger.Error("Failed to initialize file logging, using console: %v", err)
		}
		defer logger.Close()
	} else {
		logger.Init()
	}

	cfg.SetBaseURL()

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration: %v", err)
	}

	rotki, err := process.StartRotkiCore(cfg.BinPath, cfg.Port, cfg.APIReadyTimeout, cfg.DataDir)
	if err != nil {
		logger.Fatal("Failed to start rotki-core: %v", err)
	}

	syncService := services.NewSyncService(cfg)

	if !syncService.WaitForAPIReady() {
		logger.Fatal("API failed to become ready")
	}

	if !confirmRotkiVersion(syncService, skipConfirm) {
		logger.Info("Sync canceled by user")
		if err := rotki.Stop(); err != nil {
			logger.Error("Failed to stop rotki-core: %v", err)
		}
		return
	}

	if !disableTUI {
		monitor := tui.NewSyncMonitor(syncService)
		if err := monitor.Start(); err != nil {
			logger.Fatal("Failed to start TUI monitor: %v", err)
		}
		if err := monitor.Run(); err != nil {
			logger.Error("Error running TUI monitor: %v", err)
		}
	} else {
		if err := syncService.ProcessAllUsers(); err != nil {
			logger.Error("Error processing users: %v", err)
		}
		logger.Info("All users processed successfully")
	}

	defer syncService.Cleanup()

	if err := rotki.WaitForExit(); err != nil {
		logger.Error("Error waiting for rotki-core to exit: %v", err)
	}
}

// confirmRotkiVersion prompts the user to confirm the running rotki-core
// version. Returns true when the user accepts. When skipPrompt is true the
// version is logged and the function returns true without asking.
func confirmRotkiVersion(syncService *services.SyncService, skipPrompt bool) bool {
	info, err := syncService.GetInfo()
	if err != nil {
		logger.Error("Failed to fetch rotki-core version: %v", err)
		return false
	}

	fmt.Printf("rotki-core version: %s\n", info.Version.OurVersion)
	fmt.Printf("data directory:     %s\n", info.DataDirectory)

	if skipPrompt {
		return true
	}

	fmt.Print("Continue sync against this backend? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		logger.Error("Failed to read confirmation: %v", err)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func main() {
	utils.LoadEnvironment()

	// Initialize basic console logger (will be reconfigured later based on flags)
	logger.Init()

	// Initialize configuration with defaults
	cfg := config.NewConfig()
	cfg.LoadFromEnvironment()

	var backupDir string
	var disableTUI bool
	var skipConfirm bool

	rootCmd := &cobra.Command{
		Use:   "rotki-sync",
		Short: "A CLI tool for syncing rotki data",
		Long:  `rotki-sync is a CLI tool for syncing rotki data from various sources.`,
		Run: func(cmd *cobra.Command, args []string) {
			runSync(cfg, disableTUI, skipConfirm)
		},
	}

	// Add a download command
	downloadCmd := &cobra.Command{
		Use:   "download",
		Short: "Download the latest rotki-core binary",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Init() // Always use console for subcommands
			if err := download.DownloadRotkiCore(); err != nil {
				logger.Fatal("Failed to download rotki-core: %v", err)
			}
		},
	}

	// Add a backup command
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a backup of rotki's data directory",
		Long:  `Create a backup of rotki's data directory, including specific files and directories.`,
		Run: func(cmd *cobra.Command, args []string) {
			logger.Init() // Always use console for subcommands
			progress := backupProgressPrinter()
			backupFile, err := backup.CreateBackup(cfg.DataDir, backupDir, progress)
			if err != nil {
				logger.Fatal("Failed to create backup: %v", err)
			}
			logger.Info("Backup created successfully: %s", backupFile)
		},
	}
	backupCmd.Flags().StringVarP(&backupDir, "backup-dir", "", cfg.BackupDir, "Directory where the backup will be stored")

	// Add flags that bind to the configuration
	rootCmd.Flags().IntVarP(&cfg.Port, "port", "p", cfg.Port, "Port to run rotki-core on")
	rootCmd.Flags().StringVarP(&cfg.BinPath, "bin-path", "b", cfg.BinPath, "Path to rotki-core binary")
	rootCmd.Flags().StringVarP(&cfg.DataDir, "data-dir", "", cfg.DataDir, "Directory where rotki's data resides")
	rootCmd.Flags().IntVarP(&cfg.MaxRetries, "max-retries", "r", cfg.MaxRetries, "Maximum number of balance fetch retries")

	var retryDelayMs int
	rootCmd.Flags().IntVarP(&retryDelayMs, "retry-delay", "d", int(cfg.RetryDelay/time.Millisecond), "Delay between retries in milliseconds")
	rootCmd.Flags().IntVarP(&cfg.APIReadyTimeout, "api-ready-timeout", "t", cfg.APIReadyTimeout, "Maximum attempts to check API readiness")
	rootCmd.Flags().BoolVarP(&disableTUI, "no-tui", "", false, "Disable interactive TUI monitoring mode")
	rootCmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the rotki-core version confirmation prompt")

	// Update retry delay from milliseconds to duration
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		cfg.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
	}

	// Add subcommands
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(backupCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Failed to execute command: %v", err)
	}
}
