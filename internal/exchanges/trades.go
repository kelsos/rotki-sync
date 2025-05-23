package exchanges

import (
	"fmt"
	"net/http"

	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
	"github.com/kelsos/rotki-sync/internal/utils"
)

func GetConnectedExchanges(baseURL string) ([]models.Exchange, error) {
	type SettingsResponse = models.APIResponse[[]models.Exchange]

	url := fmt.Sprintf("%s/api/1/exchanges", baseURL)

	response, err := utils.FetchWithValidation[SettingsResponse](url, http.MethodGet, nil)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch settings data: %w", err)
	}

	connectedExchanges := response.Result

	logger.Info("Connected exchanges: %d exchanges", len(connectedExchanges))

	return connectedExchanges, nil
}

func FetchExchangeTrades(baseURL string, exchange models.Exchange) error {
	logger.Info("Fetching trades for exchange %s (%s)", exchange.Name, exchange.Location)

	payload := models.TradeRequestPayload{
		Name:     exchange.Name,
		Location: exchange.Location,
	}

	url := fmt.Sprintf("%s/api/1/history/events/query/exchange", baseURL)

	_, err := utils.FetchWithValidation[models.APIResponse[bool]](url, http.MethodPost, payload)

	if err != nil {
		logger.Error("Failed to fetch trades for exchange %s (%s): %v", exchange.Name, exchange.Location, err)
		return fmt.Errorf("failed to fetch trades for exchange %s (%s): %w", exchange.Name, exchange.Location, err)
	}
	logger.Debug("Fetched trades for exchange %s (%s)", exchange.Name, exchange.Location)
	return nil
}

func GetExchangeTrades(baseURL string) error {
	connectedExchanges, err := GetConnectedExchanges(baseURL)

	if len(connectedExchanges) == 0 {
		return nil
	}

	logger.Info("Fetching exchange trades from %d", len(connectedExchanges))

	if err != nil {
		return fmt.Errorf("failed to fetch connected exchanges %w", err)
	}

	for _, exchange := range connectedExchanges {
		err := FetchExchangeTrades(baseURL, exchange)
		if err != nil {
			continue
		}
	}
	return nil
}
