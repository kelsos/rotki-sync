package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kelsos/rotki-sync/internal/async"
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
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

// processUserData performs all data processing for a single user and records
// the outcome of each step into a UserReport. A non-nil second return is a
// fatal contract break (e.g. a removed endpoint) that aborts the remaining
// steps for this user and the whole run.
func (s *SyncService) processUserData(username string) (UserReport, error) {
	logger.Info("Starting data processing for user: %s", username)

	report := UserReport{Username: username}

	// Snapshot and exchange trades are single operations with no per-item count.
	report.add(StepReport{Step: "balance snapshot", Err: s.blockchain.PerformSnapshotIfNeeded()})

	detectStats, detectErr := s.blockchain.DetectTokens()
	report.add(StepReport{Step: "token detection", Stats: detectStats, Err: detectErr})

	report.add(StepReport{Step: "exchange trades", Err: s.exchange.GetExchangeTrades()})

	// The remaining steps loop over accounts/chains and can hit a removed
	// endpoint; a ContractBreakError from any of them aborts the run.
	steps := []struct {
		name string
		core bool
		run  func() (OpStats, error)
	}{
		{"online events fetch", false, s.blockchain.FetchOnlineEvents},
		{"EVM transaction fetch", true, s.blockchain.FetchEvmTransactions},
		{"non-EVM transaction fetch", true, s.blockchain.FetchNonEvmTransactions},
		{"EVM transaction decode", true, s.blockchain.DecodeEvmTransactions},
		{"non-EVM transaction decode", true, s.blockchain.DecodeNonEvmTransactions},
	}

	for _, step := range steps {
		stats, err := step.run()
		report.add(StepReport{Step: step.name, Core: step.core, Stats: stats, Err: err})

		var contractBreak *ContractBreakError
		if errors.As(err, &contractBreak) {
			logger.Error("Aborting run for user %s: %v", username, err)
			return report, contractBreak
		}
		if err != nil {
			logger.Error("Failed %s: %v", step.name, err)
		}
	}

	logger.Info("Completed data processing for user: %s", username)
	return report, nil
}

// ProcessAllUsers processes all users in the system and returns an aggregated
// run report. The returned error is a transport/setup failure that prevented
// processing; per-step and contract-break outcomes are carried in the report.
func (s *SyncService) ProcessAllUsers() (*RunReport, error) {
	report := &RunReport{}

	err := s.user.ProcessUsers(func(username string) error {
		// Once a contract break has aborted the run, skip the remaining users:
		// the same broken endpoint would fail for every one of them.
		if report.FatalErr != nil {
			logger.Warn("Skipping user %s after contract break", username)
			return nil
		}

		userReport, fatal := s.processUserData(username)
		report.add(userReport)
		if fatal != nil {
			report.FatalErr = fatal
		}
		return nil
	})

	return report, err
}

// requiredEndpoints lists the routes a sync run depends on. The preflight
// probes each so a removed or renamed route is caught at startup rather than
// surfacing as a 404 mid-run (the failure mode that silently dropped a month of
// EVM transactions after a unified-API migration).
var requiredEndpoints = []string{
	evmTransactionsEndpoint,    // /blockchains/transactions
	transactionsDecodeEndpoint, // /blockchains/transactions/decode
	"/blockchains/supported",
	"/history/events/query",
	"/balances",
	"/tasks",
}

// PreflightEndpoints verifies that every endpoint the CLI depends on is still
// registered on the backend. It returns an error naming the missing routes so a
// contract break (e.g. after a rotki-core upgrade) is caught before any work is
// done. The probe is side-effect free: it issues OPTIONS and only flags a 404.
func (s *SyncService) PreflightEndpoints() error {
	var missing []string
	for _, endpoint := range requiredEndpoints {
		exists, err := s.client.EndpointExists(endpoint)
		if err != nil {
			return fmt.Errorf("preflight probe for %s failed: %w", endpoint, err)
		}
		if !exists {
			logger.Error("Preflight: required endpoint %s is missing (404)", endpoint)
			missing = append(missing, endpoint)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required endpoints missing from backend (contract break): %s",
			strings.Join(missing, ", "))
	}
	logger.Info("Endpoint preflight passed (%d endpoints)", len(requiredEndpoints))
	return nil
}

// WaitForAPIReady waits for the API to become ready
func (s *SyncService) WaitForAPIReady() bool {
	return s.client.WaitForAPIReady()
}

// GetInfo fetches general information about the running rotki backend,
// including the version and data directory.
func (s *SyncService) GetInfo() (*models.Info, error) {
	var response models.InfoResponse
	if err := s.client.Get("/info", &response); err != nil {
		return nil, fmt.Errorf("failed to get rotki info: %w", err)
	}
	return &response.Result, nil
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
	onLoginResult func(username string, loginErr error),
	processFunc func(username string) error,
	onLogout func(username string) error,
) error {
	return s.user.ProcessUsersWithCallback(onLoginResult, processFunc, onLogout)
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
	_, err := s.blockchain.FetchOnlineEvents()
	return err
}

// FetchEvmTransactions fetches EVM transactions
func (s *SyncService) FetchEvmTransactions() error {
	_, err := s.blockchain.FetchEvmTransactions()
	return err
}

// DecodeEvmTransactions decodes EVM transactions
func (s *SyncService) DecodeEvmTransactions() error {
	_, err := s.blockchain.DecodeEvmTransactions()
	return err
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

// DetectTokens runs token detection on EVM chains
func (s *SyncService) DetectTokens() error {
	_, err := s.blockchain.DetectTokens()
	return err
}

// GetTokenDetectionChains returns EVM chains with addresses for token detection
func (s *SyncService) GetTokenDetectionChains() ([]TokenDetectionChain, error) {
	return s.blockchain.GetTokenDetectionChains()
}

// DetectTokensForAddress runs token detection for a single address on a chain
func (s *SyncService) DetectTokensForAddress(chainID string, address string) error {
	return s.blockchain.DetectTokensForAddress(chainID, address)
}

// GetCachedTokenDetection fetches cached token detection info for the given
// addresses on a chain without triggering a fresh detection.
func (s *SyncService) GetCachedTokenDetection(chainID string, addresses []string) (models.TokenDetectResponse, error) {
	return s.blockchain.GetCachedTokenDetection(chainID, addresses)
}

// ShouldSkipTokenDetection reports whether token detection can be skipped for
// an address based on its cached info.
func (s *SyncService) ShouldSkipTokenDetection(info models.TokenDetectAddressInfo) (bool, time.Duration) {
	return s.blockchain.ShouldSkipTokenDetection(info)
}

// FetchNonEvmTransactions fetches transactions for non-EVM chains
func (s *SyncService) FetchNonEvmTransactions() error {
	_, err := s.blockchain.FetchNonEvmTransactions()
	return err
}

// DecodeNonEvmTransactions decodes transactions for non-EVM chains
func (s *SyncService) DecodeNonEvmTransactions() error {
	_, err := s.blockchain.DecodeNonEvmTransactions()
	return err
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
