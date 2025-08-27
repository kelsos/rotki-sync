package services

import (
	"github.com/kelsos/rotki-sync/internal/async"
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/logger"
)

// SyncService orchestrates the data synchronization process
type SyncService struct {
	config      *config.Config
	client      *client.APIClient
	taskManager *async.TaskManager
	asyncClient *async.Client
	user        *UserService
	blockchain  *BlockchainService
	exchange    *ExchangeService
}

// NewSyncService creates a new sync service with all dependencies
func NewSyncService(cfg *config.Config) *SyncService {
	apiClient := client.NewAPIClient(cfg)
	taskManager := async.NewTaskManager(apiClient)
	asyncClient := async.NewClient(taskManager)

	return &SyncService{
		config:      cfg,
		client:      apiClient,
		taskManager: taskManager,
		asyncClient: asyncClient,
		user:        NewUserServiceWithAsyncClient(apiClient, asyncClient),
		blockchain:  NewBlockchainServiceWithAsyncClient(apiClient, asyncClient),
		exchange:    NewExchangeServiceWithAsyncClient(apiClient, asyncClient),
	}
}

// ProcessUserData performs all data processing for a single user
func (s *SyncService) ProcessUserData(username string) error {
	logger.Info("Starting data processing for user: %s", username)

	// Perform snapshot if needed
	if err := s.blockchain.PerformSnapshotIfNeeded(); err != nil {
		logger.Error("Failed to perform snapshot: %v", err)
	}

	// Fetch exchange trades
	if err := s.exchange.GetExchangeTrades(); err != nil {
		logger.Error("Failed fetch exchange trades: %v", err)
	}

	// Fetch online events
	if err := s.blockchain.FetchOnlineEvents(); err != nil {
		logger.Error("Failed to fetch online events: %v", err)
	}

	// Fetch EVM transactions
	if err := s.blockchain.FetchEvmTransactions(0, 0); err != nil {
		logger.Error("Failed to fetch EVM transactions: %v", err)
	}

	// Decode EVM transactions
	if err := s.blockchain.DecodeEvmTransactions(); err != nil {
		logger.Error("Failed to decode EVM transactions: %v", err)
	}

	logger.Info("Completed data processing for user: %s", username)
	return nil
}

// ProcessAllUsers processes all users in the system
func (s *SyncService) ProcessAllUsers() error {
	return s.user.ProcessUsers(s.ProcessUserData)
}

// WaitForAPIReady waits for the API to become ready
func (s *SyncService) WaitForAPIReady() bool {
	return s.client.WaitForAPIReady()
}

// GetConfig returns the current configuration
func (s *SyncService) GetConfig() *config.Config {
	return s.config
}

// Cleanup performs cleanup operations including stopping the task manager
func (s *SyncService) Cleanup() {
	if s.taskManager != nil {
		s.taskManager.Stop()
	}
}

// GetUsers retrieves all users from the system
func (s *SyncService) GetUsers() ([]string, error) {
	return s.user.GetUsers()
}

// ProcessUsersWithCallback processes all users with callbacks for monitoring
func (s *SyncService) ProcessUsersWithCallback(
	onLogin func(username string) error,
	processFunc func(username string) error,
	onLogout func(username string) error,
) error {
	return s.user.ProcessUsersWithCallback(onLogin, processFunc, onLogout)
}

// PerformSnapshotIfNeeded performs a blockchain snapshot if needed
func (s *SyncService) PerformSnapshotIfNeeded() error {
	return s.blockchain.PerformSnapshotIfNeeded()
}

// GetExchangeTrades fetches exchange trades
func (s *SyncService) GetExchangeTrades() error {
	return s.exchange.GetExchangeTrades()
}

// FetchOnlineEvents fetches online blockchain events
func (s *SyncService) FetchOnlineEvents() error {
	return s.blockchain.FetchOnlineEvents()
}

// FetchEvmTransactions fetches EVM transactions
func (s *SyncService) FetchEvmTransactions(fromTimestamp, toTimestamp int64) error {
	return s.blockchain.FetchEvmTransactions(fromTimestamp, toTimestamp)
}

// DecodeEvmTransactions decodes EVM transactions
func (s *SyncService) DecodeEvmTransactions() error {
	return s.blockchain.DecodeEvmTransactions()
}

// FetchAccounts retrieves all accounts for all chains
func (s *SyncService) FetchAccounts() ([]interface{}, error) {
	accounts, err := s.blockchain.FetchAccounts()
	if err != nil {
		return nil, err
	}

	// Convert to interface{} slice for TUI compatibility
	result := make([]interface{}, len(accounts))
	for i, acc := range accounts {
		result[i] = acc
	}
	return result, nil
}

// GetSupportedEvmChains retrieves supported EVM chains
func (s *SyncService) GetSupportedEvmChains() ([]interface{}, error) {
	chains, err := s.blockchain.GetSupportedEvmChains()
	if err != nil {
		return nil, err
	}

	// Convert to interface{} slice for TUI compatibility
	result := make([]interface{}, len(chains))
	for i, chain := range chains {
		result[i] = chain
	}
	return result, nil
}
