// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/token_storage.go.

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

// NewFileTokenStorage creates a new token storage instance.
func NewFileTokenStorage(path string, logger logging.Logger) (*FileTokenStorage, error) {
	// Use no-op logger if not provided.
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	// Create the directory if it doesn't exist.
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

	s.logger.Debug("Saving auth token to file.", "username", username, "path", s.path)

	// Create token data.
	data := TokenData{
		Token:     token,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Convert to JSON.
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token data")
	}

	// Write to file with secure permissions.
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

	s.logger.Info("Attempting to load token from file...", "path", s.path)

	// Check if file exists.
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		s.logger.Debug("Token file does not exist.", "path", s.path)
		return "", nil
	}

	// Read token file.
	data, err := os.ReadFile(s.path)
	if err != nil {
		s.logger.Error("Failed to read token file.", "path", s.path, "error", err)
		return "", errors.Wrap(err, "failed to read token file")
	}
	s.logger.Info("Successfully loaded token data from file.", "path", s.path)

	// Parse JSON.
	var tokenData TokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		s.logger.Error("Failed to parse token data from file.", "path", s.path, "error", err)
		return "", errors.Wrap(err, "failed to parse token data")
	}

	s.logger.Debug("Parsed auth token from file.", "username", tokenData.Username)

	return tokenData.Token, nil
}

// DeleteToken removes the stored token.
func (s *FileTokenStorage) DeleteToken() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if file exists.
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		s.logger.Debug("Token file does not exist, nothing to delete.", "path", s.path)
		return nil
	}

	s.logger.Debug("Deleting auth token file.", "path", s.path)

	// Remove the file.
	if err := os.Remove(s.path); err != nil {
		s.logger.Error("Failed to delete token file.", "path", s.path, "error", err)
		return errors.Wrap(err, "failed to delete token file")
	}

	return nil
}

// GetTokenData loads the full token data if available.
func (s *FileTokenStorage) GetTokenData() (*TokenData, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Check if file exists.
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		s.logger.Debug("Token file does not exist.", "path", s.path)
		return nil, nil
	}

	// Read token file.
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read token file")
	}

	// Parse JSON.
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

	// Check if file exists.
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		// If file doesn't exist, just save a new one.
		return s.SaveToken(token, userID, username)
	}

	// Load existing data.
	tokenData, err := s.GetTokenData() // Changed from direct load to use GetTokenData.
	if err != nil {
		return errors.Wrap(err, "failed to get existing token data")
	}
	if tokenData == nil { // If file existed but couldn't be parsed/read by GetTokenData.
		tokenData = &TokenData{CreatedAt: time.Now().UTC()} // Start fresh but preserve potential CreatedAt conceptually? Or just save new. Let's just save new.
		s.logger.Warn("Existing token file seemed present but unreadable, overwriting.")
		return s.SaveToken(token, userID, username)
	}

	// Update the token data.
	tokenData.Token = token
	if userID != "" {
		tokenData.UserID = userID
	}
	if username != "" {
		tokenData.Username = username
	}
	tokenData.UpdatedAt = time.Now().UTC()

	// Convert to JSON.
	jsonData, err := json.MarshalIndent(tokenData, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token data")
	}

	// Write to file with secure permissions.
	err = os.WriteFile(s.path, jsonData, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write token file")
	}

	return nil
}

// IsAvailable checks if the underlying storage mechanism is functional.
// For file storage, it's always considered available if the struct was created.
func (s *FileTokenStorage) IsAvailable() bool {
	return true // File storage is always assumed available once created.
}
