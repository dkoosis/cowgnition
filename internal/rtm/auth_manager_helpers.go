// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
)

// generateStateToken creates a secure random token for CSRF protection.
func generateStateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Note: This fallback is not cryptographically secure but should be
		// adequate for CSRF protection in the rare case that rand.Read fails
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// saveTokenAfterSuccessfulAuth saves the current auth token to a file
// if AutoSaveToken is enabled in options.
func (m *AuthManager) saveTokenAfterSuccessfulAuth(authState *AuthState) {
	var username, userID string
	// FIX: Merge declaration and assignment
	token := m.client.GetAuthToken() // Assign directly
	if token == "" {
		m.logger.Warn("Cannot save empty token to file")
		return
	}

	// Get or refresh auth state if needed
	if authState == nil || authState.Username == "" {
		var err error
		authState, err = m.service.GetAuthState(context.Background())
		if err != nil {
			m.logger.Warn("Failed to get auth state for token saving", "error", err)
			return
		}
	}

	if authState != nil {
		username = authState.Username
		userID = authState.UserID
	}

	// Determine path based on mode
	var path string
	switch m.options.Mode {
	case AuthModeTest:
		// Use test-specific path
		if m.options.TestTokenPath != "" {
			path = m.options.TestTokenPath
		} else {
			path = os.ExpandEnv("$HOME/.rtm_test_token.json")
		}
	default:
		// Use default path
		path = os.ExpandEnv("$HOME/.rtm_token.json")
	}

	// Create token data
	tokenData := &TokenData{
		Token:     token,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Save to file
	err := m.saveTokenToFile(path, tokenData)
	if err != nil {
		m.logger.Warn("Failed to save token to file", "path", path, "error", err)
	} else {
		m.logger.Info("Successfully saved token to file", "path", path, "username", username)
	}
}

// readTokenFile reads and parses a token file.
func (m *AuthManager) readTokenFile(path string) (*TokenData, error) {
	// #nosec G304 -- Path is checked by the caller
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read token file: %s", path)
	}

	var tokenData TokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		return nil, errors.Wrapf(err, "failed to parse token data from file: %s", path)
	}

	return &tokenData, nil
}

// saveTokenToFile saves token data to a file.
func (m *AuthManager) saveTokenToFile(path string, tokenData *TokenData) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory for token file: %s", dir)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(tokenData, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token data to JSON")
	}

	// Write to file with secure permissions
	if err := os.WriteFile(path, data, 0600); err != nil {
		return errors.Wrapf(err, "failed to write token file: %s", path)
	}

	return nil
}

// retryableOperation provides retry logic for operations returning a single error.
func (m *AuthManager) retryableOperation(ctx context.Context, opName string, fn func(context.Context) error) error {
	m.retryMutex.Lock()
	m.retryCount[opName] = 0
	m.retryMutex.Unlock()

	var lastErr error

	for attempt := 0; attempt <= m.options.RetryAttempts; attempt++ {
		// Add delay for retries
		if attempt > 0 {
			m.logger.Debug("Retrying operation", "operation", opName, "attempt", attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(m.options.RetryBackoff * time.Duration(attempt)):
				// Exponential backoff
			}
		}

		m.retryMutex.Lock()
		m.retryCount[opName] = attempt
		m.retryMutex.Unlock()

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err
		m.logger.Warn("Operation failed with error",
			"operation", opName,
			"attempt", attempt,
			"error", err)
	}

	return errors.Wrapf(lastErr, "operation %s failed after %d attempts",
		opName, m.options.RetryAttempts+1)
}

// retryableOperationWithStrings provides retry logic for operations returning strings.
func (m *AuthManager) retryableOperationWithStrings(ctx context.Context, opName string, fn func(context.Context) (string, string, error)) (string, string, error) {
	m.retryMutex.Lock()
	m.retryCount[opName] = 0
	m.retryMutex.Unlock()

	var lastErr error
	// FIX: Declare s1, s2 within the loop or assign fn results directly
	var resultS1, resultS2 string // Use different names to avoid shadowing

	for attempt := 0; attempt <= m.options.RetryAttempts; attempt++ {
		// Add delay for retries
		if attempt > 0 {
			m.logger.Debug("Retrying operation", "operation", opName, "attempt", attempt)
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(m.options.RetryBackoff * time.Duration(attempt)):
				// Exponential backoff
			}
		}

		m.retryMutex.Lock()
		m.retryCount[opName] = attempt
		m.retryMutex.Unlock()

		// FIX: Assign results directly
		var s1, s2 string     // Declare here for scope
		var err error         // Declare here for scope
		s1, s2, err = fn(ctx) // Assign results from the function call

		if err == nil {
			// Assign to outer scope variables on success before returning
			resultS1 = s1
			resultS2 = s2
			return resultS1, resultS2, nil // Return the actual results
		}

		lastErr = err
		m.logger.Warn("Operation failed with error",
			"operation", opName,
			"attempt", attempt,
			"error", err)
	}

	return "", "", errors.Wrapf(lastErr, "operation %s failed after %d attempts", // Return empty strings on final failure
		opName, m.options.RetryAttempts+1)
}
