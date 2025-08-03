package services

import (
	"fmt"
	"time"

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
	client *client.APIClient
}

// NewBlockchainService creates a new blockchain service
func NewBlockchainService(client *client.APIClient) *BlockchainService {
	return &BlockchainService{
		client: client,
	}
}

// GetSupportedEvmChains retrieves supported EVM chains
func (s *BlockchainService) GetSupportedEvmChains() ([]models.Blockchain, error) {
	var response models.BlockchainResponse
	if err := s.client.Get("/blockchains/supported", &response); err != nil {
		return nil, fmt.Errorf("failed to get supported EVM chains: %w", err)
	}

	var evmChains []models.Blockchain
	for _, blockchain := range response.Result {
		if blockchain.Type == "evm" {
			evmChains = append(evmChains, blockchain)
		}
	}

	return evmChains, nil
}

// FetchAccounts retrieves accounts for all EVM chains
func (s *BlockchainService) FetchAccounts() ([]models.ChainAccount, error) {
	evmChains, err := s.GetSupportedEvmChains()
	if err != nil {
		return nil, err
	}

	var allAccounts []models.ChainAccount

	for _, chain := range evmChains {
		logger.Info("Fetching accounts for chain: %s", chain.Name)

		endpoint := fmt.Sprintf("/blockchains/%s/accounts", chain.ID)
		var response models.AccountsResponse

		if err := s.client.Get(endpoint, &response); err != nil {
			logger.Error("Failed to fetch accounts for chain %s: %v", chain.Name, err)
			continue
		}

		for _, account := range response.Result {
			chainAccount := models.ChainAccount{
				Address:  account.Address,
				EvmChain: chain.EvmChainName,
				ChainID:  chain.ID,
			}
			allAccounts = append(allAccounts, chainAccount)
		}

		logger.Info("Found %d accounts for chain %s", len(response.Result), chain.Name)
	}

	return allAccounts, nil
}

// FetchEvmTransactions fetches EVM transactions for all accounts
func (s *BlockchainService) FetchEvmTransactions(fromTimestamp, toTimestamp int64) error {
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

		// Adjust timestamps to be safe (back 1 day from now)
		chainFromTimestamp := fromTimestamp
		if chainFromTimestamp == 0 {
			chainFromTimestamp = time.Now().AddDate(0, 0, -1).Unix()
		}

		for _, account := range accounts {
			err := s.GetAccountTransactions(account, chainFromTimestamp, toTimestamp)
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
func (s *BlockchainService) GetAccountTransactions(account models.ChainAccount, fromTimestamp, toTimestamp int64) error {
	logger.Debug("Fetching transactions for %s (%s)", account.EvmChain, account.Address)

	transactionAccount := models.EvmTransactionAccount{
		Address:  account.Address,
		EvmChain: account.EvmChain,
	}

	requestData := models.EvmTransactionsRequest{
		Accounts:      []models.EvmTransactionAccount{transactionAccount},
		FromTimestamp: fromTimestamp,
		ToTimestamp:   toTimestamp,
	}

	var response models.EvmTransactionsResponse
	if err := s.client.Post("/blockchains/evm/transactions", requestData, &response); err != nil {
		logger.Error("Failed to fetch transactions for %s for chain %s: %v",
			account.Address, account.EvmChain, err)
		return fmt.Errorf("failed to fetch transactions for %s for chain %s: %w", account.Address, account.EvmChain, err)
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

		var response models.EvmTransactionDecodeResponse
		if err := s.client.Post("/blockchains/evm/transactions/decode", requestData, &response); err != nil {
			logger.Error("Failed to decode transactions for chain %s: %v", chainName, err)
			continue
		}

		decodedTransactions := response.Result.DecodedTxNumber[chainName]
		if decodedTransactions > 0 {
			logger.Info("Decoded %d transactions for chain %s", decodedTransactions, chainName)
		}
	}

	return nil
}

// FetchOnlineEvents fetches online events
func (s *BlockchainService) FetchOnlineEvents() error {
	logger.Info("Fetching online events")
	// TODO: check if eth2 module is activated...

	for _, queryType := range []models.QueryType{models.BlockProductionsQuery, models.EthWithdrawalsQuery} {
		logger.Info("Fetching %s events", queryType)

		requestData := models.EventsQueryPayload{
			QueryType: queryType,
		}

		var response models.EventsQueryResponse
		if err := s.client.Post("/history/events/query", requestData, &response); err != nil {
			logger.Error("Failed to fetch %s events: %v", queryType, err)
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

	rate, ok := result[currency].(float64)
	if !ok {
		return 0, fmt.Errorf("exchange rate for %s not found", currency)
	}

	return rate, nil
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

// TakeBalanceSnapshot takes a balance snapshot
func (s *BlockchainService) TakeBalanceSnapshot(forceSnapshot bool) error {
	query := ""
	if forceSnapshot {
		query = "?save_data=true"
	}

	endpoint := fmt.Sprintf("/balances%s", query)
	var response map[string]interface{}

	if err := s.client.Get(endpoint, &response); err != nil {
		return fmt.Errorf("failed to take balance snapshot: %w", err)
	}

	// Fetch EUR exchange rate
	euroRate, err := s.FetchExchangeRate("EUR")
	if err != nil {
		logger.Error("Failed to fetch EUR exchange rate: %v", err)
	} else {
		logger.Info("Current EUR exchange rate: %.6f", euroRate)
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
