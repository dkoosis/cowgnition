// internal/rtm/token.go
package rtm

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors" // Ensure errors is imported
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
			errors.Wrap(err, "could not create directory"),
			map[string]interface{}{
				"token_path": tokenPath,
				"directory":  dir,
				"permission": "0755",
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
			errors.Wrap(err, "could not write file"),
			map[string]interface{}{
				"token_path":  s.TokenPath,
				"permission":  "0600",
				"token_bytes": len(token),
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
		// Replace os.IsNotExist(err) with errors.Is(err, os.ErrNotExist)
		if errors.Is(err, os.ErrNotExist) {
			return "", nil // Not an error if file doesn't exist yet
		}
		// Also update the check within the error details map for consistency
		return "", cgerr.NewAuthError(
			"Failed to read token file",
			errors.Wrap(err, "could not read file"),
			map[string]interface{}{
				"token_path":  s.TokenPath,
				"file_exists": !errors.Is(err, os.ErrNotExist), // Use errors.Is here too
			},
		)
	}

	return string(data), nil
}
