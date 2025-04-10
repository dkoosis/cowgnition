package rtm

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// Service provides Remember The Milk functionality to the MCP server.
type Service struct {
	client       *Client // Assumes Client is defined in client.go
	config       *config.Config
	logger       logging.Logger
	authState    *AuthState // Assumes AuthState is defined in types.go
	authMutex    sync.RWMutex
	tokenStorage *TokenStorage // Assumes TokenStorage is defined in token_storage.go
	initialized  bool
}

// NewService creates a new RTM service with the given configuration.
func NewService(cfg *config.Config, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	serviceLogger := logger.WithField("component", "rtm_service")

	rtmConfig := Config{ // Assumes Config is defined in types.go
		APIKey:       cfg.RTM.APIKey,
		SharedSecret: cfg.RTM.SharedSecret,
		// APIEndpoint and HTTPClient will use defaults in NewClient
	}
	client := NewClient(rtmConfig, logger) // Assumes NewClient is in client.go

	tokenPath := cfg.Auth.TokenPath
	if tokenPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json")
		} else {
			tokenPath = "rtm_token.json" //nolint:gosec
			serviceLogger.Warn("Could not determine home directory for token storage.", "error", err, "fallbackPath", tokenPath)
		}
	}

	tokenStorage, err := NewTokenStorage(tokenPath, logger)
	if err != nil {
		serviceLogger.Warn("Failed to initialize token storage.", "error", err)
		tokenStorage = nil
	}

	return &Service{
		client:       client,
		config:       cfg,
		logger:       serviceLogger,
		authState:    &AuthState{},
		tokenStorage: tokenStorage,
	}
}

// Initialize initializes the RTM service by checking config, loading tokens, and verifying auth state.
func (s *Service) Initialize(ctx context.Context) error {
	s.logger.Info("Initializing RTM service.")

	// 1. Check Prerequisites
	if err := s.checkPrerequisites(); err != nil {
		return err // Return error early if config is missing
	}

	// 2. Load Token from Storage (if available)
	s.loadAndSetTokenFromStorage()

	// 3. Check and Handle Initial Auth State
	if err := s.checkAndHandleInitialAuthState(ctx); err != nil {
		// Log the error from auth check, but initialization might still proceed
		s.logger.Warn("Initial RTM authentication check failed or indicated logged out.", "error", err)
		// Ensure state reflects not authenticated
		s.updateAuthState(&AuthState{IsAuthenticated: false})
	}

	// 4. Store Verified Token If Necessary (only if auth check succeeded)
	if s.IsAuthenticated() {
		s.storeVerifiedTokenIfNeeded()
	}

	// 5. Finalize Initialization
	s.initialized = true
	s.logger.Info("RTM service initialization complete.",
		"authenticated", s.IsAuthenticated(),
		"username", s.GetUsername())
	return nil
}

// checkPrerequisites verifies required configuration.
func (s *Service) checkPrerequisites() error {
	if s.config.RTM.APIKey == "" || s.config.RTM.SharedSecret == "" {
		return errors.New("RTM API key and shared secret are required")
	}
	// Add any other essential checks here
	return nil
}

// loadAndSetTokenFromStorage attempts to load a token from storage and set it on the client.
func (s *Service) loadAndSetTokenFromStorage() {
	if s.tokenStorage == nil {
		s.logger.Debug("Token storage not configured, skipping token load.")
		return
	}

	token, err := s.tokenStorage.LoadToken()
	if err != nil {
		s.logger.Warn("Failed to load auth token from storage.", "error", err)
		// Don't set token if loading failed
	} else if token != "" {
		s.logger.Info("Loaded auth token from storage.")
		s.client.SetAuthToken(token) // Set token on the client
	} else {
		s.logger.Debug("No token found in storage.")
	}
}

// checkAndHandleInitialAuthState verifies the current token with the RTM API.
// If the token is invalid, it clears it from the client and storage.
// Returns an error if the API call itself fails, or nil otherwise (even if not authenticated).
func (s *Service) checkAndHandleInitialAuthState(ctx context.Context) error {
	authState, err := s.client.GetAuthState(ctx) // Assumes client.GetAuthState handles its own errors
	if err != nil {
		// Auth check failed - potentially invalid token or API issue
		s.logger.Warn("GetAuthState failed during initialization.", "error", err)

		// If we had a token loaded, it's likely invalid, so clear it.
		if s.client.GetAuthToken() != "" {
			s.logger.Info("Clearing potentially invalid auth token due to GetAuthState failure.")
			s.client.SetAuthToken("")
			if s.tokenStorage != nil {
				if delErr := s.tokenStorage.DeleteToken(); delErr != nil {
					// Log deletion error, but don't overwrite the original GetAuthState error
					s.logger.Warn("Failed to delete invalid token from storage.", "error", delErr)
				}
			}
		}
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		// Return the original error from GetAuthState
		return err
	}

	// Auth check succeeded, update our cached state
	s.updateAuthState(authState)
	if !authState.IsAuthenticated && s.client.GetAuthToken() != "" {
		// RTM says we're not authenticated, but we had a token. Clear the invalid token.
		s.logger.Info("Clearing auth token as RTM reports not authenticated.")
		s.client.SetAuthToken("")
		if s.tokenStorage != nil {
			if delErr := s.tokenStorage.DeleteToken(); delErr != nil {
				s.logger.Warn("Failed to delete unauthenticated token from storage.", "error", delErr)
			}
		}
	}

	return nil // No error from the check process itself
}

// storeVerifiedTokenIfNeeded saves the currently set (and verified) auth token if it's not already stored correctly.
func (s *Service) storeVerifiedTokenIfNeeded() {
	// No need to check IsAuthenticated() here, as this is called only when authenticated.
	if s.tokenStorage == nil {
		return // Nothing to store if storage isn't configured
	}

	currentToken := s.client.GetAuthToken()
	if currentToken == "" {
		return // Nothing to store
	}

	storedToken, loadErr := s.tokenStorage.LoadToken()
	// Store only if loading failed OR the stored token doesn't match the current valid token
	if loadErr != nil || storedToken != currentToken {
		s.logger.Info("Storing verified auth token.")
		userID, username := s.getUserInfoFromState()
		if saveErr := s.tokenStorage.SaveToken(currentToken, userID, username); saveErr != nil {
			s.logger.Warn("Failed to save verified auth token.", "error", saveErr)
		}
	}
}

// --- Auth State Management ---

// IsAuthenticated checks if the service is currently authenticated.
func (s *Service) IsAuthenticated() bool {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authState != nil && s.authState.IsAuthenticated
}

// GetUsername returns the authenticated user's username, or empty string.
func (s *Service) GetUsername() string {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState == nil {
		return ""
	}
	return s.authState.Username
}

// GetAuthState retrieves the current cached or refreshed auth state.
func (s *Service) GetAuthState(ctx context.Context) (*AuthState, error) {
	// Refresh from API
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get auth state from RTM client")
	}
	s.updateAuthState(authState) // Update cache
	return authState, nil
}

// updateAuthState safely updates the cached auth state.
func (s *Service) updateAuthState(newState *AuthState) {
	s.authMutex.Lock()
	defer s.authMutex.Unlock()
	s.authState = newState
}

// getUserInfoFromState safely gets user ID and username from cached state.
func (s *Service) getUserInfoFromState() (userID, username string) {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState != nil {
		return s.authState.UserID, s.authState.Username
	}
	return "", ""
}

// --- Auth Lifecycle ---

// StartAuth begins the authentication flow. Returns the auth URL.
func (s *Service) StartAuth(ctx context.Context) (string, error) {
	s.logger.Info("Starting RTM auth flow.")
	// Correctly handle the 3 return values from the refactored client method
	authURL, frob, err := s.client.StartAuthFlow(ctx)
	if err != nil {
		// Error is already wrapped by StartAuthFlow
		return "", err
	}
	// Service layer might not need the frob directly, but log for debugging?
	s.logger.Debug("Obtained frob during StartAuth", "frobLength", len(frob))
	// Return only the URL as per the original signature requirement (or adjust if needed)
	return authURL, nil
}

// CompleteAuth completes the authentication flow using the frob.
func (s *Service) CompleteAuth(ctx context.Context, frob string) error {
	s.logger.Info("Completing RTM auth flow.") // Don't log frob

	// Correctly handle the 2 return values from the refactored client method
	token, err := s.client.CompleteAuthFlow(ctx, frob)
	if err != nil {
		return errors.Wrap(err, "failed to complete auth flow with RTM client") // Already wrapped
	}
	// Token is set internally in the client by CompleteAuthFlow

	// Immediately refresh and store the auth state
	authState, stateErr := s.client.GetAuthState(ctx)
	if stateErr != nil {
		s.logger.Error("Failed to fetch auth state immediately after auth flow.", "error", stateErr)
		s.updateAuthState(&AuthState{IsAuthenticated: false}) // Assume failure
		// Decide whether to return the stateErr or the original nil error from CompleteAuthFlow
		return errors.Wrap(stateErr, "failed to confirm auth state after completing auth flow")
	}
	s.updateAuthState(authState)

	// Save the newly obtained token if storage is available and auth is confirmed
	if s.tokenStorage != nil && s.IsAuthenticated() && token != "" {
		s.logger.Info("Saving auth token to storage after completing auth flow.")
		userID, username := s.getUserInfoFromState()
		if saveErr := s.tokenStorage.SaveToken(token, userID, username); saveErr != nil {
			s.logger.Warn("Failed to save auth token to storage.", "error", saveErr)
			// Don't fail the whole operation for a save error
		}
	} else if !s.IsAuthenticated() {
		s.logger.Warn("Authentication flow seemed complete, but state verification failed.")
	}

	return nil // Return nil if CompleteAuthFlow succeeded, even if state check/save had issues (logged)
}

// SetAuthToken explicitly sets the auth token and updates storage.
func (s *Service) SetAuthToken(token string) {
	s.client.SetAuthToken(token)

	// Clear auth state cache if token is cleared
	if token == "" {
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		if s.tokenStorage != nil {
			_ = s.tokenStorage.DeleteToken() // Attempt to delete, ignore error
		}
		return
	}

	// If setting a non-empty token, attempt verification and storage update
	if s.tokenStorage != nil {
		ctx := context.Background() // Use background context for verification
		authState, err := s.client.GetAuthState(ctx)
		if err != nil {
			s.logger.Warn("Failed to verify manually set token.", "error", err)
			s.updateAuthState(&AuthState{IsAuthenticated: false}) // Assume invalid
			// Optionally clear the invalid token from client? s.client.SetAuthToken("")
			return
		}

		s.updateAuthState(authState) // Update cache

		if s.IsAuthenticated() {
			s.logger.Info("Saving manually set (and verified) auth token to storage.")
			userID, username := s.getUserInfoFromState()
			if saveErr := s.tokenStorage.SaveToken(token, userID, username); saveErr != nil {
				s.logger.Warn("Failed to save manually set token.", "error", saveErr)
			}
		} else {
			s.logger.Warn("Manually set token appears invalid after check, not saving.")
		}
	}
}

// GetAuthToken returns the current auth token held by the client.
func (s *Service) GetAuthToken() string {
	return s.client.GetAuthToken()
}

// ClearAuth clears the current authentication state and token storage.
func (s *Service) ClearAuth() error {
	s.logger.Info("Clearing RTM authentication.")
	s.client.SetAuthToken("")
	s.updateAuthState(&AuthState{IsAuthenticated: false})
	if s.tokenStorage != nil {
		if err := s.tokenStorage.DeleteToken(); err != nil {
			return errors.Wrap(err, "failed to delete token from storage")
		}
	}
	return nil
}

// Shutdown performs cleanup for the service.
func (s *Service) Shutdown() error {
	s.logger.Info("Shutting down RTM service.")
	// No specific shutdown actions needed for client/state currently.
	return nil
}

// GetName returns the service name.
func (s *Service) GetName() string {
	return "rtm"
}
