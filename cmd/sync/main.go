package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kelsos/rotki-sync/internal/backup"
	"github.com/kelsos/rotki-sync/internal/blockchain"
	"github.com/kelsos/rotki-sync/internal/download"
	"github.com/kelsos/rotki-sync/internal/exchanges"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/process"
	"github.com/kelsos/rotki-sync/internal/user"
	"github.com/kelsos/rotki-sync/internal/utils"
)

func processUserData(baseURL, username string) error {
	if err := blockchain.PerformSnapshotIfNeeded(baseURL); err != nil {
		logger.Error("Failed to perform snapshot: %v", err)
	}

	if err := exchanges.GetExchangeTrades(baseURL); err != nil {
		logger.Error("Failed fetch exchange trades: %v", err)
	}

	if err := blockchain.FetchOnlineEvents(baseURL); err != nil {
		logger.Error("Failed to fetch online events: %v", err)
	}

	if err := blockchain.FetchEvmTransactions(baseURL, 0, 0); err != nil {
		logger.Error("Failed to fetch EVM transactions: %v", err)
	}

	if err := blockchain.DecodeEvmTransactions(baseURL); err != nil {
		logger.Error("Failed to decode EVM transactions: %v", err)
	}

	return nil
}

func main() {
	utils.LoadEnvironment()
	logger.Init()

	var (
		port            int
		binPath         string
		maxRetries      int
		retryDelay      int
		apiReadyTimeout int
		dataDir         string
	)

	rootCmd := &cobra.Command{
		Use:   "rotki-sync",
		Short: "A CLI tool for syncing rotki data",
		Long:  `rotki-sync is a CLI tool for syncing rotki data from various sources.`,
		Run: func(cmd *cobra.Command, args []string) {
			rotki, err := process.StartRotkiCore(binPath, port, apiReadyTimeout, dataDir)
			if err != nil {
				logger.Fatal("Failed to start rotki-core: %v", err)
			}

			baseURL := fmt.Sprintf("http://localhost:%d", port)

			if err := user.ProcessUsers(baseURL, processUserData); err != nil {
				logger.Error("Error processing users: %v", err)
			}

			logger.Info("All users processed successfully")

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
			if err := download.DownloadRotkiCore(); err != nil {
				logger.Fatal("Failed to download rotki-core: %v", err)
			}
		},
	}

	// Add a backup command
	var backupDir string
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a backup of rotki's data directory",
		Long:  `Create a backup of rotki's data directory, including specific files and directories.`,
		Run: func(cmd *cobra.Command, args []string) {
			backupFile, err := backup.CreateBackup(dataDir, backupDir)
			if err != nil {
				logger.Fatal("Failed to create backup: %v", err)
			}
			logger.Info("Backup created successfully: %s", backupFile)
		},
	}
	backupCmd.Flags().StringVarP(&backupDir, "backup-dir", "", "", "Directory where the backup will be stored (default: ~/backups)")

	// Add flags
	rootCmd.Flags().IntVarP(&port, "port", "p", 59001, "Port to run rotki-core on")
	rootCmd.Flags().StringVarP(&binPath, "bin-path", "b", "bin/rotki-core", "Path to rotki-core binary")
	rootCmd.Flags().StringVarP(&dataDir, "data-dir", "", "", "Directory where rotki's data resides")
	rootCmd.Flags().IntVarP(&maxRetries, "max-retries", "r", 10, "Maximum number of balance fetch retries")
	rootCmd.Flags().IntVarP(&retryDelay, "retry-delay", "d", 2000, "Delay between retries in milliseconds")
	rootCmd.Flags().IntVarP(&apiReadyTimeout, "api-ready-timeout", "t", 30, "Maximum attempts to check API readiness")

	// Add subcommands
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(backupCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Failed to execute command: %v", err)
	}
}
