// internal/rtm/token.go
package rtm

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TokenStorage handles storing and retrieving auth tokens.
type TokenStorage struct {
	TokenPath string
	mu        sync.Mutex
}

// NewTokenStorage creates a new token storage.
func NewTokenStorage(tokenPath string) (*TokenStorage, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("TokenStorage.NewTokenStorage: failed to create token directory: %w", err)
	}

	return &TokenStorage{
		TokenPath: tokenPath,
	}, nil
}

// SaveToken saves a token to the token file.
func (s *TokenStorage) SaveToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.WriteFile(s.TokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("TokenStorage.SaveToken: failed to write token file: %w", err)
	}
	return nil
}

// LoadToken loads a token from the token file.
func (s *TokenStorage) LoadToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.TokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("TokenStorage.LoadToken: failed to read token file: %w", err)
	}

	return string(data), nil
}

// ErrorMsgEnhanced:2025-03-26
