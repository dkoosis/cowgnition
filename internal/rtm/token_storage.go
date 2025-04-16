// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/token_storage.go

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// FileTokenStorage handles the storage of authentication tokens in a file.
// This is a simple file-based implementation used as a fallback when
// secure OS-specific storage is not available.
type FileTokenStorage struct {
	path   string
	logger logging.Logger
	mutex  sync.RWMutex
}

// TokenData represents the data stored for an authentication token.
type TokenData struct {
	Token     string    `json:"token"`
	UserID    string    `json:"userId,omitempty"`
	Username  string    `json:"username,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewFileTokenStorage creates a new token storage instance.
func NewFileTokenStorage(path string, logger logging.Logger) (*FileTokenStorage, error) {
	// Use no-op logger if not provided
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	// Create the directory if it doesn't exist
	err := os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create token directory")
	}

	return &FileTokenStorage{
		path:   path,
		logger: logger.WithField("component", "file_token_storage"),
	}, nil
}

// SaveToken saves an authentication token.
func (s *FileTokenStorage) SaveToken(token string, userID, username string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Debug("Saving auth token to file", "username", username)

	// Create token data
	data := TokenData{
		Token:     token,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token data")
	}

	// Write to file with secure permissions
	err = os.WriteFile(s.path, jsonData, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write token file")
	}

	return nil
}

// LoadToken loads the authentication token if available.
func (s *FileTokenStorage) LoadToken() (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Check if file exists
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		s.logger.Debug("Token file does not exist")
		return "", nil
	}

	// Read token file
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", errors.Wrap(err, "failed to read token file")
	}

	// Parse JSON
	var tokenData TokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		return "", errors.Wrap(err, "failed to parse token data")
	}

	s.logger.Debug("Loaded auth token from file", "username", tokenData.Username)

	return tokenData.Token, nil
}

// DeleteToken removes the stored token.
func (s *FileTokenStorage) DeleteToken() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		s.logger.Debug("Token file does not exist, nothing to delete")
		return nil
	}

	s.logger.Debug("Deleting auth token file")

	// Remove the file
	if err := os.Remove(s.path); err != nil {
		return errors.Wrap(err, "failed to delete token file")
	}

	return nil
}

// GetTokenData loads the full token data if available.
func (s *FileTokenStorage) GetTokenData() (*TokenData, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Check if file exists
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		s.logger.Debug("Token file does not exist")
		return nil, nil
	}

	// Read token file
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read token file")
	}

	// Parse JSON
	var tokenData TokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		return nil, errors.Wrap(err, "failed to parse token data")
	}

	return &tokenData, nil
}

// UpdateToken updates an existing token with new metadata.
func (s *FileTokenStorage) UpdateToken(token string, userID, username string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		// If file doesn't exist, just save a new one
		return s.SaveToken(token, userID, username)
	}

	// Load existing data
	tokenData, err := s.GetTokenData()
	if err != nil {
		return errors.Wrap(err, "failed to get existing token data")
	}

	// Update the token data
	tokenData.Token = token
	if userID != "" {
		tokenData.UserID = userID
	}
	if username != "" {
		tokenData.Username = username
	}
	tokenData.UpdatedAt = time.Now().UTC()

	// Convert to JSON
	jsonData, err := json.MarshalIndent(tokenData, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token data")
	}

	// Write to file with secure permissions
	err = os.WriteFile(s.path, jsonData, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write token file")
	}

	return nil
}
