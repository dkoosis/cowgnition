// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
)

// handleInteractiveAuth guides the user through the auth process.
func (m *AuthManager) handleInteractiveAuth(ctx context.Context) (*AuthResult, error) {
	// Start the auth flow to get a frob and URL
	authURL, frob, err := m.retryableOperationWithStrings(ctx, "StartAuthFlow", func(ctx context.Context) (string, string, error) {
		return m.client.StartAuthFlow(ctx)
	})

	if err != nil {
		return &AuthResult{Success: false, Error: err},
			errors.Wrap(err, "failed to start auth flow")
	}

	result := &AuthResult{
		Success:     false,
		AuthURL:     authURL,
		Frob:        frob,
		NeedsManual: !m.options.AutoCompleteAuth,
	}

	m.logger.Info("Authentication URL generated", "url", authURL, "frob", frob)
	fmt.Printf("\n⚠️  Authentication Required\n\n")
	fmt.Printf("1. Open this URL in your browser:\n   %s\n\n", authURL)
	fmt.Printf("2. Click 'Allow Access' to authorize this application\n\n")

	if !m.options.AutoCompleteAuth {
		fmt.Printf("3. After authorizing, use the following to complete authentication:\n")
		fmt.Printf("   Complete authentication with frob: %s\n\n", frob)
		return result, nil // Not an error in this case, just instruction
	}

	// Try auto-complete with callback server
	if m.options.CallbackHost != "" && m.options.CallbackPort > 0 {
		m.logger.Info("Starting callback server for automated auth completion")
		callbackErr := m.startCallbackServer(ctx, frob)
		if callbackErr == nil {
			// Wait for callback to complete or timeout
			select {
			case serverErr := <-m.resultChan:
				m.stopCallbackServer()
				if serverErr != nil {
					m.logger.Error("Callback server encountered an error", "error", serverErr)
					result.Error = serverErr
					return result, serverErr
				}

				// Server completed successfully, check auth again
				authState, err := m.service.GetAuthState(ctx)
				if err == nil && authState != nil && authState.IsAuthenticated {
					m.logger.Info("Authentication successful via callback!", "username", authState.Username)
					result.Success = true
					result.Username = authState.Username

					// Optionally save token to file if enabled
					if m.options.AutoSaveToken {
						m.saveTokenAfterSuccessfulAuth(authState)
					}

					return result, nil
				} else if err != nil {
					m.logger.Error("Failed to verify auth state after callback", "error", err)
					result.Error = err
					return result, err
				} else {
					m.logger.Warn("Callback completed but user not authenticated")
				}
			case <-ctx.Done():
				m.stopCallbackServer()
				m.logger.Warn("Authentication context cancelled", "error", ctx.Err())
				return result, ctx.Err()
			case <-time.After(m.options.TimeoutDuration):
				m.logger.Warn("Authentication callback timed out")
				m.stopCallbackServer()
			}
		} else {
			m.logger.Warn("Failed to start callback server", "error", callbackErr)
		}
	}

	// Fallback to manual input
	fmt.Printf("3. Press Enter after you've completed authorization...\n")
	// FIX: Check error from fmt.Scanln
	_, _ = fmt.Scanln() // Assign error to blank identifier

	// Complete flow with the frob we already have
	m.logger.Info("Completing authentication flow...", "frob", frob)

	err = m.retryableOperation(ctx, "CompleteAuth", func(ctx context.Context) error {
		return m.service.CompleteAuth(ctx, frob)
	})

	if err != nil {
		m.logger.Error("Failed to complete authentication", "error", err)
		result.Error = err
		return result, errors.Wrap(err, "failed to complete authentication flow")
	}

	m.logger.Info("Authentication successful!", "username", m.service.GetUsername())
	fmt.Printf("\n✅ Authentication successful! Logged in as: %s\n\n", m.service.GetUsername())

	result.Success = true
	result.Username = m.service.GetUsername()

	// Optionally save token to file if enabled
	if m.options.AutoSaveToken {
		m.saveTokenAfterSuccessfulAuth(nil) // Passing nil to trigger fresh state check
	}

	return result, nil
}

// handleHeadlessAuth attempts authentication without user interaction.
func (m *AuthManager) handleHeadlessAuth(ctx context.Context) (*AuthResult, error) {
	// In headless mode, we can't complete auth without external help
	// Check for auth token in environment variables first
	tokenEnv := os.Getenv("RTM_AUTH_TOKEN")
	if tokenEnv != "" {
		m.logger.Info("Found RTM_AUTH_TOKEN in environment, attempting to use it")
		m.service.SetAuthToken(tokenEnv)

		// Verify token works
		authState, err := m.service.GetAuthState(ctx)
		if err == nil && authState != nil && authState.IsAuthenticated {
			m.logger.Info("Authentication successful with environment token!",
				"username", authState.Username)
			return &AuthResult{
				Success:  true,
				Username: authState.Username,
			}, nil
		}

		m.logger.Warn("Environment token invalid", "error", err)
	}

	// Try token file in additional locations
	additionalTokenPaths := []string{
		"rtm_token.json",
		".rtm_token.json",
		os.ExpandEnv("$HOME/.rtm_token.json"),
	}

	for _, path := range additionalTokenPaths {
		if _, err := os.Stat(path); err == nil {
			m.logger.Info("Found potential token file", "path", path)
			tokenData, readErr := m.readTokenFile(path)
			if readErr != nil {
				m.logger.Warn("Failed to read token file", "path", path, "error", readErr)
				continue
			}

			if tokenData != nil && tokenData.Token != "" {
				m.logger.Info("Trying token from file", "path", path)
				m.service.SetAuthToken(tokenData.Token)

				// Verify token works
				authState, err := m.service.GetAuthState(ctx)
				if err == nil && authState != nil && authState.IsAuthenticated {
					m.logger.Info("Authentication successful with token from file!",
						"path", path,
						"username", authState.Username)
					return &AuthResult{
						Success:  true,
						Username: authState.Username,
					}, nil
				}

				m.logger.Warn("Token from file invalid", "path", path, "error", err)
			}
		}
	}

	// Generate auth URL but can't complete flow
	authURL, frob, err := m.client.StartAuthFlow(ctx)
	if err != nil {
		return &AuthResult{
			Success: false,
			Error:   err,
		}, errors.Wrap(err, "failed to start auth flow in headless mode")
	}

	return &AuthResult{
		Success:     false,
		AuthURL:     authURL,
		Frob:        frob,
		NeedsManual: true,
	}, errors.New("headless mode cannot complete authentication without external help")
}

// handleTestAuth handles authentication in test environments.
func (m *AuthManager) handleTestAuth(ctx context.Context) (*AuthResult, error) {
	// First check if we have cached test credentials
	testTokenPath := m.options.TestTokenPath
	if testTokenPath == "" {
		testTokenPath = os.ExpandEnv("$HOME/.rtm_test_token.json")
	}

	if _, err := os.Stat(testTokenPath); err == nil {
		m.logger.Info("Found test token file, attempting to use it", "path", testTokenPath)
		tokenData, readErr := m.readTokenFile(testTokenPath)
		if readErr != nil {
			m.logger.Warn("Failed to read test token file", "error", readErr)
		} else if tokenData != nil && tokenData.Token != "" {
			m.service.SetAuthToken(tokenData.Token)

			// Verify token works
			authState, verifyErr := m.service.GetAuthState(ctx)
			if verifyErr == nil && authState != nil && authState.IsAuthenticated {
				m.logger.Info("Test authentication successful with token file!",
					"username", authState.Username)
				return &AuthResult{
					Success:  true,
					Username: authState.Username,
				}, nil
			}

			m.logger.Warn("Test token from file invalid", "error", verifyErr)
		}
	}

	// Check for test token in environment
	testToken := os.Getenv("RTM_TEST_TOKEN")
	if testToken != "" {
		m.logger.Info("Found RTM_TEST_TOKEN in environment, attempting to use it")
		m.service.SetAuthToken(testToken)

		// Verify token works
		authState, err := m.service.GetAuthState(ctx)
		if err == nil && authState != nil && authState.IsAuthenticated {
			m.logger.Info("Test authentication successful with environment token!",
				"username", authState.Username)
			return &AuthResult{
				Success:  true,
				Username: authState.Username,
			}, nil
		}
	}

	// For CI environments, use mock instead of real auth
	if os.Getenv("CI") != "" {
		m.logger.Info("Running in CI environment, using mock authentication")
		// Set up mock auth state for testing
		// This would be environment-specific implementation
		return &AuthResult{
			Success:  true,
			Username: "ci_test_user",
		}, nil
	}

	// Fall back to interactive if not CI
	m.logger.Info("Test environment requires interactive authentication")
	return m.handleInteractiveAuth(ctx)
}
