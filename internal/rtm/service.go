// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/service.go

import (
	"context"
	"fmt" // Keep fmt import
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// Service provides Remember The Milk functionality to the MCP server.
type Service struct {
	client       *Client // Assumes Client is defined in client.go.
	config       *config.Config
	logger       logging.Logger
	authState    *AuthState // Assumes AuthState is defined in types.go.
	authMutex    sync.RWMutex
	tokenStorage TokenStorageInterface // Changed from *TokenStorage.
	initialized  bool
}

// NewService creates a new RTM service with the given configuration.
func NewService(cfg *config.Config, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	serviceLogger := logger.WithField("component", "rtm_service")

	rtmConfig := Config{ // Assumes Config is defined in types.go.
		APIKey:       cfg.RTM.APIKey,
		SharedSecret: cfg.RTM.SharedSecret,
		// APIEndpoint and HTTPClient will use defaults in NewClient.
	}
	client := NewClient(rtmConfig, logger) // Assumes NewClient is in client.go.

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
		serviceLogger.Warn("Failed to initialize token storage. Token persistence disabled.", "error", err) // Added warning
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
	s.logger.Info("Initializing RTM Service...") // Log start

	// 1. Check Prerequisites
	if err := s.checkPrerequisites(); err != nil {
		s.logger.Error("-> Initialization Failed: Prerequisites not met.", "error", err) // Log failure reason
		return err                                                                       // Return error early if config is missing.
	}

	// 2. Load Token from Storage (if available)
	tokenFound := s.loadAndSetTokenFromStorage() // Logs status internally, returns true if token found.

	// 3. Check and Handle Initial Auth State
	verificationErr := s.checkAndHandleInitialAuthState(ctx) // Logs status internally.
	if verificationErr != nil {
		s.logger.Warn("Initial RTM authentication check failed.", "error", verificationErr)
		// Continue initialization but ensure state is not authenticated.
		s.updateAuthState(&AuthState{IsAuthenticated: false})
	} else if tokenFound && !s.IsAuthenticated() {
		// Token was loaded, but verification showed it was invalid.
		s.logger.Warn("Loaded token was invalid according to RTM API.")
	}

	// 4. Store Verified Token If Necessary (only if auth check succeeded and state is now authenticated)
	s.storeVerifiedTokenIfNeeded() // Logs status internally.

	// 5. Finalize Initialization
	s.initialized = true
	statusMsg := "Not Authenticated"
	if s.IsAuthenticated() {
		statusMsg = fmt.Sprintf("Authenticated as %q", s.GetUsername())
	}
	s.logger.Info(fmt.Sprintf("Initialization complete. Status: %s.", statusMsg)) // Simplified final status log
	return nil
}

// checkPrerequisites verifies required configuration.
func (s *Service) checkPrerequisites() error {
	s.logger.Info("Checking configuration (API Key/Secret)...") // Simpler message
	if s.config.RTM.APIKey == "" || s.config.RTM.SharedSecret == "" {
		s.logger.Error("-> Configuration Check Failed: RTM API Key or Shared Secret is missing.") // Clearer error
		return errors.New("RTM API key and shared secret are required")
	}
	s.logger.Info("-> Configuration OK.") // Clearer success
	return nil
}

// loadAndSetTokenFromStorage attempts to load a token from storage and set it on the client.
// Returns true if a token was found and loaded, false otherwise.
func (s *Service) loadAndSetTokenFromStorage() bool {
	s.logger.Info("Loading saved authentication token...") // Simpler message
	if s.tokenStorage == nil {
		s.logger.Info("-> Skipped (Token storage not configured).")
		return false
	}

	token, err := s.tokenStorage.LoadToken() // tokenStorage logs source (secure/file) and attempt outcome
	if err != nil {
		s.logger.Warn("-> Failed to load token.", "error", err) // Log specific error
		return false
	} else if token != "" {
		// s.logger.Info("-> Found and loaded saved token.") // Redundant, LoadToken logs success
		s.client.SetAuthToken(token) // Set token on the client.
		return true
	}
	s.logger.Info("-> No saved token found.") // Clearer outcome
	return false
}

// checkAndHandleInitialAuthState verifies the current token with the RTM API.
// If the token is invalid, it clears it from the client and storage.
// Returns an error if the API call itself fails, or nil otherwise (even if not authenticated).
func (s *Service) checkAndHandleInitialAuthState(ctx context.Context) error {
	s.logger.Info("Verifying saved token with RTM...") // Simpler message
	currentToken := s.client.GetAuthToken()
	if currentToken == "" {
		s.logger.Info("-> Skipped (No token to verify).") // Clearer reason
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return nil // No error, just not authenticated.
	}

	authState, err := s.client.GetAuthState(ctx) // Assumes client.GetAuthState handles its own errors.

	if err != nil {
		s.logger.Warn("-> Verification API call failed.")                      // Clearer status
		s.logger.Warn("RTM token verification API call failed.", "error", err) // Keep specific error log

		// Clear potentially invalid token
		if s.client.GetAuthToken() != "" {
			s.logger.Info("-> Clearing potentially invalid token due to API error.") // Clearer reason
			s.client.SetAuthToken("")
			if s.tokenStorage != nil {
				if delErr := s.tokenStorage.DeleteToken(); delErr != nil {
					s.logger.Warn("Failed to delete invalid token from storage.", "error", delErr)
				}
			}
		}
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return err // Return the original error
	}

	// API call succeeded, check RTM's verdict.
	s.updateAuthState(authState) // Update cache first.
	if authState.IsAuthenticated {
		s.logger.Info(fmt.Sprintf("-> Token verified successfully (User: %q).", authState.Username)) // Include username
	} else {
		s.logger.Warn("-> Token reported as invalid by RTM.") // Clearer failure
		if s.client.GetAuthToken() != "" {
			s.logger.Info("-> Clearing invalid token.") // Clearer action
			s.client.SetAuthToken("")
			if s.tokenStorage != nil {
				if delErr := s.tokenStorage.DeleteToken(); delErr != nil {
					s.logger.Warn("Failed to delete unauthenticated token from storage.", "error", delErr)
				}
			}
		}
	}
	return nil // No error from the check process itself.
}

// storeVerifiedTokenIfNeeded saves the currently set (and verified) auth token if it's not already stored correctly.
func (s *Service) storeVerifiedTokenIfNeeded() {
	if s.tokenStorage == nil {
		return
	}
	currentToken := s.client.GetAuthToken()
	if currentToken == "" || !s.IsAuthenticated() { // Only save if authenticated and have a token
		return
	}

	s.logger.Info("Checking if token needs saving...") // Simpler message
	storedToken, loadErr := s.tokenStorage.LoadToken() // LoadToken logs attempt/source/result

	if loadErr != nil || storedToken != currentToken {
		s.logger.Info("-> Saving verified token to storage.") // Clearer action
		userID, username := s.getUserInfoFromState()
		if saveErr := s.tokenStorage.SaveToken(currentToken, userID, username); saveErr != nil {
			s.logger.Warn("-> Failed to save token.", "error", saveErr) // Log specific error
		}
	} else {
		s.logger.Info("-> Token already saved correctly.") // Clearer status
	}
}

// --- Auth State Management --- (No logging changes needed here)

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
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get auth state from RTM client")
	}
	s.updateAuthState(authState)
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

// --- Auth Lifecycle --- (Simplified logging)

// StartAuth begins the authentication flow. Returns the auth URL.
func (s *Service) StartAuth(ctx context.Context) (string, error) {
	s.logger.Info("Starting RTM authentication flow (getting auth URL)...") // Clarified purpose
	authURL, _, err := s.client.StartAuthFlow(ctx)                          // Frob unused here
	if err != nil {
		s.logger.Error("-> Failed to start auth flow.", "error", err) // Log specific error
		return "", err
	}
	s.logger.Info("-> Auth URL generated.")
	return authURL, nil
}

// CompleteAuth completes the authentication flow using the frob.
func (s *Service) CompleteAuth(ctx context.Context, frob string) error {
	s.logger.Info("Completing RTM authentication flow (exchanging code for token)...") // Clarified purpose
	token, err := s.client.CompleteAuthFlow(ctx, frob)
	if err != nil {
		s.logger.Error("-> Failed to complete auth flow.", "error", err) // Log specific error
		return err
	}

	// Verify state and save token
	authState, stateErr := s.client.GetAuthState(ctx)
	if stateErr != nil {
		s.logger.Error("-> Failed to verify auth state after getting token.", "error", stateErr)
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return errors.Wrap(stateErr, "failed to confirm auth state after completing auth flow")
	}
	s.updateAuthState(authState)

	if s.IsAuthenticated() {
		s.logger.Info(fmt.Sprintf("-> Authentication successful (User: %q).", s.GetUsername()))
		if s.tokenStorage != nil && token != "" {
			s.logger.Info("-> Saving new token...")
			userID, username := s.getUserInfoFromState()
			if saveErr := s.tokenStorage.SaveToken(token, userID, username); saveErr != nil {
				s.logger.Warn("-> Failed to save new token.", "error", saveErr)
			}
		}
	} else {
		s.logger.Warn("-> Authentication flow seemed complete, but state verification failed.")
	}

	return nil
}

// SetAuthToken explicitly sets the auth token and updates storage.
func (s *Service) SetAuthToken(token string) {
	// No logging changes needed here - relies on other methods that were updated
	s.client.SetAuthToken(token)
	if token == "" {
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		if s.tokenStorage != nil {
			_ = s.tokenStorage.DeleteToken()
		}
		return
	}
	if s.tokenStorage != nil {
		ctx := context.Background()
		authState, err := s.client.GetAuthState(ctx)
		if err != nil {
			s.logger.Warn("Failed to verify manually set token.", "error", err)
			s.updateAuthState(&AuthState{IsAuthenticated: false})
			return
		}
		s.updateAuthState(authState)
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
	s.logger.Info("Clearing RTM authentication...") // Simpler message
	s.client.SetAuthToken("")
	s.updateAuthState(&AuthState{IsAuthenticated: false})
	if s.tokenStorage != nil {
		if err := s.tokenStorage.DeleteToken(); err != nil {
			s.logger.Error("-> Failed to clear token from storage.", "error", err) // Log specific error
			return errors.Wrap(err, "failed to delete token from storage")
		}
	}
	s.logger.Info("-> Authentication cleared.")
	return nil
}

// Shutdown performs cleanup for the service.
func (s *Service) Shutdown() error {
	s.logger.Info("Shutting down RTM service.")
	return nil
}

// GetName returns the service name.
func (s *Service) GetName() string {
	return "rtm"
}
