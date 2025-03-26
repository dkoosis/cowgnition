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
		return nil, fmt.Errorf("failed to create token directory: %w", err)
	}

	return &TokenStorage{
		TokenPath: tokenPath,
	}, nil
}

// SaveToken saves a token to the token file.
func (s *TokenStorage) SaveToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return os.WriteFile(s.TokenPath, []byte(token), 0600)
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
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	return string(data), nil
}
