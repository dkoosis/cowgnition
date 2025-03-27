// internal/rtm/token.go
package rtm

import (
	"os"
	"path/filepath"
	"sync"

	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
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
		return nil, cgerr.NewAuthError(
			"Failed to create token directory",
			err,
			map[string]interface{}{
				"token_path": tokenPath,
				"directory":  dir,
			},
		)
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
		return cgerr.NewAuthError(
			"Failed to write token file",
			err,
			map[string]interface{}{
				"token_path": s.TokenPath,
			},
		)
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
		return "", cgerr.NewAuthError(
			"Failed to read token file",
			err,
			map[string]interface{}{
				"token_path": s.TokenPath,
			},
		)
	}

	return string(data), nil
}
