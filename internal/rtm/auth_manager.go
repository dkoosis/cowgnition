// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/auth_manager.go

import (
	"context"
	"net/http"
	"sync"
	"time"

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
	// AutoSaveToken determines if tokens should be saved to file after successful auth.
	AutoSaveToken bool
	// TestTokenPath specifies a custom path for test authentication tokens.
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

// AuthManager handles the complete RTM authentication flow.
type AuthManager struct {
	service        *Service
	client         *Client
	options        AuthManagerOptions
	logger         logging.Logger
	callbackServer *http.Server
	state          string     // CSRF protection token.
	resultChan     chan error // For callback server communication.
	callbackMutex  sync.Mutex // Protect callback server state.
	retryMutex     sync.Mutex // Protect retry counters.
	retryCount     map[string]int
}

// NewAuthManager creates a new auth manager for the given service.
func NewAuthManager(service *Service, options AuthManagerOptions, logger logging.Logger) *AuthManager {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	// Assuming generateStateToken() is defined in auth_manager_helpers.go.
	return &AuthManager{
		service:    service,
		client:     service.client,
		options:    options,
		logger:     logger.WithField("component", "rtm_auth_manager"),
		retryCount: make(map[string]int),
		state:      generateStateToken(),
	}
}

// EnsureAuthenticated makes sure the service is authenticated,
// taking care of the complete flow if needed.
// Returns (*EnsureAuthResult, error).
func (m *AuthManager) EnsureAuthenticated(ctx context.Context) (*EnsureAuthResult, error) {
	m.logger.Info("Checking authentication status...")

	var authState *AuthState
	var err error

	for attempt := 0; attempt <= m.options.RetryAttempts; attempt++ {
		if attempt > 0 {
			m.logger.Debug("Retrying auth state check.", "attempt", attempt)
			time.Sleep(m.options.RetryBackoff)
		}

		authState, err = m.service.GetAuthState(ctx)
		if err == nil {
			break
		}

		m.logger.Warn("Error checking auth state.", "error", err, "attempt", attempt)
	}

	if authState != nil && authState.IsAuthenticated {
		m.logger.Info("Already authenticated.", "username", authState.Username)
		return &EnsureAuthResult{ // Return the simpler result type.
			Success:  true,
			Username: authState.Username,
		}, nil
	}

	m.logger.Info("Authentication required, starting flow.")

	// Delegate to mode-specific handlers which must now return *EnsureAuthResult.
	switch m.options.Mode {
	case AuthModeHeadless:
		// Assuming handleHeadlessAuth exists and returns *EnsureAuthResult, error.
		return m.handleHeadlessAuth(ctx)
	case AuthModeTest:
		// Assuming handleTestAuth exists and returns *EnsureAuthResult, error.
		return m.handleTestAuth(ctx)
	default: // Interactive.
		// Assuming handleInteractiveAuth exists and returns *EnsureAuthResult, error.
		return m.handleInteractiveAuth(ctx)
	}
}

// Shutdown performs cleanup of resources (calls helper).
func (m *AuthManager) Shutdown() {
	// Assuming stopCallbackServer exists elsewhere in rtm.
	m.stopCallbackServer()
}

// Note: Ensure handleHeadlessAuth, handleTestAuth, handleInteractiveAuth (in modes.go)
// and stopCallbackServer (in callback.go) are updated to align with EnsureAuthResult
// and other changes if necessary.
