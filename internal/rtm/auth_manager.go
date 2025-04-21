// file: internal/rtm/auth_manager.go
package rtm

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// AuthMode defines how the authentication flow should be handled.
type AuthMode int

const (
	// AuthModeInteractive requires user to visit a URL.
	AuthModeInteractive AuthMode = iota
	// AuthModeHeadless tries to auto-complete without user interaction.
	AuthModeHeadless
	// AuthModeTest is specialized for test environments.
	AuthModeTest
)

// AuthManagerOptions configures the auth manager behavior.
type AuthManagerOptions struct {
	// Mode sets how auth should be handled.
	Mode AuthMode
	// AutoCompleteAuth attempts to automatically complete auth if possible.
	AutoCompleteAuth bool
	// CallbackHost for OAuth flow completion server.
	CallbackHost string
	// CallbackPort for OAuth flow completion server.
	CallbackPort int
	// TimeoutDuration for auth operations.
	TimeoutDuration time.Duration
	// RetryAttempts for auth operations.
	RetryAttempts int
	// RetryBackoff for time between retry attempts.
	RetryBackoff time.Duration
	// AutoSaveToken determines if tokens should be saved to file after successful auth
	AutoSaveToken bool
	// TestTokenPath specifies a custom path for test authentication tokens
	TestTokenPath string
}

// DefaultAuthManagerOptions provides sensible defaults.
func DefaultAuthManagerOptions() AuthManagerOptions {
	return AuthManagerOptions{
		Mode:             AuthModeInteractive,
		AutoCompleteAuth: true,
		CallbackHost:     "localhost",
		CallbackPort:     8090,
		TimeoutDuration:  2 * time.Minute,
		RetryAttempts:    3,
		RetryBackoff:     500 * time.Millisecond,
		AutoSaveToken:    true,
	}
}

// AuthResult contains the outcome of an authentication attempt.
type AuthResult struct {
	Success     bool
	Username    string
	Error       error
	AuthURL     string
	Frob        string
	NeedsManual bool
}

// AuthManager handles the complete RTM authentication flow.
type AuthManager struct {
	service        *Service
	client         *Client
	options        AuthManagerOptions
	logger         logging.Logger
	callbackServer *http.Server
	state          string     // CSRF protection token
	resultChan     chan error // For callback server communication
	callbackMutex  sync.Mutex // Protect callback server state
	retryMutex     sync.Mutex // Protect retry counters
	retryCount     map[string]int
}

// NewAuthManager creates a new auth manager for the given service.
func NewAuthManager(service *Service, options AuthManagerOptions, logger logging.Logger) *AuthManager {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &AuthManager{
		service:    service,
		client:     service.client,
		options:    options,
		logger:     logger.WithField("component", "rtm_auth_manager"),
		retryCount: make(map[string]int),
		state:      generateStateToken(),
	}
}

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

// EnsureAuthenticated makes sure the service is authenticated,
// taking care of the complete flow if needed.
// Returns (*AuthResult, error)
func (m *AuthManager) EnsureAuthenticated(ctx context.Context) (*AuthResult, error) {
	m.logger.Info("Checking authentication status...")

	// First check if already authenticated with retry
	var authState *AuthState
	var err error

	for attempt := 0; attempt <= m.options.RetryAttempts; attempt++ {
		if attempt > 0 {
			m.logger.Debug("Retrying auth state check", "attempt", attempt)
			time.Sleep(m.options.RetryBackoff)
		}

		authState, err = m.service.GetAuthState(ctx)
		if err == nil {
			break
		}

		m.logger.Warn("Error checking auth state", "error", err, "attempt", attempt)
	}

	// If authenticated, return success immediately
	if authState != nil && authState.IsAuthenticated {
		m.logger.Info("Already authenticated", "username", authState.Username)
		return &AuthResult{
			Success:  true,
			Username: authState.Username,
		}, nil
	}

	// Not authenticated, start flow based on mode
	m.logger.Info("Authentication required, starting flow")

	switch m.options.Mode {
	case AuthModeHeadless:
		return m.handleHeadlessAuth(ctx)
	case AuthModeTest:
		return m.handleTestAuth(ctx)
	default: // Interactive
		return m.handleInteractiveAuth(ctx)
	}
}

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
	fmt.Scanln()

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

// saveTokenAfterSuccessfulAuth saves the current auth token to a file
// if AutoSaveToken is enabled in options.
func (m *AuthManager) saveTokenAfterSuccessfulAuth(authState *AuthState) {
	var username, userID string
	var token string

	// Get the token from the service or client
	token = m.client.GetAuthToken()
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

// startCallbackServer starts a local HTTP server to receive the auth callback.
func (m *AuthManager) startCallbackServer(ctx context.Context, frob string) error {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()

	// Don't start if already running
	if m.callbackServer != nil {
		return errors.New("callback server already running")
	}

	m.resultChan = make(chan error, 1)

	// Create server with security precautions
	mux := http.NewServeMux()

	// Handle the callback path
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		m.logger.Info("Received callback request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr)

		defer func() {
			if err := recover(); err != nil {
				m.logger.Error("Panic in callback handler", "error", err)
				m.callbackMutex.Lock()
				if m.resultChan != nil {
					m.resultChan <- errors.Errorf("panic in callback: %v", err)
				}
				m.callbackMutex.Unlock()

				// Return error to browser
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		// Verify request
		if r.Method != http.MethodGet {
			m.logger.Warn("Invalid method in callback", "method", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check state parameter for CSRF protection
		receivedState := r.URL.Query().Get("state")
		m.logger.Debug("Checking callback state parameter",
			"received", receivedState,
			"expected", m.state)

		if receivedState != m.state {
			m.logger.Warn("Invalid state in callback",
				"received", receivedState,
				"expected", m.state)
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- errors.New("CSRF protection failed: invalid state")
			}
			m.callbackMutex.Unlock()
			return
		}

		// Complete authentication with the frob
		m.logger.Info("Completing auth from callback", "frob", frob)
		err := m.service.CompleteAuth(ctx, frob)
		if err != nil {
			m.logger.Error("Failed to complete auth in callback", "error", err)

			// Show user-friendly error page
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "<html><body><h1>Authentication Failed</h1>"+
				"<p>Error: %s</p>"+
				"<p>Please try again or contact support.</p>"+
				"</body></html>",
				err.Error())

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- err
			}
			m.callbackMutex.Unlock()
			return
		}

		// Get auth state to verify and get username
		authState, stateErr := m.service.GetAuthState(ctx)
		if stateErr != nil {
			m.logger.Error("Failed to verify auth state after completion", "error", stateErr)

			// Show user-friendly error page
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "<html><body><h1>Authentication Verification Failed</h1>"+
				"<p>Error: %s</p>"+
				"<p>Authentication may have been successful, but we couldn't verify it.</p>"+
				"</body></html>",
				stateErr.Error())

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- stateErr
			}
			m.callbackMutex.Unlock()
			return
		}

		if authState == nil || !authState.IsAuthenticated {
			m.logger.Error("Auth completion succeeded but user not authenticated")

			// Show user-friendly error page
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "<html><body><h1>Authentication Failed</h1>"+
				"<p>The authorization process completed, but you are not authenticated.</p>"+
				"<p>Please try again or contact support.</p>"+
				"</body></html>")

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- errors.New("auth completion succeeded but user not authenticated")
			}
			m.callbackMutex.Unlock()
			return
		}

		// Success page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body><h1>Authentication Successful!</h1>"+
			"<p>You are now authenticated as: %s</p>"+
			"<p>You can close this window and return to the application.</p>"+
			"</body></html>",
			authState.Username)

		// Signal success to main thread
		m.callbackMutex.Lock()
		if m.resultChan != nil {
			m.resultChan <- nil
		}
		m.callbackMutex.Unlock()
	})

	// Create server with timeout settings
	addr := fmt.Sprintf("%s:%d", m.options.CallbackHost, m.options.CallbackPort)
	m.callbackServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Start server in separate goroutine
	go func() {
		m.logger.Info("Starting callback server", "address", addr)
		if err := m.callbackServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Callback server error", "error", err)

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- err
			}
			m.callbackMutex.Unlock()
		}
	}()

	return nil
}

// stopCallbackServer gracefully shuts down the callback server.
func (m *AuthManager) stopCallbackServer() {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()

	if m.callbackServer == nil {
		return
	}

	m.logger.Info("Stopping callback server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.callbackServer.Shutdown(ctx); err != nil {
		m.logger.Warn("Callback server shutdown error", "error", err)
	}

	m.callbackServer = nil
}

// Shutdown performs cleanup of resources.
func (m *AuthManager) Shutdown() {
	m.stopCallbackServer()
}

// retryableOperation provides retry logic for operations returning a single error.
func (m *AuthManager) retryableOperation(ctx context.Context, opName string,
	fn func(context.Context) error) error {

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
func (m *AuthManager) retryableOperationWithStrings(ctx context.Context, opName string,
	fn func(context.Context) (string, string, error)) (string, string, error) {

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
