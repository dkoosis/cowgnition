// file: internal/rtm/auth_manager.go
package rtm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// AuthMode defines how the authentication flow should be handled
type AuthMode int

const (
	// AuthModeInteractive requires user to visit a URL
	AuthModeInteractive AuthMode = iota
	// AuthModeHeadless tries to auto-complete without user interaction
	AuthModeHeadless
	// AuthModeTest is specialized for test environments
	AuthModeTest
)

// AuthManagerOptions configures the auth manager behavior
type AuthManagerOptions struct {
	// Mode sets how auth should be handled
	Mode AuthMode
	// AutoCompleteAuth attempts to automatically complete auth if possible
	AutoCompleteAuth bool
	// CallbackURL for OAuth flow completion
	CallbackURL string
	// TimeoutDuration for auth operations
	TimeoutDuration time.Duration
	// RetryAttempts for auth operations
	RetryAttempts int
}

// DefaultAuthManagerOptions provides sensible defaults
func DefaultAuthManagerOptions() AuthManagerOptions {
	return AuthManagerOptions{
		Mode:             AuthModeInteractive,
		AutoCompleteAuth: true,
		TimeoutDuration:  2 * time.Minute,
		RetryAttempts:    3,
	}
}

// AuthManager handles the complete RTM authentication flow
type AuthManager struct {
	service        *Service
	client         *Client
	options        AuthManagerOptions
	logger         logging.Logger
	callbackServer *http.Server
}

// NewAuthManager creates a new auth manager for the given service
func NewAuthManager(service *Service, options AuthManagerOptions, logger logging.Logger) *AuthManager {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &AuthManager{
		service: service,
		client:  service.client,
		options: options,
		logger:  logger.WithField("component", "rtm_auth_manager"),
	}
}

// EnsureAuthenticated makes sure the service is authenticated,
// taking care of the complete flow if needed
func (m *AuthManager) EnsureAuthenticated(ctx context.Context) error {
	// First check if already authenticated
	m.logger.Info("Checking authentication status...")
	authState, err := m.service.GetAuthState(ctx)
	if err != nil {
		m.logger.Warn("Error checking auth state", "error", err)
		// Continue and try to authenticate
	}

	if authState != nil && authState.IsAuthenticated {
		m.logger.Info("Already authenticated", "username", authState.Username)
		return nil
	}

	// Not authenticated, start flow
	return m.completeAuthFlow(ctx)
}

// completeAuthFlow handles the full auth flow based on options
func (m *AuthManager) completeAuthFlow(ctx context.Context) error {
	// Start the auth flow to get a frob and URL
	authURL, frob, err := m.client.StartAuthFlow(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to start auth flow")
	}

	m.logger.Info("Authentication required")

	switch m.options.Mode {
	case AuthModeHeadless:
		return m.handleHeadlessAuth(ctx, frob, authURL)
	case AuthModeTest:
		return m.handleTestAuth(ctx, frob, authURL)
	default: // Interactive
		return m.handleInteractiveAuth(ctx, frob, authURL)
	}
}

// handleInteractiveAuth guides the user through the auth process
func (m *AuthManager) handleInteractiveAuth(ctx context.Context, frob, authURL string) error {
	m.logger.Info("Please visit this URL to authorize the application:", "url", authURL)
	fmt.Printf("\n⚠️  Authentication Required\n\n")
	fmt.Printf("1. Open this URL in your browser:\n   %s\n\n", authURL)
	fmt.Printf("2. Click 'Allow Access' to authorize this application\n\n")

	if m.options.AutoCompleteAuth {
		// Start callback server if URL provided
		if m.options.CallbackURL != "" {
			callbackErr := make(chan error, 1)
			frobCh := make(chan string, 1)
			frobCh <- frob // Pre-load the frob

			go m.startCallbackServer(frobCh, callbackErr)

			select {
			case err := <-callbackErr:
				return err
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(m.options.TimeoutDuration):
				m.logger.Warn("Authentication timed out")
				return errors.New("authentication timed out waiting for callback")
			}
		}

		// Wait for user acknowledgment
		fmt.Printf("3. Press Enter after you've completed authorization...\n")
		fmt.Scanln()

		// Complete flow with the frob we already have
		m.logger.Info("Completing authentication flow...", "frob", frob)
		err := m.service.CompleteAuth(ctx, frob)
		if err != nil {
			m.logger.Error("Failed to complete authentication", "error", err)
			return errors.Wrap(err, "failed to complete authentication flow")
		}

		m.logger.Info("Authentication successful!", "username", m.service.GetUsername())
		fmt.Printf("\n✅ Authentication successful! Logged in as: %s\n\n", m.service.GetUsername())
		return nil
	}

	// Just provide instructions if not auto-completing
	fmt.Printf("3. After authorizing, use the following to complete authentication:\n")
	fmt.Printf("   Complete authentication with frob: %s\n", frob)

	return errors.New("authentication requires manual completion")
}

// Additional methods needed but omitted for brevity:
// - handleHeadlessAuth
// - handleTestAuth
// - startCallbackServer
// - stopCallbackServer
