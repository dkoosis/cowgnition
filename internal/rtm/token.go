// Package rtm provides a client for interacting with the Remember The Milk (RTM) API v2.
// This file specifically deals with persistent storage of the RTM authentication token.
package rtm

import (
	"os"            // Provides functions for interacting with the operating system, used here for file I/O.
	"path/filepath" // Used for manipulating filesystem paths (getting directory).
	"sync"          // Provides basic synchronization primitives like mutexes.

	"github.com/cockroachdb/errors"                           // Error handling library with wrapping capabilities.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // Project's custom error types.
)

// TokenStorage handles the persistent storage and retrieval of an RTM authentication token
// to/from a specified file path. It ensures thread-safe access to the token file.
type TokenStorage struct {
	// TokenPath is the fully qualified path to the file where the RTM token is stored.
	TokenPath string
	// mu is a mutex to synchronize access to the token file, preventing race conditions
	// during read and write operations.
	mu sync.Mutex
}

// NewTokenStorage creates a new TokenStorage instance for the given file path.
// It ensures that the directory containing the token file exists, creating it if necessary.
// Returns an error if the directory cannot be created.
func NewTokenStorage(tokenPath string) (*TokenStorage, error) {
	// Ensure the directory for the token file exists.
	dir := filepath.Dir(tokenPath)
	// Create all necessary parent directories with default permissions (0755).
	if err := os.MkdirAll(dir, 0755); err != nil {
		// Wrap the underlying filesystem error with context.
		return nil, cgerr.NewAuthError(
			"Failed to create token directory.",
			errors.Wrap(err, "could not create directory for token storage"),
			map[string]interface{}{
				"token_path": tokenPath,
				"directory":  dir,
				"permission": "0755",
			},
		)
	}

	// Return the initialized TokenStorage.
	return &TokenStorage{
		TokenPath: tokenPath,
	}, nil
}

// SaveToken securely saves the provided authentication token string to the file
// specified by TokenPath. It overwrites the file if it already exists.
// File permissions are set to 0600 (read/write for owner only).
// Returns an error if the file cannot be written.
func (s *TokenStorage) SaveToken(token string) error {
	// Lock the mutex to ensure exclusive access to the file during the write operation.
	s.mu.Lock()
	// Defer unlocking the mutex so it's released even if errors occur.
	defer s.mu.Unlock()

	// Write the token data to the file with restricted permissions (0600).
	if err := os.WriteFile(s.TokenPath, []byte(token), 0600); err != nil {
		// Wrap the underlying filesystem error with context.
		return cgerr.NewAuthError(
			"Failed to write token file.",
			errors.Wrap(err, "could not write token to file"),
			map[string]interface{}{
				"token_path":  s.TokenPath,
				"permission":  "0600",
				"token_bytes": len(token), // Include token length for context, not the token itself.
			},
		)
	}
	// Return nil on successful write.
	return nil
}

// LoadToken reads the authentication token from the file specified by TokenPath.
// If the file does not exist, it returns an empty string and no error, indicating
// that no token is currently stored.
// Returns the token string and a nil error on success, or an empty string and
// an error if reading fails for reasons other than the file not existing.
func (s *TokenStorage) LoadToken() (string, error) {
	// Lock the mutex to ensure exclusive access during the read operation.
	s.mu.Lock()
	// Defer unlocking the mutex.
	defer s.mu.Unlock()

	// Attempt to read the entire content of the token file.
	data, err := os.ReadFile(s.TokenPath)
	if err != nil {
		// Check if the error is specifically os.ErrNotExist.
		if errors.Is(err, os.ErrNotExist) {
			// File not found is not considered an error in this context; it just means no token is saved yet.
			return "", nil
		}
		// For any other read error, wrap it and return.
		return "", cgerr.NewAuthError(
			"Failed to read token file.",
			errors.Wrap(err, "could not read token from file"),
			map[string]interface{}{
				"token_path": s.TokenPath,
				// Use errors.Is for the existence check for robustness.
				"file_exists": !errors.Is(err, os.ErrNotExist),
			},
		)
	}

	// Return the file content as a string.
	return string(data), nil
}
