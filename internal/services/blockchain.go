package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/kelsos/rotki-sync/internal/async"
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
)

// excludedChains contains chains that should be excluded from EVM operations
// (transaction fetch, decode, and token detection).
//
// avalanche is excluded because rotki-core does not support transaction/history
// retrieval for it: accounts are still enumerated for balances, but there is no
// transaction source to fetch or decode, so attempting those operations is a
// no-op at best. This was verified against a live run (avalanche accounts are
// found but have no tx history path).
var excludedChains = map[string]bool{
	"avalanche": true,
	// Add other chains to exclude here if needed in the future
}

// isChainExcluded checks if a chain should be excluded from EVM operations
func isChainExcluded(chainName string) bool {
	return excludedChains[chainName]
}

// BlockchainService handles blockchain-related operations
type BlockchainService struct {
	client      *client.APIClient
	asyncClient *async.Client
}

// NewBlockchainServiceWithAsyncClient creates a new blockchain service with an async client
func NewBlockchainServiceWithAsyncClient(client *client.APIClient, asyncClient *async.Client) *BlockchainService {
	return &BlockchainService{
		client:      client,
		asyncClient: asyncClient,
	}
}

// nonDecodableChainTypes contains chain types that don't support transaction decoding
var nonDecodableChainTypes = map[string]bool{
	models.ChainTypeBitcoin: true,
}

// nonEvmChainTypes lists the chain types considered non-EVM for transaction fetching
var nonEvmChainTypes = []string{
	models.ChainTypeEvmLike,
	models.ChainTypeBitcoin,
	models.ChainTypeSolana,
}

// GetSupportedChainsByType retrieves supported chains filtered by type
func (s *BlockchainService) GetSupportedChainsByType(chainType string) ([]models.Blockchain, error) {
	var response models.BlockchainResponse
	if err := s.client.Get("/blockchains/supported", &response); err != nil {
		return nil, fmt.Errorf("failed to get supported chains for type %s: %w", chainType, err)
	}

	var chains []models.Blockchain
	for _, blockchain := range response.Result {
		if blockchain.Type == chainType {
			chains = append(chains, blockchain)
		}
	}

	return chains, nil
}

// GetSupportedEvmChains retrieves supported EVM chains
func (s *BlockchainService) GetSupportedEvmChains() ([]models.Blockchain, error) {
	return s.GetSupportedChainsByType(models.ChainTypeEvm)
}

// FetchAccountsForChains retrieves accounts for the given chains
func (s *BlockchainService) FetchAccountsForChains(chains []models.Blockchain) ([]models.ChainAccount, error) {
	var allAccounts []models.ChainAccount

	for _, chain := range chains {
		logger.Info("Fetching accounts for chain: %s", chain.Name)

		endpoint := fmt.Sprintf("/blockchains/%s/accounts", chain.ID)
		var response models.AccountsResponse

		if err := s.client.Get(endpoint, &response); err != nil {
			logger.Error("Failed to fetch accounts for chain %s: %v", chain.Name, err)
			continue
		}

		for _, account := range response.Result {
			chainAccount := models.ChainAccount{
				Address:    account.Address,
				EvmChain:   chain.EvmChainName,
				ChainID:    chain.ID,
				Blockchain: chain.ID,
				ChainType:  chain.Type,
			}
			allAccounts = append(allAccounts, chainAccount)
		}

		logger.Info("Found %d accounts for chain %s", len(response.Result), chain.Name)
	}

	return allAccounts, nil
}

// FetchAccounts retrieves accounts for all EVM chains
func (s *BlockchainService) FetchAccounts() ([]models.ChainAccount, error) {
	evmChains, err := s.GetSupportedEvmChains()
	if err != nil {
		return nil, err
	}

	return s.FetchAccountsForChains(evmChains)
}

// evmTransactionsEndpoint is the unified transactions route the desktop app
// uses for every chain type. The legacy /blockchains/evm/transactions route was
// removed when rotki migrated to this unified API.
const evmTransactionsEndpoint = "/blockchains/transactions"

// FetchEvmTransactions fetches EVM transactions for all accounts through the
// unified transactions endpoint. It returns per-account ok/failed counts; a
// removed endpoint (404) aborts the run with a ContractBreakError rather than
// being retried once per account.
func (s *BlockchainService) FetchEvmTransactions() (OpStats, error) {
	logger.Info("Starting EVM transaction fetch...")

	var stats OpStats

	chainAccounts, err := s.FetchAccounts()
	if err != nil {
		return stats, fmt.Errorf("failed to fetch accounts: %w", err)
	}

	logger.Info("Found %d total accounts across all chains", len(chainAccounts))

	// Group accounts by chain for efficient processing (exclude problematic chains)
	accountsByChain := make(map[string][]models.ChainAccount)
	for _, account := range chainAccounts {
		if !isChainExcluded(account.EvmChain) {
			accountsByChain[account.EvmChain] = append(accountsByChain[account.EvmChain], account)
		}
	}
	logger.Debug("Grouped accounts into %d unique chains (excluding problematic chains)", len(accountsByChain))

	for chainID, accounts := range accountsByChain {
		logger.Info("Processing %d accounts for chain %s", len(accounts), chainID)

		// Sort accounts alphabetically by address for consistent processing order
		sort.Slice(accounts, func(i, j int) bool {
			return accounts[i].Address < accounts[j].Address
		})

		for _, account := range accounts {
			if err := s.GetAccountTransactions(account); err != nil {
				if client.IsEndpointMissing(err) {
					return stats, &ContractBreakError{
						Step:     "EVM transaction fetch",
						Endpoint: evmTransactionsEndpoint,
						Err:      err,
					}
				}
				stats.Failed++
				logger.Error("Failed to get transactions for account %s on chain %s: %v",
					account.Address, account.EvmChain, err)
				continue
			}
			stats.Ok++
		}
	}

	logger.Info("Completed EVM transaction fetch (%d ok / %d failed)", stats.Ok, stats.Failed)
	return stats, nil
}

// GetAccountTransactions fetches transactions for a specific account through the
// unified endpoint, using the chain id (e.g. "ethereum", "optimism") as the
// account's blockchain — the same shape the non-EVM path and the desktop app
// send.
func (s *BlockchainService) GetAccountTransactions(account models.ChainAccount) error {
	logger.Debug("Fetching transactions for %s (%s)", account.Blockchain, account.Address)

	requestData := models.TransactionsRequest{
		Accounts: []models.TransactionAccount{
			{
				Address:    account.Address,
				Blockchain: account.Blockchain,
			},
		},
	}

	// Use async for fetching transactions
	response, err := async.Post[bool](s.asyncClient, evmTransactionsEndpoint, requestData)
	if err != nil {
		return fmt.Errorf("failed to fetch transactions for %s on chain %s: %w", account.Address, account.Blockchain, err)
	}
	if response == nil {
		return fmt.Errorf("received nil response for transactions of %s on chain %s", account.Address, account.Blockchain)
	}

	return nil
}

// transactionsDecodeEndpoint is the unified decode route shared by every chain
// type. The legacy /blockchains/evm/transactions/decode route was removed in
// the unified-API migration.
const transactionsDecodeEndpoint = "/blockchains/transactions/decode"

// DecodeEvmTransactions decodes EVM transactions for each supported chain
// through the unified decode endpoint, one chain id per request. It returns
// per-chain ok/failed counts; a removed endpoint (404) aborts with a
// ContractBreakError.
func (s *BlockchainService) DecodeEvmTransactions() (OpStats, error) {
	var stats OpStats

	evmChains, err := s.GetSupportedEvmChains()
	if err != nil {
		return stats, fmt.Errorf("failed to get EVM chains: %w", err)
	}

	// Decode by chain id, skipping chains without an EVM chain name and
	// excluded chains.
	chainIDs := make([]string, 0)
	for _, chain := range evmChains {
		if chain.EvmChainName != "" && !isChainExcluded(chain.EvmChainName) {
			chainIDs = append(chainIDs, chain.ID)
		}
	}

	logger.Info("Found %d EVM chains for transaction decoding", len(chainIDs))

	for _, chainID := range chainIDs {
		logger.Debug("Decoding transactions for chain %s", chainID)

		requestData := models.TransactionDecodeRequest{
			Chain: chainID,
		}

		response, err := async.Post[models.TransactionDecodeResult](s.asyncClient, transactionsDecodeEndpoint, requestData)
		if err != nil {
			if client.IsEndpointMissing(err) {
				return stats, &ContractBreakError{
					Step:     "EVM transaction decode",
					Endpoint: transactionsDecodeEndpoint,
					Err:      err,
				}
			}
			stats.Failed++
			logger.Error("Failed to decode transactions for chain %s: %v", chainID, err)
			continue
		}
		if response == nil {
			stats.Failed++
			logger.Error("Received nil response for decoding transactions on chain %s", chainID)
			continue
		}

		stats.Ok++
		if decoded := response.Result.DecodedTxNumber; decoded > 0 {
			logger.Info("Decoded %d transactions for chain %s", decoded, chainID)
		}
	}

	return stats, nil
}

// TokenDetectionChain holds a chain's ID, name, and the addresses to detect tokens for
type TokenDetectionChain struct {
	ChainID   string
	ChainName string
	Addresses []string
}

// GetTokenDetectionChains returns EVM chains with their addresses for token detection
func (s *BlockchainService) GetTokenDetectionChains() ([]TokenDetectionChain, error) {
	evmChains, err := s.GetSupportedEvmChains()
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM chains for token detection: %w", err)
	}

	accounts, err := s.FetchAccountsForChains(evmChains)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch accounts for token detection: %w", err)
	}

	// Build lookup for chain ID -> chain info
	chainInfo := make(map[string]models.Blockchain)
	for _, chain := range evmChains {
		chainInfo[chain.ID] = chain
	}

	// Group addresses by chain ID, skipping excluded chains
	addressesByChain := make(map[string][]string)
	for _, acc := range accounts {
		chain := chainInfo[acc.ChainID]
		if chain.EvmChainName == "" || isChainExcluded(chain.EvmChainName) {
			continue
		}
		addressesByChain[acc.ChainID] = append(addressesByChain[acc.ChainID], acc.Address)
	}

	var result []TokenDetectionChain
	for chainID, addresses := range addressesByChain {
		result = append(result, TokenDetectionChain{
			ChainID:   chainID,
			ChainName: chainInfo[chainID].Name,
			Addresses: addresses,
		})
	}

	// Sort for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ChainName < result[j].ChainName
	})

	return result, nil
}

// tokenDetectionMaxAge is the maximum age of a cached token detection before
// a fresh detection is triggered.
const tokenDetectionMaxAge = 5 * 24 * time.Hour

// shouldSkipTokenDetection reports whether token detection can be skipped for
// an address based on its cached info. It returns the cached entry's age so
// callers can include it in skip logs.
func shouldSkipTokenDetection(info models.TokenDetectAddressInfo, now time.Time, maxAge time.Duration) (bool, time.Duration) {
	if info.LastUpdateTimestamp <= 0 {
		return false, 0
	}
	age := now.Sub(time.Unix(info.LastUpdateTimestamp, 0))
	return age < maxAge, age
}

// ShouldSkipTokenDetection reports whether token detection can be skipped for
// an address based on its cached info, using the package's max-age threshold.
// It returns the cached entry's age so callers can include it in skip logs.
func (s *BlockchainService) ShouldSkipTokenDetection(info models.TokenDetectAddressInfo) (bool, time.Duration) {
	return shouldSkipTokenDetection(info, time.Now(), tokenDetectionMaxAge)
}

// DetectTokensForAddress runs token detection on a single chain for a single address
func (s *BlockchainService) DetectTokensForAddress(chainID string, address string) error {
	endpoint := fmt.Sprintf("/blockchains/%s/tokens/detect", chainID)
	requestData := models.TokenDetectRequest{
		Addresses: []string{address},
	}

	_, err := async.Post[json.RawMessage](s.asyncClient, endpoint, requestData)
	if err != nil {
		return fmt.Errorf("failed to detect tokens for %s on %s: %w", address, chainID, err)
	}

	return nil
}

// GetCachedTokenDetection fetches cached token detection info for the given
// addresses on a chain without triggering a fresh detection.
func (s *BlockchainService) GetCachedTokenDetection(chainID string, addresses []string) (models.TokenDetectResponse, error) {
	endpoint := fmt.Sprintf("/blockchains/%s/tokens/detect", chainID)
	requestData := models.TokenDetectRequest{
		Addresses: addresses,
		OnlyCache: true,
	}

	resp, err := async.Post[models.TokenDetectResponse](s.asyncClient, endpoint, requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to query cached token detection on %s: %w", chainID, err)
	}

	return resp.Result, nil
}

// DetectTokens runs token detection on EVM chains (excluding avalanche).
// Per-address detection is skipped when a cached detection younger than
// tokenDetectionMaxAge exists.
func (s *BlockchainService) DetectTokens() (OpStats, error) {
	var stats OpStats

	chains, err := s.GetTokenDetectionChains()
	if err != nil {
		return stats, err
	}

	for _, chain := range chains {
		cached, err := s.GetCachedTokenDetection(chain.ChainID, chain.Addresses)
		if err != nil {
			logger.Error("Failed to query token detection cache on %s, will run detection: %v", chain.ChainName, err)
			cached = nil
		}

		for _, address := range chain.Addresses {
			if skip, age := shouldSkipTokenDetection(cached[address], time.Now(), tokenDetectionMaxAge); skip {
				logger.Info("Skipping token detection for %s on %s: last detection %s ago (< %s)",
					address, chain.ChainName, age.Round(time.Hour), tokenDetectionMaxAge)
				continue
			}

			logger.Info("Detecting tokens for %s on %s", address, chain.ChainName)

			if err := s.DetectTokensForAddress(chain.ChainID, address); err != nil {
				stats.Failed++
				logger.Error("Failed to detect tokens for %s on %s: %v", address, chain.ChainName, err)
				continue
			}

			stats.Ok++
			logger.Info("Token detection completed for %s on %s", address, chain.ChainName)
		}
	}

	return stats, nil
}

// FetchNonEvmTransactions fetches transactions for non-EVM chain types. It
// returns per-account ok/failed counts; a removed endpoint (404) aborts with a
// ContractBreakError.
func (s *BlockchainService) FetchNonEvmTransactions() (OpStats, error) {
	var stats OpStats

	for _, chainType := range nonEvmChainTypes {
		chains, err := s.GetSupportedChainsByType(chainType)
		if err != nil {
			logger.Error("Failed to get supported %s chains: %v", chainType, err)
			continue
		}

		if len(chains) == 0 {
			continue
		}

		accounts, err := s.FetchAccountsForChains(chains)
		if err != nil {
			logger.Error("Failed to fetch accounts for %s chains: %v", chainType, err)
			continue
		}

		logger.Info("Fetching %s transactions for %d accounts", chainType, len(accounts))

		for _, account := range accounts {
			requestData := models.TransactionsRequest{
				Accounts: []models.TransactionAccount{
					{
						Address:    account.Address,
						Blockchain: account.Blockchain,
					},
				},
			}

			_, err := async.Post[bool](s.asyncClient, evmTransactionsEndpoint, requestData)
			if err != nil {
				if client.IsEndpointMissing(err) {
					return stats, &ContractBreakError{
						Step:     "non-EVM transaction fetch",
						Endpoint: evmTransactionsEndpoint,
						Err:      err,
					}
				}
				stats.Failed++
				logger.Error("Failed to fetch transactions for %s on %s: %v",
					account.Address, account.Blockchain, err)
				continue
			}
			stats.Ok++
		}

		logger.Info("Completed %s transaction fetch", chainType)
	}

	return stats, nil
}

// DecodeNonEvmTransactions decodes transactions for non-EVM chain types that
// support decoding. It returns per-chain ok/failed counts; a removed endpoint
// (404) aborts with a ContractBreakError.
func (s *BlockchainService) DecodeNonEvmTransactions() (OpStats, error) {
	var stats OpStats

	for _, chainType := range nonEvmChainTypes {
		if nonDecodableChainTypes[chainType] {
			continue
		}

		chains, err := s.GetSupportedChainsByType(chainType)
		if err != nil {
			logger.Error("Failed to get supported %s chains for decoding: %v", chainType, err)
			continue
		}

		for _, chain := range chains {
			logger.Debug("Decoding %s transactions for chain %s", chainType, chain.ID)

			requestData := models.TransactionDecodeRequest{
				Chain: chain.ID,
			}

			_, err := async.Post[models.TransactionDecodeResult](s.asyncClient, transactionsDecodeEndpoint, requestData)
			if err != nil {
				if client.IsEndpointMissing(err) {
					return stats, &ContractBreakError{
						Step:     "non-EVM transaction decode",
						Endpoint: transactionsDecodeEndpoint,
						Err:      err,
					}
				}
				stats.Failed++
				logger.Error("Failed to decode transactions for chain %s: %v", chain.ID, err)
				continue
			}

			stats.Ok++
			logger.Info("Decoded transactions for %s chain %s", chainType, chain.ID)
		}
	}

	return stats, nil
}

// FetchOnlineEvents fetches online events. It returns per-query ok/failed
// counts. Each query type is gated on whether its integration is set up:
// gnosis_pay and monerium only when their credentials are configured, and the
// eth2-specific queries (block_productions, eth_withdrawals) only when the eth2
// module is active. Gating avoids a recurring per-run failure from querying an
// integration the user has not enabled.
func (s *BlockchainService) FetchOnlineEvents() (OpStats, error) {
	logger.Info("Fetching online events")

	var stats OpStats

	// gnosis_pay and monerium are independent integrations, not part of eth2;
	// include them only when configured.
	queryTypes := make([]models.QueryType, 0, 4)
	queryTypes = s.appendIfConfigured(queryTypes, "gnosis_pay", models.GnosisPayQuery, s.isGnosisPayConfigured)
	queryTypes = s.appendIfConfigured(queryTypes, "monerium", models.MoneriumQuery, s.isMoneriumConfigured)

	// Check if eth2 module is activated before adding its query types.
	isEth2Active, err := s.IsEth2ModuleActive()
	if err != nil {
		logger.Error("Failed to check eth2 module status: %v", err)
		return stats, fmt.Errorf("failed to check eth2 module status: %w", err)
	}
	if isEth2Active {
		queryTypes = append(queryTypes, models.BlockProductionsQuery, models.EthWithdrawalsQuery)
	} else {
		logger.Info("Eth2 module is not active, skipping eth2 online events")
	}

	if len(queryTypes) == 0 {
		logger.Info("No online-event integrations are configured; nothing to fetch")
		return stats, nil
	}

	for _, queryType := range queryTypes {
		logger.Info("Fetching %s events", queryType)

		requestData := models.EventsQueryPayload{
			QueryType: queryType,
		}

		// Use async for fetching history events
		response, err := async.Post[bool](s.asyncClient, "/history/events/query", requestData)
		if err != nil {
			if client.IsEndpointMissing(err) {
				return stats, &ContractBreakError{
					Step:     "online events fetch",
					Endpoint: "/history/events/query",
					Err:      err,
				}
			}
			stats.Failed++
			logger.Error("Failed to fetch %s events: %v", queryType, err)
			continue
		}
		if response == nil {
			stats.Failed++
			logger.Error("Received nil response for %s events", queryType)
			continue
		}

		stats.Ok++
		if response.Result {
			logger.Info("Successfully fetched %s events", queryType)
		}
	}

	return stats, nil
}

// appendIfConfigured adds query to queryTypes when check reports the integration
// named name as configured. If the check itself fails the query is still added
// (fail-loud: a transient status-check error must not silently drop a
// configured integration); when not configured it is skipped with an info log.
func (s *BlockchainService) appendIfConfigured(
	queryTypes []models.QueryType,
	name string,
	query models.QueryType,
	check func() (bool, error),
) []models.QueryType {
	configured, err := check()
	if err != nil {
		logger.Debug("Could not determine %s status, attempting anyway: %v", name, err)
		return append(queryTypes, query)
	}
	if !configured {
		logger.Info("%s is not configured, skipping its online events", name)
		return queryTypes
	}
	return append(queryTypes, query)
}

// isMoneriumConfigured reports whether Monerium OAuth credentials are present,
// via GET /services/monerium ({"result": {"authenticated": bool}}).
func (s *BlockchainService) isMoneriumConfigured() (bool, error) {
	var response map[string]interface{}
	if err := s.client.Get("/services/monerium", &response); err != nil {
		return false, fmt.Errorf("failed to get monerium status: %w", err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return false, nil
	}
	authenticated, _ := result["authenticated"].(bool)
	return authenticated, nil
}

// isGnosisPayConfigured reports whether Gnosis Pay credentials are present. The
// GET /external_services result is a map keyed by configured service name, so
// the presence of a "gnosis_pay" key means credentials are stored.
func (s *BlockchainService) isGnosisPayConfigured() (bool, error) {
	var response map[string]interface{}
	if err := s.client.Get("/external_services", &response); err != nil {
		return false, fmt.Errorf("failed to get external services: %w", err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return false, nil
	}
	_, configured := result["gnosis_pay"]
	return configured, nil
}

// Balance-related methods

// FetchExchangeRate fetches exchange rate for a currency
func (s *BlockchainService) FetchExchangeRate(currency string) (float64, error) {
	endpoint := fmt.Sprintf("/exchange_rates?currencies=%s", currency)
	var response map[string]interface{}

	if err := s.client.Get(endpoint, &response); err != nil {
		return 0, fmt.Errorf("failed to fetch exchange rate for %s: %w", currency, err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid response format for exchange rate")
	}

	// Check if the currency key exists and what type it is
	if currencyData, exists := result[currency]; exists {
		// Try different possible formats
		switch v := currencyData.(type) {
		case float64:
			return v, nil
		case string:
			// Parse string to float64
			if rate, parseErr := strconv.ParseFloat(v, 64); parseErr == nil {
				return rate, nil
			} else {
				return 0, fmt.Errorf("failed to parse exchange rate string %s for %s: %w", v, currency, parseErr)
			}
		case map[string]interface{}:
			// Check if there's a nested structure
			if rate, ok := v["rate"].(float64); ok {
				return rate, nil
			}
			if rate, ok := v["value"].(float64); ok {
				return rate, nil
			}
		}
	}

	return 0, fmt.Errorf("exchange rate for %s not found in response", currency)
}

// GetLastBalanceSave gets the timestamp of the last balance save
func (s *BlockchainService) GetLastBalanceSave() (int64, error) {
	var response map[string]interface{}
	if err := s.client.Get("/periodic", &response); err != nil {
		return 0, fmt.Errorf("failed to get last balance save: %w", err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid response format for periodic data")
	}

	timestamp, ok := result["last_balance_save"].(float64)
	if !ok {
		return 0, fmt.Errorf("last_balance_save not found in response")
	}

	return int64(timestamp), nil
}

// GetBalanceSaveFrequency gets the balance save frequency setting
func (s *BlockchainService) GetBalanceSaveFrequency() (int, error) {
	var response map[string]interface{}
	if err := s.client.Get("/settings", &response); err != nil {
		return 0, fmt.Errorf("failed to get settings: %w", err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid response format for settings")
	}

	frequency, ok := result["balance_save_frequency"].(float64)
	if !ok {
		return 0, fmt.Errorf("balance_save_frequency not found in settings")
	}

	return int(frequency), nil
}

// IsEth2ModuleActive checks if the eth2 module is active in settings
func (s *BlockchainService) IsEth2ModuleActive() (bool, error) {
	var response map[string]interface{}
	if err := s.client.Get("/settings", &response); err != nil {
		return false, fmt.Errorf("failed to get settings: %w", err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid response format for settings")
	}

	activeModules, ok := result["active_modules"].([]interface{})
	if !ok {
		logger.Debug("No active_modules found in settings")
		return false, nil
	}

	for _, module := range activeModules {
		if moduleStr, ok := module.(string); ok && moduleStr == "eth2" {
			return true, nil
		}
	}

	return false, nil
}

// TakeBalanceSnapshot takes a balance snapshot
func (s *BlockchainService) TakeBalanceSnapshot(forceSnapshot bool) error {
	query := ""
	if forceSnapshot {
		query = "?save_data=true"
	}

	endpoint := fmt.Sprintf("/balances%s", query)

	// Use async for balance snapshot
	response, err := async.Get[map[string]interface{}](s.asyncClient, endpoint)
	if err != nil {
		return fmt.Errorf("failed to take balance snapshot: %w", err)
	}
	if response == nil {
		return fmt.Errorf("received nil response for balance snapshot")
	}

	// Fetch EUR exchange rate
	euroRate, err := s.FetchExchangeRate("EUR")
	if err != nil {
		logger.Error("Failed to fetch EUR exchange rate: %v", err)
	} else {
		logger.Debug("Current EUR exchange rate: %.6f", euroRate)
	}

	logger.Info("Balance snapshot completed successfully")
	return nil
}

// PerformSnapshotIfNeeded performs a balance snapshot if enough time has elapsed
func (s *BlockchainService) PerformSnapshotIfNeeded() error {
	lastBalanceSave, err := s.GetLastBalanceSave()
	if err != nil {
		return fmt.Errorf("failed to get last balance save: %w", err)
	}

	balanceSaveFrequency, err := s.GetBalanceSaveFrequency()
	if err != nil {
		return fmt.Errorf("failed to get balance save frequency: %w", err)
	}

	currentTime := time.Now().Unix()
	timeSinceLastSave := currentTime - lastBalanceSave
	requiredInterval := int64(balanceSaveFrequency * 3600) // Convert hours to seconds

	enoughTimeElapsed := timeSinceLastSave >= requiredInterval/2
	requiredTimeElapsed := timeSinceLastSave >= requiredInterval

	logger.Info("Time since last balance save: %d seconds (required: %d)", timeSinceLastSave, requiredInterval)

	if enoughTimeElapsed {
		if err := s.TakeBalanceSnapshot(enoughTimeElapsed && !requiredTimeElapsed); err != nil {
			return fmt.Errorf("failed to take balance snapshot: %w", err)
		}
		logger.Info("Balance snapshot completed")
	} else {
		logger.Info("Skipping balance snapshot - not enough time elapsed")
	}

	return nil
}
