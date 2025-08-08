package services

import (
	"fmt"

	"github.com/kelsos/rotki-sync/internal/async"
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
)

// ExchangeService handles exchange-related operations
type ExchangeService struct {
	client      *client.APIClient
	asyncClient *async.Client
}

// NewExchangeServiceWithAsyncClient creates a new exchange service with an async client
func NewExchangeServiceWithAsyncClient(client *client.APIClient, asyncClient *async.Client) *ExchangeService {
	return &ExchangeService{
		client:      client,
		asyncClient: asyncClient,
	}
}

// GetConnectedExchanges retrieves all connected exchanges
func (s *ExchangeService) GetConnectedExchanges() ([]models.Exchange, error) {
	var response models.APIResponse[[]models.Exchange]
	if err := s.client.Get("/exchanges", &response); err != nil {
		return nil, fmt.Errorf("failed to get connected exchanges: %w", err)
	}

	logger.Info("Found %d connected exchanges", len(response.Result))
	return response.Result, nil
}

// FetchExchangeTrades fetches trades for a specific exchange
func (s *ExchangeService) FetchExchangeTrades(exchange models.Exchange) error {
	logger.Info("Fetching trades for exchange: %s", exchange.Name)

	requestData := map[string]interface{}{
		"location": exchange.Location,
	}

	// Use async for fetching exchange trades
	response, err := async.Post[bool](s.asyncClient, "/history/events/query/exchange", requestData)
	if err != nil {
		return fmt.Errorf("failed to fetch trades for exchange %s: %w", exchange.Name, err)
	}
	if response == nil {
		return fmt.Errorf("received nil response for exchange %s trades", exchange.Name)
	}

	logger.Info("Successfully fetched trades for exchange: %s", exchange.Name)
	return nil
}

// GetExchangeTrades fetches trades for all connected exchanges
func (s *ExchangeService) GetExchangeTrades() error {
	connectedExchanges, err := s.GetConnectedExchanges()
	if err != nil {
		return fmt.Errorf("failed to get connected exchanges: %w", err)
	}

	if len(connectedExchanges) == 0 {
		logger.Info("No connected exchanges found")
		return nil
	}

	logger.Info("Processing %d connected exchanges", len(connectedExchanges))

	for _, exchange := range connectedExchanges {
		err := s.FetchExchangeTrades(exchange)
		if err != nil {
			logger.Error("Failed to fetch trades for exchange %s: %v", exchange.Name, err)
			continue
		}
	}

	logger.Info("Completed fetching trades for all exchanges")
	return nil
}
