// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/token_storage_secure.go

// file: internal/rtm/token_storage_secure.go

import (
	"encoding/json"
	"fmt" // Import fmt
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
func NewSecureTokenStorage(logger logging.Logger) *SecureTokenStorage {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	return &SecureTokenStorage{
		logger: logger.WithField("component", "secure_token_storage"),
	}
}

// IsAvailable checks if the OS keyring service is accessible.
func (s *SecureTokenStorage) IsAvailable() bool {
	_, err := keyring.Get(keyringService, keyringUser)

	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("Keyring service is accessible (no token found, which is normal for first use).") // Added period
			return true
		}
		s.logger.Warn("Keyring service is inaccessible or permissions are insufficient.", "error", err) // Added period
		return false
	}

	s.logger.Debug("Keyring service is accessible and contains an existing token.") // Added period
	return true
}

// LoadToken loads the token from the secure OS keyring.
func (s *SecureTokenStorage) LoadToken() (string, error) {
	s.logger.Info("Attempting to load authentication token from system keyring.", "service", keyringService, "account", keyringUser) // Log details before load
	jsonData, err := keyring.Get(keyringService, keyringUser)

	// *** ADDED DETAILED ERROR LOGGING HERE ***
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("keyring.Get: No authentication token found in system keyring (ErrNotFound).") // More specific log
			return "", nil                                                                                // Not an error, just not found.
		}
		// Log other errors from keyring.Get
		s.logger.Error("keyring.Get operation failed.", "error", fmt.Sprintf("%+v", err)) // Log error with stack trace
		return "", errors.Wrap(err, "failed to load token from system keyring")
	}
	// *** END ADDED LOGGING ***

	s.logger.Info("Successfully retrieved token data from system keyring.") // Added period

	var data TokenData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		s.logger.Error("Token data in keyring is corrupted and cannot be parsed, attempting deletion.", "error", err) // Added period
		_ = s.DeleteToken()                                                                                           // Attempt to delete corrupted entry.
		return "", errors.Wrap(err, "failed to parse token data from secure storage")
	}

	s.logger.Debug("Successfully decoded authentication token.", "username", data.Username) // Added period
	return data.Token, nil
}

// SaveToken saves token data securely to the OS keyring.
func (s *SecureTokenStorage) SaveToken(token string, userID, username string) error {
	s.logger.Debug("Preparing to save token to system keyring.", "username", username, "service", keyringService, "account", keyringUser) // Log details before save
	if token == "" {
		err := errors.New("cannot save empty token to keyring")
		s.logger.Error("SaveToken failed.", "error", err) // Added period
		return err
	}

	data := TokenData{
		Token:     token,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		s.logger.Error("Failed to encode token data for secure storage.", "error", err) // Added period
		return errors.Wrap(err, "failed to encode token data for secure storage")
	}

	jsonDataString := string(jsonData)
	s.logger.Debug("Attempting keyring.Set.", "service", keyringService, "user", keyringUser, "dataSize", len(jsonDataString)) // Added period

	// *** ADDED DETAILED ERROR LOGGING HERE ***
	err = keyring.Set(keyringService, keyringUser, jsonDataString)
	if err != nil {
		// Log the specific error returned by keyring.Set
		s.logger.Error("keyring.Set operation failed.", "error", fmt.Sprintf("%+v", err)) // Log error with stack trace if available
		// Optionally log context about potential causes on macOS
		s.logger.Warn("Potential macOS Keychain issues: Check Keychain Access permissions, ensure 'login' keychain is unlocked, or try locking/unlocking it.") // Added period
		return errors.Wrap(err, "failed to save token to system keyring")
	}
	// *** END ADDED LOGGING ***

	s.logger.Info("Authentication token successfully saved to system keyring.") // Added period
	return nil
}

// DeleteToken removes the token from the secure OS keyring.
func (s *SecureTokenStorage) DeleteToken() error {
	s.logger.Debug("Deleting authentication token from system keyring.") // Added period

	err := keyring.Delete(keyringService, keyringUser)

	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("No token to delete - system keyring entry not found.") // Added period
			return nil
		}
		s.logger.Error("Failed to delete token from system keyring.", "error", err) // Added period
		return errors.Wrap(err, "failed to delete token from system keyring")
	}

	s.logger.Info("Authentication token successfully deleted from system keyring.") // Added period
	return nil
}

// GetTokenData loads the full token data from the secure storage.
func (s *SecureTokenStorage) GetTokenData() (*TokenData, error) {
	jsonData, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil // Return nil data and nil error if not found.
		}
		s.logger.Error("Failed to load token data from keyring.", "error", err) // Added period
		return nil, errors.Wrap(err, "failed to load token data from keyring")
	}

	var data TokenData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		s.logger.Error("Failed to parse token data from secure storage, attempting deletion.", "error", err) // Added period
		_ = s.DeleteToken()                                                                                  // Attempt to delete corrupted entry.
		return nil, errors.Wrap(err, "failed to parse token data from secure storage")
	}

	return &data, nil
}

// UpdateToken updates the stored token data. Re-uses SaveToken as keyring.Set often overwrites.
func (s *SecureTokenStorage) UpdateToken(token string, userID, username string) error {
	// Keyring Set usually overwrites, so we can just call SaveToken.
	// If more complex update logic is needed (e.g., preserving CreatedAt),
	// load existing data first.
	s.logger.Debug("Updating token in secure storage.", "username", username) // Added period
	return s.SaveToken(token, userID, username)
}
