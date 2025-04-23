// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// It provides a unified interface for authentication, data retrieval (tasks, lists, tags),
// and task manipulation, handling token storage and API client interactions internally.
// file: internal/rtm/service.go.
package rtm

import (
	"context"
	"fmt" // Keep fmt import.
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// Service provides Remember The Milk functionality, acting as an intermediary between
// higher-level application logic (like MCP handlers) and the low-level RTM API client.
// It manages authentication state, token persistence, and exposes RTM operations.
type Service struct {
	client       *Client // The underlying client making direct API calls.
	config       *config.Config
	logger       logging.Logger
	authState    *AuthState // Cached authentication state.
	authMutex    sync.RWMutex
	tokenStorage TokenStorageInterface // Handles saving/loading the auth token (keyring or file).
	initialized  bool                  // Tracks if Initialize has been called.
}

// NewService creates a new RTM service instance.
// It requires application configuration (for API keys and token path) and a logger.
// It initializes the RTM client and attempts to configure token storage (secure or file-based).
func NewService(cfg *config.Config, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	serviceLogger := logger.WithField("component", "rtm_service")

	// Create the low-level client configuration from the main app config.
	rtmConfig := Config{
		APIKey:       cfg.RTM.APIKey,
		SharedSecret: cfg.RTM.SharedSecret,
		// APIEndpoint and HTTPClient will use defaults in NewClient if not set here.
	}
	client := NewClient(rtmConfig, logger) // Create the underlying API client.

	// Determine token storage path, defaulting to ~/.config/cowgnition/rtm_token.json.
	tokenPath := cfg.Auth.TokenPath
	if tokenPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json")
		} else {
			tokenPath = "rtm_token.json" //nolint:gosec // G101: Fallback path, not a secret itself.
			serviceLogger.Warn("Could not determine home directory for token storage.", "error", err, "fallbackPath", tokenPath)
		}
	}

	// Initialize token storage, preferring secure OS storage.
	tokenStorage, err := NewTokenStorage(tokenPath, logger)
	if err != nil {
		// Log failure but continue without token persistence.
		serviceLogger.Warn("Failed to initialize token storage. Token persistence disabled.", "error", err)
		tokenStorage = nil // Ensure it's nil if initialization failed.
	}

	return &Service{
		client:       client,
		config:       cfg,
		logger:       serviceLogger,
		authState:    &AuthState{}, // Initialize with non-authenticated state.
		tokenStorage: tokenStorage,
		initialized:  false, // Mark as not initialized until Initialize() is called.
	}
}

// Initialize prepares the RTM service for use.
// It verifies prerequisites (API keys), loads any existing auth token from storage,
// checks the token's validity with the RTM API, and caches the authentication state.
// This should be called once before invoking other service methods.
func (s *Service) Initialize(ctx context.Context) error {
	s.logger.Info("Initializing RTM Service...")

	// 1. Check Prerequisites (API Key/Secret).
	if err := s.checkPrerequisites(); err != nil {
		s.logger.Error("-> Initialization Failed: Prerequisites not met.", "error", err)
		return err // Return error early if config is missing.
	}

	// 2. Load Token from Storage (if available).
	tokenFound := s.loadAndSetTokenFromStorage() // Logs status internally.

	// 3. Check and Handle Initial Auth State.
	// This verifies the loaded token (if any) with the RTM API.
	verificationErr := s.checkAndHandleInitialAuthState(ctx) // Logs status internally.
	if verificationErr != nil {
		s.logger.Warn("Initial RTM authentication check failed.", "error", verificationErr)
		// Continue initialization but ensure state is not authenticated.
		s.updateAuthState(&AuthState{IsAuthenticated: false})
	} else if tokenFound && !s.IsAuthenticated() {
		// Token was loaded, but verification showed it was invalid.
		s.logger.Warn("Loaded token was invalid according to RTM API.")
		// The invalid token should have been cleared within checkAndHandleInitialAuthState.
	}

	// 4. Store Verified Token If Necessary.
	// If authentication was successful (either from a loaded token or a recent auth flow),
	// ensure the current token is saved to storage.
	s.storeVerifiedTokenIfNeeded() // Logs status internally.

	// 5. Finalize Initialization.
	s.initialized = true
	statusMsg := "Not Authenticated"
	if s.IsAuthenticated() {
		statusMsg = fmt.Sprintf("Authenticated as %q", s.GetUsername())
	}
	s.logger.Info(fmt.Sprintf("Initialization complete. Status: %s.", statusMsg))
	return nil // Return nil on successful initialization, even if not authenticated.
}

// checkPrerequisites verifies required configuration is present.
func (s *Service) checkPrerequisites() error {
	s.logger.Info("Checking configuration (API Key/Secret)...")
	if s.config.RTM.APIKey == "" || s.config.RTM.SharedSecret == "" {
		s.logger.Error("-> Configuration Check Failed: RTM API Key or Shared Secret is missing.")
		return errors.New("RTM API key and shared secret are required")
	}
	s.logger.Info("-> Configuration OK.")
	return nil
}

// loadAndSetTokenFromStorage attempts to load a token from storage and set it on the client.
// Returns true if a token was found and loaded, false otherwise.
func (s *Service) loadAndSetTokenFromStorage() bool {
	s.logger.Info("Loading saved authentication token...")
	if s.tokenStorage == nil {
		s.logger.Info("-> Skipped (Token storage not configured).")
		return false
	}

	token, err := s.tokenStorage.LoadToken()
	if err != nil {
		s.logger.Warn("-> Failed to load token.", "error", err)
		return false
	} else if token != "" {
		s.client.SetAuthToken(token) // Set token on the underlying client.
		return true
	}
	s.logger.Info("-> No saved token found.")
	return false
}

// checkAndHandleInitialAuthState verifies the current token with the RTM API.
// If the token is invalid, it clears it from the client and storage.
// Returns an error only if the API call itself fails.
func (s *Service) checkAndHandleInitialAuthState(ctx context.Context) error {
	s.logger.Info("Verifying saved token with RTM...")
	currentToken := s.client.GetAuthToken()
	if currentToken == "" {
		s.logger.Info("-> Skipped (No token to verify).")
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return nil // Not an error, just not authenticated.
	}

	// Call the underlying client to check the token against the RTM API.
	authState, err := s.client.GetAuthState(ctx)

	if err != nil {
		s.logger.Warn("-> Verification API call failed.")
		s.logger.Warn("RTM token verification API call failed.", "error", err) // Log specific error.

		// Clear potentially invalid token from client and storage.
		s.clearTokenFromClientAndStorage("Clearing potentially invalid token due to API error.")
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return err // Return the API call error.
	}

	// API call succeeded, update state based on RTM's response.
	s.updateAuthState(authState)
	if authState.IsAuthenticated {
		s.logger.Info(fmt.Sprintf("-> Token verified successfully (User: %q).", authState.Username))
	} else {
		s.logger.Warn("-> Token reported as invalid by RTM.")
		// Clear the invalid token from client and storage.
		s.clearTokenFromClientAndStorage("Clearing invalid token reported by RTM.")
	}
	return nil // No error from the check process itself, even if token was invalid.
}

// storeVerifiedTokenIfNeeded saves the currently set (and verified) auth token if it's not already stored correctly.
func (s *Service) storeVerifiedTokenIfNeeded() {
	if s.tokenStorage == nil {
		s.logger.Debug("Skipping token save check: Token storage not configured.")
		return
	}
	currentToken := s.client.GetAuthToken()
	// Only save if we are currently authenticated and have a token.
	if currentToken == "" || !s.IsAuthenticated() {
		s.logger.Debug("Skipping token save check: Not authenticated or no token set.")
		return
	}

	s.logger.Info("Checking if token needs saving...")
	storedToken, loadErr := s.tokenStorage.LoadToken() // Logs attempt/source/result internally.

	// Save if loading failed or the stored token doesn't match the current valid one.
	if loadErr != nil || storedToken != currentToken {
		s.logger.Info("-> Saving verified token to storage.")
		userID, username := s.getUserInfoFromState()
		if saveErr := s.tokenStorage.SaveToken(currentToken, userID, username); saveErr != nil {
			s.logger.Warn("-> Failed to save token.", "error", saveErr)
		} else {
			s.logger.Info("-> Token successfully saved to storage.")
		}
	} else {
		s.logger.Info("-> Token already saved correctly.")
	}
}

// clearTokenFromClientAndStorage removes the token from the client and attempts to delete from storage.
// Takes a reason string for logging purposes.
func (s *Service) clearTokenFromClientAndStorage(reason string) {
	if s.client.GetAuthToken() != "" {
		s.logger.Info(fmt.Sprintf("-> %s.", reason))
		s.client.SetAuthToken("")
		if s.tokenStorage != nil {
			if delErr := s.tokenStorage.DeleteToken(); delErr != nil {
				s.logger.Warn("Failed to delete token from storage.", "error", delErr)
			} else {
				s.logger.Info("-> Successfully deleted token from storage.")
			}
		}
	}
}

// --- Auth State Management ---.

// IsAuthenticated checks if the service currently holds a verified authentication state.
func (s *Service) IsAuthenticated() bool {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authState != nil && s.authState.IsAuthenticated
}

// GetUsername returns the authenticated user's username, or an empty string if not authenticated.
func (s *Service) GetUsername() string {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState == nil {
		return ""
	}
	return s.authState.Username
}

// GetAuthState retrieves the current authentication state, potentially refreshing it by calling the RTM API.
// It updates the internally cached state.
func (s *Service) GetAuthState(ctx context.Context) (*AuthState, error) {
	// Delegate to the client's GetAuthState for the actual API check.
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		// Update internal state to reflect failure before returning error.
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return nil, errors.Wrap(err, "failed to get auth state from RTM client")
	}
	// Update internal cached state with the result from the API.
	s.updateAuthState(authState)
	return authState, nil
}

// updateAuthState safely updates the cached authentication state using a mutex.
func (s *Service) updateAuthState(newState *AuthState) {
	s.authMutex.Lock()
	defer s.authMutex.Unlock()
	if newState == nil {
		// Ensure authState is never nil, default to non-authenticated.
		s.authState = &AuthState{IsAuthenticated: false}
	} else {
		s.authState = newState
	}
}

// getUserInfoFromState safely gets user ID and username from the cached auth state.
func (s *Service) getUserInfoFromState() (userID, username string) {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState != nil {
		return s.authState.UserID, s.authState.Username
	}
	return "", ""
}

// --- Auth Lifecycle ---.

// StartAuth begins the RTM authentication flow by obtaining an authentication URL from the client.
// Returns the URL the user needs to visit to grant permissions.
func (s *Service) StartAuth(ctx context.Context) (string, error) {
	s.logger.Info("Starting RTM authentication flow (getting auth URL)...")
	// Frob is obtained but not directly used by the service initiator here.
	authURL, _, err := s.client.StartAuthFlow(ctx)
	if err != nil {
		s.logger.Error("-> Failed to start auth flow.", "error", err)
		return "", err
	}
	s.logger.Info("-> Auth URL generated.")
	return authURL, nil
}

// CompleteAuth exchanges the provided 'frob' (obtained after user authorization) for a permanent auth token.
// It verifies the new token, updates the internal auth state, and saves the token to storage.
func (s *Service) CompleteAuth(ctx context.Context, frob string) error {
	s.logger.Info("Completing RTM authentication flow (exchanging code for token)...")
	// The client's CompleteAuthFlow handles the API call and sets the token internally on success.
	token, err := s.client.CompleteAuthFlow(ctx, frob)
	if err != nil {
		s.logger.Error("-> Failed to complete auth flow.", "error", err)
		return err // Return the error from the client.
	}

	// If CompleteAuthFlow succeeded, the client now holds the token. Verify it.
	authState, stateErr := s.client.GetAuthState(ctx)
	if stateErr != nil {
		s.logger.Error("-> Failed to verify auth state after getting token.", "error", stateErr)
		s.updateAuthState(&AuthState{IsAuthenticated: false}) // Ensure state is consistent.
		// Even though we got a token, verification failed, so return an error.
		return errors.Wrap(stateErr, "failed to confirm auth state after completing auth flow")
	}
	s.updateAuthState(authState) // Update cached state.

	// If verification succeeded and we are now authenticated, save the token.
	if s.IsAuthenticated() {
		s.logger.Info(fmt.Sprintf("-> Authentication successful (User: %q).", s.GetUsername()))
		if s.tokenStorage != nil && token != "" {
			s.logger.Info("-> Saving new token...")
			userID, username := s.getUserInfoFromState()
			if saveErr := s.tokenStorage.SaveToken(token, userID, username); saveErr != nil {
				// Log failure to save but don't return it as an error for CompleteAuth itself.
				s.logger.Warn("-> Failed to save new token.", "error", saveErr)
			} else {
				s.logger.Info("-> Successfully saved token to storage.")
			}
		} else if token != "" {
			// Log if storage is unavailable.
			s.logger.Warn("-> Token storage not available, cannot persist new authentication.")
		}
	} else {
		// This case indicates an issue (e.g., API inconsistency).
		s.logger.Warn("-> Authentication flow seemed complete, but state verification failed.")
		return errors.New("authentication flow completed but state verification failed")
	}

	return nil // Authentication successful.
}

// SetAuthToken explicitly sets the auth token on the underlying client.
// It also attempts to verify the token and update the persisted storage if valid.
// If the provided token is empty, it clears the current authentication.
func (s *Service) SetAuthToken(token string) {
	s.logger.Info("Explicitly setting RTM auth token.")
	s.client.SetAuthToken(token) // Set on the client first.

	if token == "" {
		// If setting an empty token, clear auth state and storage.
		s.logger.Info("-> Clearing authentication because empty token was set.")
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		if s.tokenStorage != nil {
			if err := s.tokenStorage.DeleteToken(); err != nil {
				s.logger.Warn("Failed to delete token from storage while clearing auth.", "error", err)
			}
		}
		return
	}

	// Verify the newly set token.
	s.logger.Info("-> Verifying manually set token...")
	ctx := context.Background() // Use background context for this internal verification.
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		s.logger.Warn("-> Failed to verify manually set token, clearing state.", "error", err)
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		s.client.SetAuthToken("") // Clear invalid token from client too.
		// Optionally try deleting from storage if it might exist there.
		if s.tokenStorage != nil {
			_ = s.tokenStorage.DeleteToken() // Ignore error here.
		}
		return
	}

	// Update internal state.
	s.updateAuthState(authState)

	// If token is valid and storage is available, attempt to save it.
	if s.IsAuthenticated() {
		s.logger.Info(fmt.Sprintf("-> Manually set token verified (User: %q).", s.GetUsername()))
		s.storeVerifiedTokenIfNeeded() // Reuse logic to save if needed.
	} else {
		s.logger.Warn("-> Manually set token appears invalid after check, not saving.")
		// Token was already cleared from client by GetAuthState if invalid.
	}
}

// GetAuthToken returns the current auth token held by the client, if any.
func (s *Service) GetAuthToken() string {
	return s.client.GetAuthToken()
}

// ClearAuth clears the current authentication state, removes the token from the client,
// and attempts to delete the token from storage.
func (s *Service) ClearAuth() error {
	s.logger.Info("Clearing RTM authentication...")
	s.client.SetAuthToken("")                             // Remove from client.
	s.updateAuthState(&AuthState{IsAuthenticated: false}) // Update cached state.

	if s.tokenStorage != nil {
		if err := s.tokenStorage.DeleteToken(); err != nil {
			// Don't return error if token wasn't found, but log other errors.
			if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errors.New("keyring: item not found")) { // Check common not found errors.
				s.logger.Error("-> Failed to clear token from storage.", "error", err)
				return errors.Wrap(err, "failed to delete token from storage")
			}
			s.logger.Debug("-> No token found in storage to delete.")
		} else {
			s.logger.Info("-> Successfully deleted token from storage.")
		}
	}
	s.logger.Info("-> Authentication cleared.")
	return nil
}

// Shutdown performs cleanup for the RTM service. Currently a no-op.
func (s *Service) Shutdown() error {
	s.logger.Info("Shutting down RTM service.")
	// Add cleanup tasks here if needed (e.g., close persistent connections).
	return nil
}

// GetName returns the service name identifier ("rtm").
func (s *Service) GetName() string {
	return "rtm" // Service identifier.
}

// GetClient returns the underlying RTM API client instance.
// Returns nil if the service or its client is not initialized properly.
func (s *Service) GetClient() *Client {
	if s == nil {
		return nil
	}
	return s.client
}

// GetClientAPIEndpoint returns the API endpoint URL used by the service's client.
// Returns an empty string if the service or client is not configured.
func (s *Service) GetClientAPIEndpoint() string {
	if s == nil || s.client == nil {
		return ""
	}
	// Delegate to the client's method.
	return s.client.GetAPIEndpoint()
}

// GetTokenStorageInfo returns details about the configured token storage mechanism.
// Returns method ("secure", "file", "none", "unknown"), path/description, and availability status.
func (s *Service) GetTokenStorageInfo() (method string, path string, available bool) {
	if s.tokenStorage == nil {
		return "none", "", false
	}

	// Use type assertions to determine the storage type.
	switch storage := s.tokenStorage.(type) {
	case *SecureTokenStorage:
		// Path is conceptual for secure storage.
		return "secure", "OS keychain/credentials manager", storage.IsAvailable()
	case *FileTokenStorage:
		// Path is the actual file path. Assumed available if created.
		return "file", storage.path, storage.IsAvailable()
	default:
		// Unknown storage type.
		return "unknown", "", s.tokenStorage.IsAvailable()
	}
}
