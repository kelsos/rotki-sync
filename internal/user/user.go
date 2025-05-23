package user

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
	"github.com/kelsos/rotki-sync/internal/utils"
)

// GetUsers retrieves all users from the API
func GetUsers(baseURL string) ([]string, error) {
	usersURL := fmt.Sprintf("%s/api/1/users", baseURL)

	response, err := utils.FetchWithValidation[models.UserResponse](usersURL, http.MethodGet, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}

	usernames := make([]string, 0, len(response.Result))
	for username := range response.Result {
		usernames = append(usernames, username)
	}

	logger.Info("Found the following users: %v", usernames)
	return usernames, nil
}

// Login logs in a user with the given username
func Login(baseURL, username string) error {
	logger.Info("Logging in user %s", username)

	// Get password from environment variable
	passwordEnvVar := fmt.Sprintf("%s_PASSWORD", strings.ToUpper(username))
	password := os.Getenv(passwordEnvVar)

	if password == "" {
		return fmt.Errorf("missing environment variable %s for user %s", passwordEnvVar, username)
	}

	loginURL := fmt.Sprintf("%s/api/1/users/%s", baseURL, username)
	loginPayload := map[string]string{"password": password}

	_, err := utils.FetchWithValidation[models.UserLoginResponse](loginURL, http.MethodPost, loginPayload)
	if err != nil {
		return fmt.Errorf("failed to login user %s: %w", username, err)
	}

	logger.Debug("User %s logged in successfully", username)
	return nil
}

// Logout logs out a user with the given username
func Logout(baseURL, username string) error {
	logger.Info("Logging out user %s", username)

	logoutURL := fmt.Sprintf("%s/api/1/users/%s", baseURL, username)
	logoutPayload := map[string]string{"action": "logout"}

	_, err := utils.FetchWithValidation[models.UserActionResponse](logoutURL, http.MethodPatch, logoutPayload)
	if err != nil {
		return fmt.Errorf("failed to logout user %s: %w", username, err)
	}

	logger.Debug("User %s logged out successfully", username)
	return nil
}

// ProcessUsers retrieves all users and processes them one by one
func ProcessUsers(baseURL string, processFunc func(baseURL, username string) error) error {
	response, err := utils.FetchWithValidation[models.UserResponse](fmt.Sprintf("%s/api/1/users", baseURL), http.MethodGet, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch users: %w", err)
	}

	for username, status := range response.Result {
		logger.Info("Processing user: %s (current status: %s)\n", username, status)

		passwordEnvVar := fmt.Sprintf("%s_PASSWORD", strings.ToUpper(username))
		password := os.Getenv(passwordEnvVar)

		if password == "" {
			logger.Error("Missing environment variable %s for user %s", passwordEnvVar, username)
			continue
		}

		if status == models.StatusLoggedIn {
			logger.Info("User %s is already logged in, logging out first...", username)
			if err := Logout(baseURL, username); err != nil {
				logger.Error("Failed to logout user %s: %v", username, err)
				continue
			}
		}

		if err := Login(baseURL, username); err != nil {
			logger.Error("Failed to login user %s: %v", username, err)
			continue
		}

		logger.Info("Processing data for user %s...", username)
		if err := processFunc(baseURL, username); err != nil {
			logger.Error("Error processing user %s: %v", username, err)
		}

		if err := Logout(baseURL, username); err != nil {
			logger.Error("Failed to logout user %s: %v", username, err)
		}
	}

	return nil
}
