// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/token_storage_secure.go

import (
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/zalando/go-keyring" // Import the keyring library
)

const (
	keyringService = "CowGnitionRTM"       // Service name for keyring.
	keyringUser    = "RTMAuthTokenDetails" // User/Account name for keyring entry.
)

// SecureTokenStorage handles storing/retrieving tokens using the OS keychain.
type SecureTokenStorage struct {
	logger logging.Logger
}

// Ensure SecureTokenStorage implements TokenStorageInterface.
var _ TokenStorageInterface = (*SecureTokenStorage)(nil)

// NewSecureTokenStorage creates a new secure token storage instance.
// Note: The logger is passed but keyring itself doesn't use it directly.
// We keep it for consistency and potential future logging within this struct's methods.
func NewSecureTokenStorage(logger logging.Logger) *SecureTokenStorage {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	return &SecureTokenStorage{
		logger: logger.WithField("component", "secure_token_storage"),
	}
}

// IsAvailable checks if the OS keyring service is accessible.
// file: internal/rtm/token_storage_secure.go

// IsAvailable checks if the OS keyring service is accessible.
func (s *SecureTokenStorage) IsAvailable() bool {
	// Try a basic Get operation to check if keyring service is accessible
	_, err := keyring.Get(keyringService, keyringUser)

	// Check error type to determine availability
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			// This is expected for first-time usage - the token doesn't exist yet
			s.logger.Debug("Keyring service is accessible (no token found, which is normal for first use)")
			return true
		}
		// Other errors indicate the keyring service isn't working properly
		s.logger.Warn("Keyring service is inaccessible or permissions are insufficient", "error", err)
		return false
	}

	// No error means service is available and token exists
	s.logger.Debug("Keyring service is accessible and contains an existing token")
	return true
}

// file: internal/rtm/token_storage_secure.go

// LoadToken loads the token from the secure OS keyring.
func (s *SecureTokenStorage) LoadToken() (string, error) {
	s.logger.Info("Attempting to load authentication token from system keyring")
	jsonData, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("No authentication token found in system keyring")
			return "", nil // Not an error, just not found.
		}
		s.logger.Error("Failed to access system keyring", "error", err)
		return "", errors.Wrap(err, "failed to load token from system keyring")
	}
	s.logger.Info("Successfully retrieved token data from system keyring")

	var data TokenData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		// Data might be corrupted, delete it and report error
		s.logger.Error("Token data in keyring is corrupted and cannot be parsed", "error", err)
		_ = s.DeleteToken() // Attempt to delete corrupted entry.
		return "", errors.Wrap(err, "failed to parse token data from secure storage")
	}

	s.logger.Debug("Successfully decoded authentication token", "username", data.Username)
	return data.Token, nil
}

// SaveToken saves token data securely to the OS keyring.
func (s *SecureTokenStorage) SaveToken(token string, userID, username string) error {
	s.logger.Debug("Saving authentication token to system keyring", "username", username)
	data := TokenData{
		Token:     token,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "failed to encode token data for secure storage")
	}

	err = keyring.Set(keyringService, keyringUser, string(jsonData))
	if err != nil {
		s.logger.Error("Failed to save token to system keyring", "error", err)
		return errors.Wrap(err, "failed to save token to system keyring")
	}
	s.logger.Info("Authentication token successfully saved to system keyring")
	return nil
}

// DeleteToken removes the token from the secure OS keyring.
func (s *SecureTokenStorage) DeleteToken() error {
	s.logger.Debug("Deleting authentication token from system keyring")

	err := keyring.Delete(keyringService, keyringUser)

	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("No token to delete - system keyring entry not found")
			return nil
		}
		s.logger.Error("Failed to delete token from system keyring", "error", err)
		return errors.Wrap(err, "failed to delete token from system keyring")
	}

	s.logger.Info("Authentication token successfully deleted from system keyring")
	return nil
}

// GetTokenData loads the full token data from the secure storage.
func (s *SecureTokenStorage) GetTokenData() (*TokenData, error) {
	jsonData, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil // Return nil data and nil error if not found.
		}
		s.logger.Error("Failed to load token data from keyring.", "error", err)
		return nil, errors.Wrap(err, "failed to load token data from keyring")
	}

	var data TokenData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		s.logger.Error("Failed to parse token data from secure storage, attempting deletion.", "error", err)
		_ = s.DeleteToken() // Attempt to delete corrupted entry.
		return nil, errors.Wrap(err, "failed to parse token data from secure storage")
	}

	return &data, nil
}

// UpdateToken updates the stored token data. Re-uses SaveToken as keyring.Set often overwrites.
func (s *SecureTokenStorage) UpdateToken(token string, userID, username string) error {
	// Keyring Set usually overwrites, so we can just call SaveToken.
	// If more complex update logic is needed (e.g., preserving CreatedAt),
	// load existing data first.
	s.logger.Debug("Updating token in secure storage.", "username", username)
	return s.SaveToken(token, userID, username)
}
