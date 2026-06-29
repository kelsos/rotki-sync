package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/kelsos/rotki-sync/internal/alert"
	"github.com/kelsos/rotki-sync/internal/backup"
	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/download"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/process"
	"github.com/kelsos/rotki-sync/internal/services"
	"github.com/kelsos/rotki-sync/internal/tui"
	"github.com/kelsos/rotki-sync/internal/utils"
)

// Build-time version stamp, injected via -ldflags "-X main.version=..." by the
// Makefile. Defaults make a plain `go build` (no ldflags) still report sanely.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Exit codes communicate the run outcome to the cron wrapper so a broken sync
// can never look green again.
const (
	exitOK            = 0 // all core steps did work
	exitStepFailure   = 1 // a core step ran but had zero successes
	exitContractBreak = 2 // a depended-on endpoint is gone (preflight or mid-run 404)
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
// It returns a process exit code so a non-interactive (cron) run can signal a
// failed or aborted sync instead of always exiting 0.
func runSync(cfg *config.Config, disableTUI, skipConfirm bool) int {
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
	defer syncService.Cleanup()

	if !syncService.WaitForAPIReady() {
		logger.Fatal("API failed to become ready")
	}

	if !confirmRotkiVersion(syncService, skipConfirm) {
		logger.Info("Sync canceled by user")
		stopRotki(rotki)
		return exitOK
	}

	// Preflight: catch a removed/renamed endpoint before doing any work, so a
	// contract break is an immediate, loud failure rather than a silent month
	// of missing data.
	if err := syncService.PreflightEndpoints(); err != nil {
		logger.Error("Endpoint preflight failed: %v", err)
		alert.Notify("rotki-sync: endpoint preflight failed", err.Error())
		stopRotki(rotki)
		return exitContractBreak
	}

	exitCode := exitOK

	if !disableTUI {
		monitor := tui.NewSyncMonitor(syncService)
		if err := monitor.Start(); err != nil {
			logger.Fatal("Failed to start TUI monitor: %v", err)
		}
		if err := monitor.Run(); err != nil {
			logger.Error("Error running TUI monitor: %v", err)
		}
	} else {
		report, err := syncService.ProcessAllUsers()
		if err != nil {
			logger.Error("Error processing users: %v", err)
		}
		exitCode = reportExitCode(report)
		logger.Info("%s", report.Summary())
		if exitCode == exitOK {
			logger.Info("Sync completed successfully")
		} else {
			logger.Error("Sync completed with failures (exit %d)", exitCode)
			alert.Notify(
				fmt.Sprintf("rotki-sync: run failed (exit %d)", exitCode),
				report.Summary())
		}
	}

	if err := rotki.WaitForExit(); err != nil {
		logger.Error("Error waiting for rotki-core to exit: %v", err)
	}

	return exitCode
}

// reportExitCode maps a run report to a process exit code: a contract break
// takes priority, then any other step failure, otherwise success.
func reportExitCode(report *services.RunReport) int {
	if report == nil {
		return exitStepFailure
	}
	if report.FatalErr != nil {
		return exitContractBreak
	}
	if report.HasFailures() {
		return exitStepFailure
	}
	return exitOK
}

// stopRotki stops rotki-core, logging any failure.
func stopRotki(rotki *process.RotkiProcess) {
	if err := rotki.Stop(); err != nil {
		logger.Error("Failed to stop rotki-core: %v", err)
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

	// Version-compatibility gate: a core upgrade is what removed the legacy EVM
	// transaction endpoints last time, so warn loudly when the running core is
	// outside the range the endpoint contract was tested against.
	if status := services.CheckCoreVersion(info.Version.OurVersion); !status.Compatible {
		logger.Warn("rotki-core version check: %s", status.Warning)
		fmt.Printf("WARNING: %s\n", status.Warning)
		fmt.Printf("         (last tested against rotki-core %s)\n", status.Tested)
	}

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

// versionString renders the build stamp as "rotki-sync <version> (commit <c>, built <date>)".
func versionString() string {
	return fmt.Sprintf("rotki-sync %s (commit %s, built %s)", version, commit, date)
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

	// exitCode is set by commands that need to control the process exit status
	// (e.g. a sync that failed or aborted). It is applied after Execute so
	// deferred cleanup still runs.
	exitCode := exitOK

	rootCmd := &cobra.Command{
		Use:     "rotki-sync",
		Short:   "A CLI tool for syncing rotki data",
		Long:    `rotki-sync is a CLI tool for syncing rotki data from various sources.`,
		Version: version,
		Run: func(cmd *cobra.Command, args []string) {
			exitCode = runSync(cfg, disableTUI, skipConfirm)
		},
	}
	// Richer `--version` output than cobra's default one-liner, and a matching
	// `version` subcommand for callers that prefer it.
	rootCmd.SetVersionTemplate(versionString() + "\n")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(versionString())
		},
	}

	// Add a preflight command: boot rotki-core and assert every endpoint the
	// CLI depends on is still registered, without running a sync. Intended for
	// CI to catch endpoint removal the moment the bundled core is bumped.
	preflightCmd := &cobra.Command{
		Use:   "preflight",
		Short: "Verify the backend still exposes every endpoint the sync depends on",
		Run: func(cmd *cobra.Command, args []string) {
			exitCode = runPreflight(cfg)
		},
	}
	preflightCmd.Flags().IntVarP(&cfg.Port, "port", "p", cfg.Port, "Port to run rotki-core on")
	preflightCmd.Flags().StringVarP(&cfg.BinPath, "bin-path", "b", cfg.BinPath, "Path to rotki-core binary")
	preflightCmd.Flags().StringVarP(&cfg.DataDir, "data-dir", "", cfg.DataDir, "Directory where rotki's data resides")
	preflightCmd.Flags().IntVarP(&cfg.APIReadyTimeout, "api-ready-timeout", "t", cfg.APIReadyTimeout, "Maximum attempts to check API readiness")

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
	rootCmd.AddCommand(preflightCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(secretCmd(cfg))

	// Add an `install` subcommand under Cobra's auto-generated `completion`
	// command (which only prints), so users can install/update completions in
	// one step. InitDefaultCompletionCmd materializes that command now; Execute
	// sees it already present and won't re-add it.
	rootCmd.InitDefaultCompletionCmd()
	for _, sub := range rootCmd.Commands() {
		if sub.Name() == "completion" {
			sub.AddCommand(completionInstallCmd(rootCmd))
			break
		}
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Failed to execute command: %v", err)
	}

	if exitCode != exitOK {
		os.Exit(exitCode)
	}
}

// runPreflight boots rotki-core, waits for the API, and verifies every required
// endpoint is registered. It returns exitContractBreak when a route is missing
// and exitOK when all are present.
func runPreflight(cfg *config.Config) int {
	logger.Init()
	cfg.SetBaseURL()

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration: %v", err)
	}

	rotki, err := process.StartRotkiCore(cfg.BinPath, cfg.Port, cfg.APIReadyTimeout, cfg.DataDir)
	if err != nil {
		logger.Fatal("Failed to start rotki-core: %v", err)
	}

	syncService := services.NewSyncService(cfg)
	defer syncService.Cleanup()

	if !syncService.WaitForAPIReady() {
		logger.Fatal("API failed to become ready")
	}

	preflightErr := syncService.PreflightEndpoints()
	stopRotki(rotki)

	if preflightErr != nil {
		logger.Error("Preflight failed: %v", preflightErr)
		alert.Notify("rotki-sync: preflight failed", preflightErr.Error())
		return exitContractBreak
	}

	logger.Info("Preflight succeeded: all required endpoints are present")
	return exitOK
}
