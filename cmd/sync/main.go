package main

import (
	"time"

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

func main() {
	utils.LoadEnvironment()

	// Initialize basic console logger (will be reconfigured later based on flags)
	logger.Init()

	// Initialize configuration with defaults
	cfg := config.NewConfig()
	cfg.LoadFromEnvironment()

	var backupDir string
	var disableTUI bool

	rootCmd := &cobra.Command{
		Use:   "rotki-sync",
		Short: "A CLI tool for syncing rotki data",
		Long:  `rotki-sync is a CLI tool for syncing rotki data from various sources.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize logger based on TUI mode
			if !disableTUI {
				// Use file-only logging for TUI mode
				if err := logger.InitFileOnly(); err != nil {
					// Fallback to console logger if file init fails
					logger.Init()
					logger.Error("Failed to initialize file logging, using console: %v", err)
				}
				// Ensure we close the log file on exit
				defer logger.Close()
			} else {
				// Use console logging for non-TUI mode
				logger.Init()
			}

			// Update config with flag values
			cfg.SetBaseURL()

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				logger.Fatal("Invalid configuration: %v", err)
			}

			// Start rotki-core process
			rotki, err := process.StartRotkiCore(cfg.BinPath, cfg.Port, cfg.APIReadyTimeout, cfg.DataDir)
			if err != nil {
				logger.Fatal("Failed to start rotki-core: %v", err)
			}

			// Initialize sync service with the configuration
			syncService := services.NewSyncService(cfg)

			// Wait for API to be ready
			if !syncService.WaitForAPIReady() {
				logger.Fatal("API failed to become ready")
			}

			// Process users with or without TUI (TUI is default)
			if !disableTUI {
				// Run with TUI monitoring (default)
				monitor := tui.NewSyncMonitor(syncService)
				if err := monitor.Start(); err != nil {
					logger.Fatal("Failed to start TUI monitor: %v", err)
				}

				if err := monitor.Run(); err != nil {
					logger.Error("Error running TUI monitor: %v", err)
				}
			} else {
				// Process all users without TUI (when --no-tui flag is used)
				if err := syncService.ProcessAllUsers(); err != nil {
					logger.Error("Error processing users: %v", err)
				}
				logger.Info("All users processed successfully")
			}

			// Cleanup resources
			defer syncService.Cleanup()

			// Wait for rotki-core to exit
			if err := rotki.WaitForExit(); err != nil {
				logger.Error("Error waiting for rotki-core to exit: %v", err)
			}
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
			backupFile, err := backup.CreateBackup(cfg.DataDir, backupDir)
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
