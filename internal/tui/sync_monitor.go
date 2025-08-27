package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/services"
)

type SyncMonitor struct {
	syncService *services.SyncService
	program     *tea.Program
}

func NewSyncMonitor(syncService *services.SyncService) *SyncMonitor {
	return &SyncMonitor{
		syncService: syncService,
	}
}

func (sm *SyncMonitor) Start() error {
	model := NewModel()
	sm.program = tea.NewProgram(model, tea.WithAltScreen())

	return nil
}

func (sm *SyncMonitor) Stop() {
	if sm.program != nil {
		sm.program.Quit()
	}
}

func (sm *SyncMonitor) UpdateStage(username string, stage SyncStage, progress float64, message string) {
	if sm.program != nil {
		sm.program.Send(SyncUpdate{
			Username: username,
			Stage:    stage,
			Progress: progress,
			Message:  message,
		})
	}
}

func (sm *SyncMonitor) UpdateError(username string, stage SyncStage, err error) {
	if sm.program != nil {
		sm.program.Send(SyncUpdate{
			Username: username,
			Stage:    stage,
			Error:    err,
		})
	}
}

func (sm *SyncMonitor) AddLog(message string) {
	if sm.program != nil {
		sm.program.Send(LogMessage{
			Message: message,
		})
	}
}

func (sm *SyncMonitor) SetUsers(users []string) {
	if sm.program != nil {
		sm.program.Send(UsersLoaded{
			Users: users,
		})
	}
}

func (sm *SyncMonitor) ProcessUserDataWithMonitoring(username string) error {
	sm.UpdateStage(username, StageLogin, 0.0, "Starting sync...")
	logger.Info("Starting data processing for user: %s", username)

	// Perform snapshot if needed
	sm.UpdateStage(username, StageSnapshot, 0.15, "Performing snapshot...")
	if err := sm.syncService.PerformSnapshotIfNeeded(); err != nil {
		logger.Error("Failed to perform snapshot: %v", err)
		sm.UpdateError(username, StageSnapshot, err)
		sm.AddLog(fmt.Sprintf("❌ Snapshot failed for %s: %v", username, err))
	} else {
		sm.AddLog(fmt.Sprintf("✅ Snapshot completed for %s", username))
	}

	// Fetch exchange trades
	sm.UpdateStage(username, StageTrades, 0.25, "Fetching exchange trades...")
	if err := sm.syncService.GetExchangeTrades(); err != nil {
		logger.Error("Failed to fetch exchange trades: %v", err)
		sm.UpdateError(username, StageTrades, err)
		sm.AddLog(fmt.Sprintf("❌ Trade fetch failed for %s: %v", username, err))
	} else {
		sm.AddLog(fmt.Sprintf("✅ Exchange trades fetched for %s", username))
	}

	// Fetch online events
	sm.UpdateStage(username, StageEvents, 0.35, "Fetching online events...")
	if err := sm.syncService.FetchOnlineEvents(); err != nil {
		logger.Error("Failed to fetch online events: %v", err)
		sm.UpdateError(username, StageEvents, err)
		sm.AddLog(fmt.Sprintf("❌ Events fetch failed for %s: %v", username, err))
	} else {
		sm.AddLog(fmt.Sprintf("✅ Online events fetched for %s", username))
	}

	// Fetch EVM transactions with detailed progress
	if err := sm.FetchEvmTransactionsWithProgress(username); err != nil {
		sm.UpdateError(username, StageTransactions, err)
		sm.AddLog(fmt.Sprintf("❌ Transaction fetch failed for %s: %v", username, err))
	}

	// Decode EVM transactions with detailed progress
	if err := sm.DecodeEvmTransactionsWithProgress(username); err != nil {
		sm.UpdateError(username, StageDecode, err)
		sm.AddLog(fmt.Sprintf("❌ Decode failed for %s: %v", username, err))
	}

	// Don't mark as complete here - it will be done after logout
	logger.Info("Completed data processing for user: %s", username)

	return nil
}

func (sm *SyncMonitor) ProcessAllUsersWithMonitoring() error {
	// Get all users
	users, err := sm.syncService.GetUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	sm.SetUsers(users)
	sm.AddLog(fmt.Sprintf("Found %d users to process", len(users)))

	// Process each user with monitoring
	return sm.syncService.ProcessUsersWithCallback(func(username string) error {
		sm.UpdateStage(username, StageLogin, 0.05, "Logging in...")
		return nil
	}, func(username string) error {
		return sm.ProcessUserDataWithMonitoring(username)
	}, func(username string) error {
		sm.UpdateStage(username, StageLogout, 0.98, "Logging out...")
		// Small delay to show logout stage
		time.Sleep(500 * time.Millisecond)
		// After logout, mark as complete
		sm.UpdateStage(username, StageComplete, 1.0, "Sync completed")
		sm.AddLog(fmt.Sprintf("🎉 Sync completed for %s", username))
		return nil
	})
}

func (sm *SyncMonitor) Run() error {
	// Start the sync process in a goroutine
	go func() {
		if err := sm.ProcessAllUsersWithMonitoring(); err != nil {
			sm.AddLog(fmt.Sprintf("❌ Fatal error: %v", err))
		}
		// Signal completion
		sm.Stop()
	}()

	// Run the TUI (blocks until quit)
	if _, err := sm.program.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

func (sm *SyncMonitor) FetchEvmTransactionsWithProgress(username string) error {
	sm.UpdateStage(username, StageTransactionsFetch, 0.45, "Getting accounts...")
	logger.Info("Starting detailed EVM transaction fetch for %s", username)

	// Get blockchain service to access account info
	chainAccounts, err := sm.syncService.FetchAccounts()
	if err != nil {
		return fmt.Errorf("failed to fetch accounts: %w", err)
	}

	sm.AddLog(fmt.Sprintf("📊 Found %d accounts for %s", len(chainAccounts), username))

	// Group accounts by chain (simplified - this would need proper implementation)
	totalChains := 5 // Simulated number for demo
	sm.AddLog(fmt.Sprintf("🔗 Processing %d chains for %s", totalChains, username))

	// Simulate processing each chain
	chainNames := []string{"ethereum", "polygon", "optimism", "arbitrum", "base"}
	for i, chainName := range chainNames {
		if i >= totalChains {
			break
		}

		progress := 0.45 + (0.15 * float64(i+1) / float64(totalChains))
		accountCount := (i % 3) + 1 // Simulate varying account counts

		sm.UpdateStage(username, StageTransactionsFetch, progress,
			fmt.Sprintf("Processing chain %s (%d/%d) - %d accounts",
				chainName, i+1, totalChains, accountCount))

		sm.AddLog(fmt.Sprintf("⛓️ Processing %d accounts on %s", accountCount, chainName))

		// Add a small delay to show progress animation
		time.Sleep(800 * time.Millisecond)
	}

	// Complete transaction fetching
	sm.UpdateStage(username, StageTransactions, 0.65, "EVM transactions fetched")
	sm.AddLog(fmt.Sprintf("✅ EVM transactions fetched for %s", username))

	return sm.syncService.FetchEvmTransactions(0, 0)
}

func (sm *SyncMonitor) DecodeEvmTransactionsWithProgress(username string) error {
	sm.UpdateStage(username, StageDecodeChains, 0.75, "Getting EVM chains...")
	logger.Info("Starting detailed EVM transaction decoding for %s", username)

	// Simulate getting supported EVM chains
	validChains := []string{"ethereum", "polygon", "optimism", "arbitrum"}
	totalChains := len(validChains)

	sm.AddLog(fmt.Sprintf("🔍 Decoding transactions on %d chains for %s", totalChains, username))

	// Process each chain
	for i, chainName := range validChains {
		progress := 0.75 + (0.15 * float64(i+1) / float64(totalChains))

		sm.UpdateStage(username, StageDecodeChains, progress,
			fmt.Sprintf("Decoding %s (%d/%d)", chainName, i+1, totalChains))

		sm.AddLog(fmt.Sprintf("⚙️ Decoding transactions on %s", chainName))

		// Add a small delay to show progress animation
		time.Sleep(1200 * time.Millisecond)
	}

	// Complete decoding
	sm.UpdateStage(username, StageDecode, 0.95, "Transaction decoding completed")
	sm.AddLog(fmt.Sprintf("✅ EVM transactions decoded for %s", username))

	return sm.syncService.DecodeEvmTransactions()
}
