// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/auth_manager_modes.go

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
)

// handleInteractiveAuth guides the user through the auth process.
// This function should primarily be called by user-facing tools like `rtm_connection_test` or `cowgnition setup`.
// Updated to return *EnsureAuthResult.
func (m *AuthManager) handleInteractiveAuth(ctx context.Context) (*EnsureAuthResult, error) {
	// Start the auth flow to get a frob and URL.
	authURL, frob, err := m.retryableOperationWithStrings(ctx, "StartAuthFlow", func(ctx context.Context) (string, string, error) {
		return m.client.StartAuthFlow(ctx)
	})

	if err != nil {
		// Return EnsureAuthResult with error.
		return &EnsureAuthResult{Success: false, Error: err},
			errors.Wrap(err, "failed to start auth flow")
	}

	// Initialize EnsureAuthResult.
	result := &EnsureAuthResult{
		Success:     false,
		AuthURL:     authURL,
		Frob:        frob,
		NeedsManual: !m.options.AutoCompleteAuth,
	}

	m.logger.Info("Authentication URL generated.",
		"url", authURL,
		"frob", frob,
		"autoComplete", m.options.AutoCompleteAuth)

	fmt.Printf("\n⚠️  Authentication Required\n\n")
	fmt.Printf("1. Open this URL in your browser:\n   %s\n\n", authURL)
	fmt.Printf("2. Click 'Allow Access' to authorize this application.\n\n")

	if !m.options.AutoCompleteAuth {
		fmt.Printf("3. After authorizing, use the following to complete authentication:\n")
		fmt.Printf("   Complete authentication with frob: %s\n\n", frob)
		// Return success=false, but no Go error, just instructions.
		return result, nil
	}

	// Try auto-complete with callback server.
	if m.options.CallbackHost != "" && m.options.CallbackPort > 0 {
		m.logger.Info("Starting callback server for automated auth completion.",
			"host", m.options.CallbackHost,
			"port", m.options.CallbackPort)

		// Pass background context as callback server runs independently.
		callbackErr := m.startCallbackServer(context.Background(), frob)
		if callbackErr == nil {
			// Wait for callback to complete or timeout.
			select {
			case serverErr := <-m.resultChan:
				m.stopCallbackServer()
				if serverErr != nil {
					m.logger.Error("Callback server encountered an error.", "error", serverErr)
					result.Error = serverErr // Set Error field on EnsureAuthResult.
					// Don't return error yet - try manual completion as fallback.
				} else {
					// Server completed successfully, check auth again.
					authState, checkErr := m.service.GetAuthState(ctx)
					if checkErr == nil && authState != nil && authState.IsAuthenticated {
						m.logger.Info("Authentication successful via callback!.", "username", authState.Username)
						result.Success = true
						result.Username = authState.Username
						if m.options.AutoSaveToken {
							m.saveTokenToStorage(m.service.GetAuthToken(), authState.UserID, authState.Username)
						}
						return result, nil // Success!.
					} else if checkErr != nil {
						m.logger.Error("Failed to verify auth state after callback.", "error", checkErr)
						result.Error = checkErr // Set Error field.
						// Don't return yet - try manual completion as fallback.
					} else {
						m.logger.Warn("Callback completed but user not authenticated - trying manual completion.")
					}
				}
			case <-ctx.Done():
				m.stopCallbackServer()
				m.logger.Warn("Authentication context cancelled.", "error", ctx.Err())
				return result, ctx.Err()
			case <-time.After(m.options.TimeoutDuration):
				m.logger.Warn("Authentication callback timed out - trying manual completion.",
					"timeout", m.options.TimeoutDuration.String())
				m.stopCallbackServer()
			}
		} else {
			m.logger.Warn("Failed to start callback server - falling back to manual.", "error", callbackErr)
		}
	}

	// Fallback to manual confirmation.
	fmt.Printf("3. Automatic completion couldn't be established or timed out.\n")
	fmt.Printf("   Press Enter after you've completed authorization in your browser...\n")
	_, scanErr := fmt.Scanln()
	if scanErr != nil {
		m.logger.Warn("Failed to read user input for manual confirmation.", "error", scanErr)
	}

	time.Sleep(2 * time.Second)

	m.logger.Info("Completing authentication flow...", "frob", frob)

	const maxRetries = 3
	var authErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(500*attempt) * time.Millisecond
			m.logger.Info("Retrying auth completion.", "attempt", attempt+1, "delay", delay.String())
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
			}
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			authErr = m.service.CompleteAuth(ctx, frob)
			if authErr == nil {
				break // Success.
			}
			m.logger.Warn("Auth completion attempt failed.",
				"attempt", attempt+1,
				"error", authErr.Error())
		}
	}

	if authErr != nil {
		m.logger.Error("Failed to complete authentication.", "error", authErr)
		result.Error = authErr // Set Error field.
		return result, errors.Wrap(authErr, "failed to complete authentication flow after multiple attempts")
	}

	authState, verifyErr := m.service.GetAuthState(ctx)
	if verifyErr != nil || authState == nil || !authState.IsAuthenticated {
		m.logger.Error("Auth completion succeeded but failed verification.",
			"error", verifyErr,
			"authenticated", authState != nil && authState.IsAuthenticated)
		if verifyErr != nil {
			result.Error = verifyErr // Set Error field.
			return result, errors.Wrap(verifyErr, "authentication verification failed")
		}
		result.Error = errors.New("authentication verification failed - not authenticated") // Set Error field.
		return result, result.Error
	}

	m.logger.Info("Authentication successful!.", "username", authState.Username)
	fmt.Printf("\n✅ Authentication successful! Logged in as: %s\n\n", authState.Username)

	result.Success = true
	result.Username = authState.Username
	if m.options.AutoSaveToken {
		m.saveTokenToStorage(m.service.GetAuthToken(), authState.UserID, authState.Username)
	}

	return result, nil
}

// handleHeadlessAuth attempts authentication without user interaction.
// Updated to return *EnsureAuthResult.
func (m *AuthManager) handleHeadlessAuth(ctx context.Context) (*EnsureAuthResult, error) {
	token, username, err := m.findExistingTokens(ctx) // Assuming findExistingTokens exists elsewhere.
	if err == nil && token != "" {
		m.logger.Info("Headless authentication successful using existing token.", "username", username)
		return &EnsureAuthResult{ // Return correct type.
			Success:  true,
			Username: username,
		}, nil
	}
	m.logger.Warn("Headless authentication failed: No valid existing tokens found.", "discoveryError", err)

	authURL, frob, startErr := m.client.StartAuthFlow(ctx)
	if startErr != nil {
		return &EnsureAuthResult{Success: false, Error: startErr},
			errors.Wrap(startErr, "failed to start auth flow in headless mode")
	}

	headlessErr := errors.New("headless mode cannot complete authentication without external help (e.g., pre-configured token)")
	return &EnsureAuthResult{ // Return correct type.
		Success:     false,
		AuthURL:     authURL,
		Frob:        frob,
		NeedsManual: true,
		Error:       headlessErr,
	}, headlessErr
}

// handleTestAuth handles authentication in test environments.
// Updated to return *EnsureAuthResult.
func (m *AuthManager) handleTestAuth(ctx context.Context) (*EnsureAuthResult, error) {
	token, username, err := m.findExistingTokens(ctx) // Assuming findExistingTokens exists elsewhere.
	if err == nil && token != "" {
		m.logger.Info("Test authentication successful using existing token.", "username", username)
		return &EnsureAuthResult{ // Return correct type.
			Success:  true,
			Username: username,
		}, nil
	}
	m.logger.Info("No valid existing tokens found during test auth, proceeding.", "discoveryError", err)

	if os.Getenv("CI") != "" {
		m.logger.Info("Running in CI environment, using mock authentication.")
		return &EnsureAuthResult{ // Return correct type.
			Success:  true,
			Username: "ci_test_user",
		}, nil
	}

	m.logger.Error("Test environment requires pre-authentication. No valid token found.")
	authURL, frob, startErr := m.client.StartAuthFlow(ctx)
	if startErr != nil {
		return &EnsureAuthResult{Success: false, Error: startErr},
			errors.Wrap(startErr, "failed to start auth flow to get instructions")
	}

	testAuthErr := errors.New("test authentication failed: no pre-existing valid token found; run 'go run ./cmd/rtm_connection_test' manually to authenticate first")
	return &EnsureAuthResult{ // Return correct type.
		Success: false,
		AuthURL: authURL,
		Frob:    frob,
		Error:   testAuthErr,
	}, testAuthErr
}
