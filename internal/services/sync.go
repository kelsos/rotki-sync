package services

import (
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/logger"
)

// SyncService orchestrates the data synchronization process
type SyncService struct {
	config     *config.Config
	client     *client.APIClient
	user       *UserService
	blockchain *BlockchainService
	exchange   *ExchangeService
}

// NewSyncService creates a new sync service with all dependencies
func NewSyncService(cfg *config.Config) *SyncService {
	apiClient := client.NewAPIClient(cfg)

	return &SyncService{
		config:     cfg,
		client:     apiClient,
		user:       NewUserService(apiClient),
		blockchain: NewBlockchainService(apiClient),
		exchange:   NewExchangeService(apiClient),
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
