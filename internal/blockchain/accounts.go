package blockchain

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
	"github.com/kelsos/rotki-sync/internal/storage"
	"github.com/kelsos/rotki-sync/internal/utils"
)

// FetchAccounts fetches accounts for all supported EVM chains and returns unique address-chainId combinations
func FetchAccounts(baseURL string) ([]models.ChainAccount, error) {
	// Get supported EVM chains
	evmChains, err := GetSupportedEvmChains(baseURL)
	if err != nil {
		return nil, err
	}

	chains := make([]struct {
		ID           string
		EvmChainName string
	}, 0, len(evmChains))

	for _, chain := range evmChains {
		chains = append(chains, struct {
			ID           string
			EvmChainName string
		}{
			ID:           chain.ID,
			EvmChainName: chain.EvmChainName,
		})
	}

	logger.Debug("Found %d EVM chains to check for accounts", len(chains))

	allChainAccounts := make([]models.ChainAccount, 0)

	for _, chain := range chains {
		url := fmt.Sprintf("%s/api/1/blockchains/%s/accounts", baseURL, chain.ID)

		response, err := utils.FetchWithValidation[models.AccountsResponse](url, http.MethodGet, nil)
		if err != nil {
			logger.Warn("Failed to fetch accounts for chain %s: %v", chain.ID, err)
			continue
		}

		chainAccounts := make([]models.ChainAccount, 0, len(response.Result))
		for _, account := range response.Result {
			evmChain := chain.EvmChainName
			if evmChain == "" {
				evmChain = chain.ID
			}

			chainAccounts = append(chainAccounts, models.ChainAccount{
				ChainID:  chain.ID,
				EvmChain: evmChain,
				Address:  account.Address,
				Label:    &account.Label,
				Tags:     account.Tags,
			})
		}

		allChainAccounts = append(allChainAccounts, chainAccounts...)

		logger.Debug("Fetched %d accounts for chain %s", len(chainAccounts), chain.ID)
	}

	uniqueChainAccounts := createUniqueChainAccounts(allChainAccounts)

	logger.Info("Total unique accounts found: %d", len(uniqueChainAccounts))

	return uniqueChainAccounts, nil
}

// createUniqueChainAccounts creates a unique list of chain accounts based on chainId + address combination
func createUniqueChainAccounts(accounts []models.ChainAccount) []models.ChainAccount {
	uniqueMap := make(map[string]models.ChainAccount)

	for _, account := range accounts {
		key := fmt.Sprintf("%s:%s", account.ChainID, strings.ToLower(account.Address))

		if _, exists := uniqueMap[key]; !exists {
			uniqueMap[key] = account
		}
	}

	uniqueAccounts := make([]models.ChainAccount, 0, len(uniqueMap))
	for _, account := range uniqueMap {
		uniqueAccounts = append(uniqueAccounts, account)
	}

	return uniqueAccounts
}

// FetchEvmTransactions fetches EVM transactions for accounts, processing one chain at a time
func FetchEvmTransactions(baseURL string, fromTimestamp, toTimestamp int64) error {
	if toTimestamp == 0 {
		toTimestamp = time.Now().Unix()
	}

	chainAccounts, err := FetchAccounts(baseURL)
	if err != nil {
		return err
	}

	accountsByChain := GroupAccountsByChain(chainAccounts)

	chains := make([]string, 0, len(accountsByChain))
	for chain := range accountsByChain {
		chains = append(chains, chain)
	}
	sort.Strings(chains)

	logger.Info("Fetching transactions for %d EVM chains: %s", len(chains), strings.Join(chains, " "))

	for _, evmChain := range chains {
		chainFromTimestamp := GetRequestRangeStart(fromTimestamp, evmChain)

		logger.Debug("Starting EVM transaction fetch for chain %s from %s to %s",
			evmChain,
			time.Unix(chainFromTimestamp, 0).Format(time.RFC3339),
			time.Unix(toTimestamp, 0).Format(time.RFC3339))

		accounts := accountsByChain[evmChain]
		logger.Debug("Getting transactions for %s number of addresses %d", evmChain, len(accounts))

		for _, account := range accounts {
			err := GetAccountTransactions(baseURL, account, chainFromTimestamp, toTimestamp)
			if err != nil {
				continue
			}
		}

		if err := storage.SaveTimestamp(evmChain, toTimestamp); err != nil {
			logger.Warn("Failed to save timestamp for chain %s: %v", evmChain, err)
		} else {
			logger.Debug("Saved timestamp for chain %s: %s", evmChain, time.Unix(toTimestamp, 0).Format(time.RFC3339))
		}
	}

	return nil
}

func GetAccountTransactions(baseURL string, account models.ChainAccount, fromTimestamp int64, toTimestamp int64) error {
	logger.Debug("Fetching transactions for %s (%s)", account.EvmChain, account.Address)

	transactionAccount := models.EvmTransactionAccount{
		Address:  account.Address,
		EvmChain: account.EvmChain,
	}

	payload := models.EvmTransactionsRequest{
		Accounts:      []models.EvmTransactionAccount{transactionAccount},
		FromTimestamp: fromTimestamp,
		ToTimestamp:   toTimestamp,
	}

	url := fmt.Sprintf("%s/api/1/blockchains/evm/transactions", baseURL)
	response, err := utils.FetchWithValidation[models.EvmTransactionsResponse](url, http.MethodPost, payload)
	if err != nil {
		logger.Error("Failed to fetch transactions for %s for chain %s: %v",
			account.Address, account.EvmChain, err)
		return fmt.Errorf("failed to fetch transactions for %s for chain %s: %v", account.Address, account.EvmChain, err)
	}

	if response.Result {
		logger.Debug("Fetched transactions for %s for chain %s",
			account.Address, account.EvmChain)
	} else {
		logger.Error("Failed to fetch transactions for %s for chain %s",
			account.Address, account.EvmChain)
	}
	return nil
}

func GetRequestRangeStart(fromTimestamp int64, evmChain string) int64 {
	chainFromTimestamp := fromTimestamp
	if fromTimestamp == 0 {
		lastTimestamp, err := storage.GetLastTimestamp(evmChain)
		if err != nil {
			logger.Warn("Failed to get last timestamp for chain %s: %v", evmChain, err)
			chainFromTimestamp = 0
		} else if lastTimestamp > 0 {
			chainFromTimestamp = lastTimestamp
			formattedTimestamp := time.Unix(chainFromTimestamp, 0).Format(time.RFC3339)
			logger.Debug("Using last timestamp for chain %s: %s", evmChain, formattedTimestamp)
		} else {
			chainFromTimestamp = 0
		}
	}
	return chainFromTimestamp
}

func GroupAccountsByChain(chainAccounts []models.ChainAccount) map[string][]models.ChainAccount {
	accountsByChain := make(map[string][]models.ChainAccount)
	for _, account := range chainAccounts {
		if account.EvmChain != "avalanche" {
			accountsByChain[account.EvmChain] = append(accountsByChain[account.EvmChain], account)
		}
	}
	logger.Debug("Grouped accounts into %d unique chains", len(accountsByChain))
	return accountsByChain
}
