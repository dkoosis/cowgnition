// Package auth provides authentication utilities for the Remember The Milk service.
package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TokenManager handles secure storage and retrieval of authentication tokens.
type TokenManager struct {
	tokenPath string
}

// NewTokenManager creates a new token manager.
// It ensures the token directory exists before returning.
func NewTokenManager(tokenPath string) (*TokenManager, error) {
	// Create the directory if it doesn't exist
	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("error creating token directory: %w", err)
	}

	return &TokenManager{
		tokenPath: tokenPath,
	}, nil
}

// SaveToken securely writes a token to the token path.
func (tm *TokenManager) SaveToken(token string) error {
	return os.WriteFile(tm.tokenPath, []byte(token), 0600)
}

// LoadToken reads a token from the token path.
// Returns an error if the token file doesn't exist or can't be read.
func (tm *TokenManager) LoadToken() (string, error) {
	data, err := os.ReadFile(tm.tokenPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeleteToken removes the token file if it exists.
// Does nothing if the file doesn't exist.
func (tm *TokenManager) DeleteToken() error {
	// Check if the file exists first
	if _, err := os.Stat(tm.tokenPath); os.IsNotExist(err) {
		return nil // Already doesn't exist
	}
	return os.Remove(tm.tokenPath)
}

// HasToken checks if a token exists at the token path.
func (tm *TokenManager) HasToken() bool {
	_, err := os.Stat(tm.tokenPath)
	return err == nil
}

// GetTokenFileInfo returns file information about the token file.
// This is useful for checking when the token was last modified.
func (tm *TokenManager) GetTokenFileInfo() (os.FileInfo, error) {
	return os.Stat(tm.tokenPath)
}

// DisableEncryption disables token encryption for testing purposes.
// This should never be used in production.
func (tm *TokenManager) DisableEncryption() {
	// This is a placeholder since encryption is not implemented yet
	// In a full implementation, this would disable the encryption mechanism
}

// Add this function to internal/auth/token_manager.go

// GenerateTokenFilename creates a unique token filename based on the API key.
// This helps avoid collisions when multiple applications use the token manager.
func GenerateTokenFilename(apiKey string) string {
	// Use first 8 chars of API key as identifier if available
	identifier := apiKey
	if len(identifier) > 8 {
		identifier = identifier[:8]
	}

	// Create a unique filename with timestamp to avoid collisions
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("rtm_token_%s_%d", identifier, timestamp)
}
