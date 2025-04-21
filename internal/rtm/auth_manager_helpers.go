// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// file: internal/rtm/auth_manager_helpers.go
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

// standardTokenPaths lists common filenames for RTM tokens.
var standardTokenPaths = []string{
	"rtm_token.json",
	".rtm_token.json",
	"rtm_test_token.json",
	".rtm_test_token.json",
}

// generateStateToken creates a secure random token for CSRF protection.
func generateStateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback is not cryptographically secure but adequate for CSRF protection.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// findExistingTokens searches multiple standard locations for valid tokens.
// Returns the valid token, username, and nil error if found, otherwise error.
func (m *AuthManager) findExistingTokens(ctx context.Context) (string, string, error) {
	// First check environment variables.
	envTokens := []string{
		"RTM_AUTH_TOKEN",
		"RTM_TEST_TOKEN",
	}

	for _, envName := range envTokens {
		token := os.Getenv(envName)
		if token != "" {
			m.logger.Info("Found token in environment.", "env", envName)
			m.service.SetAuthToken(token) // Set temporarily for verification.

			// Verify it works.
			authState, err := m.service.GetAuthState(ctx)
			if err == nil && authState != nil && authState.IsAuthenticated {
				m.logger.Info("Environment token valid.",
					"env", envName,
					"username", authState.Username)
				return token, authState.Username, nil // Return the valid token.
			}

			m.logger.Warn("Environment token invalid.",
				"env", envName,
				"error", err)
			m.service.SetAuthToken("") // Clear invalid token.
		}
	}

	// Check standard file locations.
	paths := standardTokenPaths

	// Add user home directory paths.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths,
			filepath.Join(homeDir, ".rtm_token.json"),
			filepath.Join(homeDir, ".rtm_test_token.json"),
			filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json"))
	}

	// Check all paths.
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			m.logger.Info("Found token file.", "path", path)

			tokenData, readErr := m.readTokenFile(path)
			if readErr != nil {
				m.logger.Warn("Failed to read token file.",
					"path", path,
					"error", readErr)
				continue
			}

			if tokenData == nil || tokenData.Token == "" {
				m.logger.Warn("Token file contains no valid token.", "path", path)
				continue
			}

			// Try using the token.
			m.service.SetAuthToken(tokenData.Token)
			authState, verifyErr := m.service.GetAuthState(ctx)
			if verifyErr == nil && authState != nil && authState.IsAuthenticated {
				m.logger.Info("File token valid.",
					"path", path,
					"username", authState.Username)
				return tokenData.Token, authState.Username, nil // Return the valid token.
			}

			m.logger.Warn("Token from file invalid.",
				"path", path,
				"error", verifyErr)
			m.service.SetAuthToken("") // Clear invalid token.
		}
	}

	return "", "", errors.New("no valid tokens found in standard locations")
}

// saveTokenToAllLocations attempts to save the token to multiple standard locations.
func (m *AuthManager) saveTokenToAllLocations(token, username string) {
	if token == "" {
		m.logger.Warn("Cannot save empty token.")
		return
	}

	// Create token data.
	tokenData := &TokenData{
		Token:     token,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Try to save in multiple locations to ensure at least one succeeds.
	saveLocations := []string{
		"rtm_token.json", // Current directory.
	}

	// Add home directory location if possible.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(homeDir, ".config", "cowgnition")
		saveLocations = append(saveLocations,
			filepath.Join(configDir, "rtm_token.json"),
			filepath.Join(homeDir, ".rtm_token.json"),
			filepath.Join(homeDir, ".rtm_test_token.json")) // Save test token too.

		// Create config directory if needed.
		_ = os.MkdirAll(configDir, 0700) // Ignore error.
	}

	// Save to all locations.
	saved := false
	for _, path := range saveLocations {
		err := m.saveTokenToFile(path, tokenData)
		if err == nil {
			m.logger.Info("Successfully saved token.", "path", path)
			saved = true
		} else {
			m.logger.Warn("Failed to save token.", "path", path, "error", err)
		}
	}
	if !saved {
		m.logger.Error("Failed to save token to ANY standard location.")
	}
}

// saveTokenAfterSuccessfulAuth saves the current auth token to standard locations.
func (m *AuthManager) saveTokenAfterSuccessfulAuth(authState *AuthState) {
	token := m.client.GetAuthToken()
	if token == "" {
		m.logger.Warn("Cannot save empty token.")
		return
	}

	// Get or refresh auth state if needed.
	var username string
	if authState == nil || authState.Username == "" {
		var err error
		// Use a background context as this might happen after main context expires.
		authState, err = m.service.GetAuthState(context.Background())
		if err != nil {
			m.logger.Warn("Failed to get auth state for token saving.", "error", err)
			// Proceed without username, token is the important part.
		}
	}
	if authState != nil {
		username = authState.Username
	}

	// Save to all standard locations.
	m.saveTokenToAllLocations(token, username)
}

// readTokenFile reads and parses a token file.
func (m *AuthManager) readTokenFile(path string) (*TokenData, error) {
	// #nosec G304 -- Path comes from internal list or config.
	data, err := os.ReadFile(path)
	if err != nil {
		// Don't wrap if it's just NotExist, return it directly.
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, errors.Wrapf(err, "failed to read token file: %s", path)
	}

	var tokenData TokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		return nil, errors.Wrapf(err, "failed to parse token data from file: %s", path)
	}

	return &tokenData, nil
}

// saveTokenToFile saves token data to a specific file path.
func (m *AuthManager) saveTokenToFile(path string, tokenData *TokenData) error {
	// Create directory if it doesn't exist.
	dir := filepath.Dir(path)
	if dir != "." && dir != "" { // Avoid trying to create current dir.
		if err := os.MkdirAll(dir, 0700); err != nil {
			return errors.Wrapf(err, "failed to create directory for token file: %s", dir)
		}
	}

	// Marshal to JSON.
	data, err := json.MarshalIndent(tokenData, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token data to JSON")
	}

	// Write to file with secure permissions.
	// #nosec G306 -- Permissions are intentionally 0600.
	if err := os.WriteFile(path, data, 0600); err != nil {
		return errors.Wrapf(err, "failed to write token file: %s", path)
	}

	return nil
}

// retryableOperationWithStrings provides retry logic for operations returning strings.
func (m *AuthManager) retryableOperationWithStrings(ctx context.Context, opName string, fn func(context.Context) (string, string, error)) (string, string, error) {
	m.retryMutex.Lock()
	m.retryCount[opName] = 0 // Reset count for this operation.
	m.retryMutex.Unlock()

	var lastErr error
	var resultS1, resultS2 string // Use different names to avoid shadowing.

	for attempt := 0; attempt <= m.options.RetryAttempts; attempt++ {
		// Add delay for retries.
		if attempt > 0 {
			m.logger.Debug("Retrying operation.", "operation", opName, "attempt", attempt)
			select {
			case <-ctx.Done():
				return "", "", ctx.Err() // Respect context cancellation.
			case <-time.After(m.options.RetryBackoff * time.Duration(attempt)):
				// Simple linear backoff.
			}
		}

		m.retryMutex.Lock()
		m.retryCount[opName] = attempt // Update current attempt count.
		m.retryMutex.Unlock()

		// Assign results directly.
		s1, s2, err := fn(ctx) // Assign results from the function call.

		if err == nil {
			// Assign to outer scope variables on success before returning.
			resultS1 = s1
			resultS2 = s2
			return resultS1, resultS2, nil // Return the actual results.
		}

		lastErr = err
		m.logger.Warn("Operation failed with error.",
			"operation", opName,
			"attempt", attempt,
			"error", err)
	}

	// All attempts failed.
	return "", "", errors.Wrapf(lastErr, "operation %s failed after %d attempts", // Return empty strings on final failure.
		opName, m.options.RetryAttempts+1)
}

// Removed the unused retryableOperation function.
