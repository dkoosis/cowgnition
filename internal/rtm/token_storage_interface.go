// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/token_storage_interface.go

import "github.com/dkoosis/cowgnition/internal/logging"

// TokenStorage defines the interface for storing and retrieving auth tokens.
type TokenStorageInterface interface {
	// SaveToken saves a token with associated user information.
	SaveToken(token string, userID, username string) error

	// LoadToken loads the stored token (if any).
	LoadToken() (string, error)

	// DeleteToken removes any stored token.
	DeleteToken() error

	// GetTokenData gets the full token data including user information.
	GetTokenData() (*TokenData, error)

	// UpdateToken updates an existing token entry with new information.
	UpdateToken(token string, userID, username string) error
}

// NewTokenStorage creates the most appropriate token storage implementation.
// It will try to use secure storage if available, falling back to file-based storage.
func NewTokenStorage(tokenPath string, logger logging.Logger) (TokenStorageInterface, error) {
	// First try to use secure storage
	secureStorage := NewSecureTokenStorage(logger)

	// Check if secure storage is available
	if secureStorage.IsAvailable() {
		logger.Info("Using secure token storage (OS keyring)")
		return secureStorage, nil
	}

	// Fall back to file-based storage
	logger.Info("Secure token storage not available, falling back to file-based storage",
		"path", tokenPath)
	return NewFileTokenStorage(tokenPath, logger)
}
