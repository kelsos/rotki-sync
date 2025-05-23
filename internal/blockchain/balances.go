package blockchain

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
	"github.com/kelsos/rotki-sync/internal/utils"
)

// FetchExchangeRate fetches the exchange rate for a specific currency against USD
func FetchExchangeRate(baseURL string, currency string) (float64, error) {
	url := fmt.Sprintf("%s/api/1/exchange_rates?currencies=%s", baseURL, currency)

	response, err := utils.FetchWithValidation[models.ExchangeRateResponse](url, http.MethodGet, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch exchange rate for %s: %w", currency, err)
	}

	rate, exists := response.Result[currency]
	if !exists {
		return 0, fmt.Errorf("currency %s not found in exchange rate response", currency)
	}

	exchangeRate, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse exchange rate for %s: %w", currency, err)
	}

	logger.Info("Exchange rate for %s: %f", currency, exchangeRate)
	return exchangeRate, nil
}

// GetLastBalanceSave fetches periodic data from the API and returns the last_balance_save timestamp
func GetLastBalanceSave(baseURL string) (int64, error) {
	url := fmt.Sprintf("%s/api/1/periodic", baseURL)

	response, err := utils.FetchWithValidation[models.PeriodicResponse](url, http.MethodGet, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch periodic data: %w", err)
	}

	lastBalanceSave := response.Result.LastBalanceSave

	logger.Debug("Last balance save date: %s", time.Unix(lastBalanceSave, 0).Format(time.RFC3339))

	return lastBalanceSave, nil
}

// GetBalanceSaveFrequency fetches settings data from the API and returns the balance_save_frequency value
func GetBalanceSaveFrequency(baseURL string) (int, error) {
	type SettingsResult struct {
		BalanceSaveFrequency int `json:"balance_save_frequency"`
	}
	type SettingsResponse = models.APIResponse[SettingsResult]

	url := fmt.Sprintf("%s/api/1/settings", baseURL)

	response, err := utils.FetchWithValidation[SettingsResponse](url, http.MethodGet, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch settings data: %w", err)
	}

	balanceSaveFrequency := response.Result.BalanceSaveFrequency

	logger.Debug("Balance save frequency: %d hours", balanceSaveFrequency)

	return balanceSaveFrequency, nil
}

// TakeBalanceSnapshot fetches balances and logs them
func TakeBalanceSnapshot(baseURL string, forceSnapshot bool) error {
	logger.Info("Fetching balances for snapshot (force: %t)", forceSnapshot)

	query := ""
	if forceSnapshot {
		query = "?save_data=true"
	}

	url := fmt.Sprintf("%s/api/1/balances%s", baseURL, query)

	response, err := utils.FetchWithValidation[models.BalanceResponse](url, http.MethodGet, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch balances: %w", err)
	}

	euroRate, err := FetchExchangeRate(baseURL, "EUR")
	if err != nil {
		return err
	}

	PrintPerLocationBalances(response, euroRate)
	logger.Info("Fetched balances successfully")

	return nil
}

func PrintPerLocationBalances(response *models.BalanceResponse, euroRate float64) {
	perLocation := make([]string, 0, len(response.Result.Location))
	for location, balance := range response.Result.Location {
		usdValue, err := strconv.ParseFloat(balance.UsdValue, 64)
		if err != nil {
			logger.Warn("Failed to parse USD value for location %s: %v", location, err)
			continue
		}

		perLocation = append(perLocation, fmt.Sprintf("Location: %s - %.2f EUR (%s %%)",
			location, usdValue*euroRate, balance.PercentageOfNetValue))
	}

	logger.Info("\nBalances per location:\n%s]\n", strings.Join(perLocation, "\n"))
}

// PerformSnapshotIfNeeded checks if it's time to take a new balance snapshot and does so if necessary
func PerformSnapshotIfNeeded(baseURL string) error {
	lastBalanceSave, err := GetLastBalanceSave(baseURL)
	if err != nil {
		return err
	}

	balanceSaveFrequency, err := GetBalanceSaveFrequency(baseURL)
	if err != nil {
		return err
	}

	snapshotIntervalInSeconds := int64(balanceSaveFrequency * 60 * 60)
	currentTime := time.Now().Unix()

	enoughTimeElapsed := utils.HasEnoughTimeElapsed(lastBalanceSave)
	requiredTimeElapsed := currentTime-lastBalanceSave > snapshotIntervalInSeconds
	if requiredTimeElapsed || enoughTimeElapsed {
		logger.Info("Time to take a snapshot!")
		if err := TakeBalanceSnapshot(baseURL, enoughTimeElapsed && !requiredTimeElapsed); err != nil {
			return err
		}
	} else {
		nextSnapshotTime := time.Unix(lastBalanceSave+snapshotIntervalInSeconds, 0)
		logger.Info("The next snapshot will be taken at %s", nextSnapshotTime.Format(time.RFC3339))
	}

	return nil
}
