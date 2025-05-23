package blockchain

import (
	"fmt"
	"net/http"

	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
	"github.com/kelsos/rotki-sync/internal/utils"
)

// GetSupportedEvmChains fetches supported blockchains and returns the EVM chains
func GetSupportedEvmChains(baseURL string) ([]models.Blockchain, error) {
	url := fmt.Sprintf("%s/api/1/blockchains/supported", baseURL)

	response, err := utils.FetchWithValidation[models.BlockchainResponse](url, http.MethodGet, nil)
	if err != nil {
		logger.Error("Failed to fetch supported blockchains: %v", err)
		return nil, fmt.Errorf("failed to fetch supported blockchains: %w", err)
	}

	// Filter for EVM chains
	evmChains := make([]models.Blockchain, 0)
	for _, chain := range response.Result {
		if chain.Type == "evm" {
			evmChains = append(evmChains, chain)
		}
	}

	logger.Info("Found %d EVM chains", len(evmChains))
	return evmChains, nil
}

// DecodeEvmTransactions decodes EVM transactions for each supported chain
func DecodeEvmTransactions(baseURL string) error {
	evmChains, err := GetSupportedEvmChains(baseURL)
	if err != nil {
		return err
	}

	// Filter out chains without an EVM chain name and avalanche
	chainNames := make([]string, 0)
	for _, chain := range evmChains {
		if chain.EvmChainName != "" && chain.EvmChainName != "avalanche" {
			chainNames = append(chainNames, chain.EvmChainName)
		}
	}

	logger.Info("Found %d EVM chains for transaction decoding", len(chainNames))

	for _, chainName := range chainNames {
		logger.Debug("Decoding transactions for chain %s", chainName)

		payload := models.EvmTransactionDecodeRequest{
			Chains: []string{chainName},
		}

		url := fmt.Sprintf("%s/api/1/blockchains/evm/transactions/decode", baseURL)
		response, err := utils.FetchWithValidation[models.EvmTransactionDecodeResponse](url, http.MethodPost, payload)
		if err != nil {
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

func FetchOnlineEvents(baseURL string) error {
	logger.Info("Fetching online events")
	// TODO check if eth2 module is activated...

	for _, queryType := range []models.QueryType{models.BlockProductionsQuery, models.EthWithdrawalsQuery} {
		logger.Info("Fetching %s events", queryType)
		payload := models.EventsQueryPayload{
			QueryType: queryType,
		}
		url := fmt.Sprintf("%s/api/1/history/events/query", baseURL)
		response, err := utils.FetchWithValidation[models.EventsQueryResponse](url, http.MethodPost, payload)

		if err != nil {
			logger.Error("Failed to fetch %s events: %v", queryType, err)
			continue
		}

		if response.Result {
			logger.Info("Fetched %s events", queryType)
		}
	}

	return nil
}
