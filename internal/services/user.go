package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/kelsos/rotki-sync/internal/async"
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
)

// UserService handles user-related operations
type UserService struct {
	client      *client.APIClient
	asyncClient *async.Client
}

// NewUserServiceWithAsyncClient creates a new user service with an async client
func NewUserServiceWithAsyncClient(client *client.APIClient, asyncClient *async.Client) *UserService {
	return &UserService{
		client:      client,
		asyncClient: asyncClient,
	}
}

// GetUsers retrieves all users from the API
func (s *UserService) GetUsers() ([]string, error) {
	var response models.UserResponse
	if err := s.client.Get("/users", &response); err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	var users []string
	for username := range response.Result {
		users = append(users, username)
	}

	return users, nil
}

// Login logs in a user with password from environment variable
func (s *UserService) Login(username string) error {
	logger.Info("Logging in user %s", username)

	// Get password from environment variable
	passwordEnvVar := fmt.Sprintf("%s_PASSWORD", strings.ToUpper(username))
	password := os.Getenv(passwordEnvVar)

	if password == "" {
		return fmt.Errorf("missing environment variable %s for user %s", passwordEnvVar, username)
	}

	endpoint := fmt.Sprintf("/users/%s", username)
	loginData := map[string]interface{}{
		"password": password,
	}

	// Use async login
	response, err := async.Post[models.UserLoginResponse](s.asyncClient, endpoint, loginData)
	if err != nil {
		return fmt.Errorf("failed to login user %s: %w", username, err)
	}
	// Check if response is not nil to ensure successful async execution
	if response == nil {
		return fmt.Errorf("received nil response for user %s login", username)
	}

	logger.Debug("User %s logged in successfully", username)
	return nil
}

// Logout logs out a user
func (s *UserService) Logout(username string) error {
	logger.Info("Logging out user %s", username)

	endpoint := fmt.Sprintf("/users/%s", username)
	logoutData := map[string]string{
		"action": "logout",
	}

	var response models.UserActionResponse
	if err := s.client.Patch(endpoint, logoutData, &response); err != nil {
		return fmt.Errorf("failed to logout user %s: %w", username, err)
	}

	logger.Debug("User %s logged out successfully", username)
	return nil
}

// ProcessUsers processes all users with the given function
func (s *UserService) ProcessUsers(processFunc func(username string) error) error {
	var userResponse models.UserResponse
	if err := s.client.Get("/users", &userResponse); err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	var loggedInUsers []string
	for username, userStatus := range userResponse.Result {
		if userStatus == models.StatusLoggedIn {
			loggedInUsers = append(loggedInUsers, username)
		}
	}

	// Logout all currently logged-in users
	for _, username := range loggedInUsers {
		if err := s.Logout(username); err != nil {
			logger.Error("Failed to logout user %s: %v", username, err)
		}
	}

	// Process each user
	for username := range userResponse.Result {
		if err := s.Login(username); err != nil {
			logger.Error("Failed to login user %s: %v", username, err)
			continue
		}

		logger.Info("Processing user: %s", username)
		if err := processFunc(username); err != nil {
			logger.Error("Error processing user %s: %v", username, err)
		}

		if err := s.Logout(username); err != nil {
			logger.Error("Failed to logout user %s: %v", username, err)
		}
	}

	return nil
}

// ProcessUsersWithCallback processes all users with callbacks for monitoring
func (s *UserService) ProcessUsersWithCallback(
	onLogin func(username string) error,
	processFunc func(username string) error,
	onLogout func(username string) error,
) error {
	var userResponse models.UserResponse
	if err := s.client.Get("/users", &userResponse); err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	var loggedInUsers []string
	for username, userStatus := range userResponse.Result {
		if userStatus == models.StatusLoggedIn {
			loggedInUsers = append(loggedInUsers, username)
		}
	}

	// Logout all currently logged-in users
	for _, username := range loggedInUsers {
		if err := s.Logout(username); err != nil {
			logger.Error("Failed to logout user %s: %v", username, err)
		}
	}

	// Process each user
	for username := range userResponse.Result {
		// Call onLogin callback
		if onLogin != nil {
			if err := onLogin(username); err != nil {
				logger.Error("onLogin callback failed for user %s: %v", username, err)
			}
		}

		if err := s.Login(username); err != nil {
			logger.Error("Failed to login user %s: %v", username, err)
			continue
		}

		logger.Info("Processing user: %s", username)
		if err := processFunc(username); err != nil {
			logger.Error("Error processing user %s: %v", username, err)
		}

		// Call onLogout callback
		if onLogout != nil {
			if err := onLogout(username); err != nil {
				logger.Error("onLogout callback failed for user %s: %v", username, err)
			}
		}

		if err := s.Logout(username); err != nil {
			logger.Error("Failed to logout user %s: %v", username, err)
		}
	}

	return nil
}
