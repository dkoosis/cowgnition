// Package auth provides authentication utilities for the Remember The Milk service.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// TokenManager handles secure storage and retrieval of authentication tokens.
type TokenManager struct {
	tokenPath          string
	encryptionDisabled bool
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
		tokenPath:          tokenPath,
		encryptionDisabled: false,
	}, nil
}

// SaveToken securely writes a token to the token path.
func (tm *TokenManager) SaveToken(token string) error {
	var data []byte
	var err error

	if tm.encryptionDisabled {
		data = []byte(token)
	} else {
		// Encrypt the token before saving
		data, err = tm.encryptToken(token)
		if err != nil {
			return fmt.Errorf("error encrypting token: %w", err)
		}
	}

	return os.WriteFile(tm.tokenPath, data, 0600)
}

// LoadToken reads a token from the token path.
// Returns an error if the token file doesn't exist or can't be read.
func (tm *TokenManager) LoadToken() (string, error) {
	data, err := os.ReadFile(tm.tokenPath)
	if err != nil {
		return "", err
	}

	if tm.encryptionDisabled {
		return string(data), nil
	}

	// Decrypt the token after reading
	token, err := tm.decryptToken(data)
	if err != nil {
		return "", fmt.Errorf("error decrypting token: %w", err)
	}

	return token, nil
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
	tm.encryptionDisabled = true
}

// encryptToken encrypts a token string using AES-GCM.
// It uses a derived key from the machine-specific information.
func (tm *TokenManager) encryptToken(token string) ([]byte, error) {
	// Get encryption key based on machine-specific information
	key, err := tm.getDerivedKey()
	if err != nil {
		return nil, fmt.Errorf("error getting encryption key: %w", err)
	}

	// Create a new AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %w", err)
	}

	// Create a new GCM cipher mode
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error creating GCM: %w", err)
	}

	// Create a nonce
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("error generating nonce: %w", err)
	}

	// Encrypt the token
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(token), nil)

	// Encode the result in base64 for easy storage
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return []byte(encoded), nil
}

// decryptToken decrypts a token that was encrypted with encryptToken.
func (tm *TokenManager) decryptToken(data []byte) (string, error) {
	// Get the same encryption key used for encryption
	key, err := tm.getDerivedKey()
	if err != nil {
		return "", fmt.Errorf("error getting encryption key: %w", err)
	}

	// Create a new AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("error creating cipher: %w", err)
	}

	// Create a new GCM cipher mode
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating GCM: %w", err)
	}

	// Decode the base64 data
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return "", fmt.Errorf("error decoding base64: %w", err)
	}

	// Get the nonce size
	nonceSize := aesGCM.NonceSize()
	if len(decoded) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract the nonce and ciphertext
	nonce, ciphertext := decoded[:nonceSize], decoded[nonceSize:]

	// Decrypt the token
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("error decrypting token: %w", err)
	}

	return string(plaintext), nil
}

// getDerivedKey generates a 32-byte key derived from machine-specific information.
// This ensures the token can only be decrypted on the same machine.
func (tm *TokenManager) getDerivedKey() ([]byte, error) {
	// Use a combination of hostname, username, and token path as the key seed
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}
	if username == "" {
		username = "unknown"
	}

	// Create a stable seed for the key
	seed := hostname + username + tm.tokenPath

	// Hash the seed to get a 32-byte key (SHA-256)
	hash := sha256.Sum256([]byte(seed))
	return hash[:], nil
}

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
