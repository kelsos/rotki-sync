package services

import (
	"fmt"
	"sort"

	"github.com/kelsos/rotki-sync/internal/async"
	"github.com/kelsos/rotki-sync/internal/client"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/models"
	"github.com/kelsos/rotki-sync/internal/secrets"
)

// UserService handles user-related operations
type UserService struct {
	client      *client.APIClient
	asyncClient *async.Client
	secrets     *secrets.Store
}

// NewUserServiceWithAsyncClient creates a new user service with an async client
// and the secret store used to resolve per-user login passwords.
func NewUserServiceWithAsyncClient(client *client.APIClient, asyncClient *async.Client, store *secrets.Store) *UserService {
	return &UserService{
		client:      client,
		asyncClient: asyncClient,
		secrets:     store,
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
	sort.Strings(users)

	return users, nil
}

// Login logs in a user with the password resolved from the secret store.
func (s *UserService) Login(username string) error {
	logger.Info("Logging in user %s", username)

	password, ok, err := s.secrets.Get(secrets.ScopeUsers, username)
	if err != nil {
		return fmt.Errorf("failed to read stored password for user %s: %w", username, err)
	}
	if !ok || password == "" {
		return fmt.Errorf("no stored password for user %s; run: rotki-sync secret set %s", username, username)
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

// getSortedUsers fetches users and returns sorted usernames plus any currently logged-in users
func (s *UserService) getSortedUsers() (allUsers []string, loggedIn []string, err error) {
	var userResponse models.UserResponse
	if err := s.client.Get("/users", &userResponse); err != nil {
		return nil, nil, fmt.Errorf("failed to get users: %w", err)
	}

	for username, userStatus := range userResponse.Result {
		allUsers = append(allUsers, username)
		if userStatus == models.StatusLoggedIn {
			loggedIn = append(loggedIn, username)
		}
	}
	sort.Strings(allUsers)
	sort.Strings(loggedIn)

	return allUsers, loggedIn, nil
}

// logoutUsers logs out a list of users, logging errors but not failing
func (s *UserService) logoutUsers(usernames []string) {
	for _, username := range usernames {
		if err := s.Logout(username); err != nil {
			logger.Error("Failed to logout user %s: %v", username, err)
		}
	}
}

// ProcessUsers processes all users with the given function
func (s *UserService) ProcessUsers(processFunc func(username string) error) error {
	allUsers, loggedIn, err := s.getSortedUsers()
	if err != nil {
		return err
	}

	// Logout all currently logged-in users
	s.logoutUsers(loggedIn)

	// Process each user
	for _, username := range allUsers {
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

// ProcessUsersWithCallback processes all users with callbacks for monitoring.
// onLoginResult is called after a login attempt with the error (nil on success).
// onLogout is called after processing or on login failure.
func (s *UserService) ProcessUsersWithCallback(
	onLoginResult func(username string, loginErr error),
	processFunc func(username string) error,
	onLogout func(username string) error,
) error {
	allUsers, loggedIn, err := s.getSortedUsers()
	if err != nil {
		return err
	}

	// Logout all currently logged-in users
	s.logoutUsers(loggedIn)

	// Process each user
	for _, username := range allUsers {
		loginErr := s.Login(username)
		if loginErr != nil {
			logger.Error("Failed to login user %s: %v", username, loginErr)
		}

		// Notify about login result
		if onLoginResult != nil {
			onLoginResult(username, loginErr)
		}

		if loginErr != nil {
			// Call onLogout so the TUI can mark this user as complete/failed
			if onLogout != nil {
				_ = onLogout(username)
			}
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
