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

// FetchEvmTransactions fetches EVM transactions for all accounts
func (s *BlockchainService) FetchEvmTransactions() error {
	logger.Info("Starting EVM transaction fetch...")

	chainAccounts, err := s.FetchAccounts()
	if err != nil {
		return fmt.Errorf("failed to fetch accounts: %w", err)
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
			err := s.GetAccountTransactions(account)
			if err != nil {
				logger.Error("Failed to get transactions for account %s on chain %s: %v",
					account.Address, account.EvmChain, err)
				continue
			}
		}
	}

	logger.Info("Completed EVM transaction fetch")
	return nil
}

// GetAccountTransactions fetches transactions for a specific account
func (s *BlockchainService) GetAccountTransactions(account models.ChainAccount) error {
	logger.Debug("Fetching transactions for %s (%s)", account.EvmChain, account.Address)

	transactionAccount := models.EvmTransactionAccount{
		Address:  account.Address,
		EvmChain: account.EvmChain,
	}

	requestData := models.EvmTransactionsRequest{
		Accounts: []models.EvmTransactionAccount{transactionAccount},
	}

	// Use async for fetching EVM transactions
	response, err := async.Post[bool](s.asyncClient, "/blockchains/evm/transactions", requestData)
	if err != nil {
		logger.Error("Failed to fetch transactions for %s for chain %s: %v",
			account.Address, account.EvmChain, err)
		return fmt.Errorf("failed to fetch transactions for %s for chain %s: %w", account.Address, account.EvmChain, err)
	}
	if response == nil {
		return fmt.Errorf("received nil response for transactions of %s on chain %s", account.Address, account.EvmChain)
	}

	return nil
}

// DecodeEvmTransactions decodes EVM transactions for each supported chain
func (s *BlockchainService) DecodeEvmTransactions() error {
	evmChains, err := s.GetSupportedEvmChains()
	if err != nil {
		return fmt.Errorf("failed to get EVM chains: %w", err)
	}

	// Filter out chains without an EVM chain name and excluded chains
	chainNames := make([]string, 0)
	for _, chain := range evmChains {
		if chain.EvmChainName != "" && !isChainExcluded(chain.EvmChainName) {
			chainNames = append(chainNames, chain.EvmChainName)
		}
	}

	logger.Info("Found %d EVM chains for transaction decoding", len(chainNames))

	for _, chainName := range chainNames {
		logger.Debug("Decoding transactions for chain %s", chainName)

		requestData := models.EvmTransactionDecodeRequest{
			Chains: []string{chainName},
		}

		// Use async for decoding EVM transactions
		response, err := async.Post[models.EvmTransactionDecodeResult](s.asyncClient, "/blockchains/evm/transactions/decode", requestData)
		if err != nil {
			logger.Error("Failed to decode transactions for chain %s: %v", chainName, err)
			continue
		}
		if response == nil {
			logger.Error("Received nil response for decoding transactions on chain %s", chainName)
			continue
		}

		decodedTransactions := response.Result.DecodedTxNumber[chainName]
		if decodedTransactions > 0 {
			logger.Info("Decoded %d transactions for chain %s", decodedTransactions, chainName)
		}
	}

	return nil
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
func (s *BlockchainService) DetectTokens() error {
	chains, err := s.GetTokenDetectionChains()
	if err != nil {
		return err
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
				logger.Error("Failed to detect tokens for %s on %s: %v", address, chain.ChainName, err)
				continue
			}

			logger.Info("Token detection completed for %s on %s", address, chain.ChainName)
		}
	}

	return nil
}

// FetchNonEvmTransactions fetches transactions for non-EVM chain types
func (s *BlockchainService) FetchNonEvmTransactions() error {
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

			_, err := async.Post[bool](s.asyncClient, "/blockchains/transactions", requestData)
			if err != nil {
				logger.Error("Failed to fetch transactions for %s on %s: %v",
					account.Address, account.Blockchain, err)
				continue
			}
		}

		logger.Info("Completed %s transaction fetch", chainType)
	}

	return nil
}

// DecodeNonEvmTransactions decodes transactions for non-EVM chain types that support decoding
func (s *BlockchainService) DecodeNonEvmTransactions() error {
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

			_, err := async.Post[models.EvmTransactionDecodeResult](s.asyncClient, "/blockchains/transactions/decode", requestData)
			if err != nil {
				logger.Error("Failed to decode transactions for chain %s: %v", chain.ID, err)
				continue
			}

			logger.Info("Decoded transactions for %s chain %s", chainType, chain.ID)
		}
	}

	return nil
}

// FetchOnlineEvents fetches online events
func (s *BlockchainService) FetchOnlineEvents() error {
	logger.Info("Fetching online events")

	// Check if eth2 module is activated
	isEth2Active, err := s.IsEth2ModuleActive()
	if err != nil {
		logger.Error("Failed to check eth2 module status: %v", err)
		return fmt.Errorf("failed to check eth2 module status: %w", err)
	}

	if !isEth2Active {
		logger.Info("Eth2 module is not active, skipping online events fetch")
		return nil
	}

	for _, queryType := range []models.QueryType{models.BlockProductionsQuery, models.EthWithdrawalsQuery} {
		logger.Info("Fetching %s events", queryType)

		requestData := models.EventsQueryPayload{
			QueryType: queryType,
		}

		// Use async for fetching history events
		response, err := async.Post[bool](s.asyncClient, "/history/events/query", requestData)
		if err != nil {
			logger.Error("Failed to fetch %s events: %v", queryType, err)
			continue
		}
		if response == nil {
			logger.Error("Received nil response for %s events", queryType)
			continue
		}

		if response.Result {
			logger.Info("Successfully fetched %s events", queryType)
		}
	}

	return nil
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
