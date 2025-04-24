// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// This file defines the rtm.Service struct and makes it implement the services.Service interface,
// coordinating authentication, API calls, and MCP interactions.
package rtm

// file: internal/rtm/service.go
// TODO: #2 Set defaults to match https://www.rememberthemilk.com/services/api/methods/rtm.settings.getList.rtm .

import (
	"context"
	"encoding/json" // Required for CallTool args and ReadResource response marshalling.
	"fmt"
	"net/url" // Required for ReadResource URI parsing.
	"os"
	"path/filepath"
	"strings" // Required for ReadResource URI parsing and CallTool name splitting.
	"sync"

	// Removed time import as it's likely not needed here directly anymore.

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"

	// Use mcptypes alias defined below.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/services" // Use services interface.
)

// Service provides Remember The Milk functionality.
type Service struct {
	client       *Client
	config       *config.Config
	logger       logging.Logger
	authState    *AuthState
	authMutex    sync.RWMutex
	tokenStorage TokenStorageInterface
	initialized  bool
}

// Compile-time check to ensure *Service implements services.Service.
var _ services.Service = (*Service)(nil)

// NewService creates a new RTM service instance.
func NewService(cfg *config.Config, logger logging.Logger) *Service {
	// (Constructor logic remains the same as previous version...).
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	serviceLogger := logger.WithField("component", "rtm_service")

	rtmConfig := Config{
		APIKey:       cfg.RTM.APIKey,
		SharedSecret: cfg.RTM.SharedSecret,
	}
	client := NewClient(rtmConfig, logger)

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

	tokenStorage, err := NewTokenStorage(tokenPath, logger)
	if err != nil {
		serviceLogger.Warn("Failed to initialize token storage. Token persistence disabled.", "error", err)
		tokenStorage = nil
	}

	return &Service{
		client:       client,
		config:       cfg,
		logger:       serviceLogger,
		authState:    &AuthState{},
		tokenStorage: tokenStorage,
		initialized:  false,
	}
}

// Initialize prepares the RTM service for use.
func (s *Service) Initialize(ctx context.Context) error {
	// (Initialize logic remains the same as previous version...).
	if s.initialized {
		s.logger.Info("RTM Service already initialized.")
		return nil
	}
	s.logger.Info("Initializing RTM Service...")
	if err := s.checkPrerequisites(); err != nil {
		s.logger.Error("-> Initialization Failed: Prerequisites not met.", "error", err)
		return err
	}
	tokenFound := s.loadAndSetTokenFromStorage()
	verificationErr := s.checkAndHandleInitialAuthState(ctx)
	if verificationErr != nil {
		s.logger.Warn("Initial RTM authentication check failed.", "error", verificationErr)
		s.updateAuthState(&AuthState{IsAuthenticated: false})
	} else if tokenFound && !s.IsAuthenticated() {
		s.logger.Warn("Loaded token was invalid according to RTM API.")
	}
	s.storeVerifiedTokenIfNeeded()
	s.initialized = true
	statusMsg := "Not Authenticated"
	if s.IsAuthenticated() {
		statusMsg = fmt.Sprintf("Authenticated as %q", s.GetUsername())
	}
	s.logger.Info(fmt.Sprintf("Initialization complete. Status: %s.", statusMsg))
	return nil
}

// --- services.Service Interface Implementation ---.

// GetName returns the unique identifier for the service ("rtm").
func (s *Service) GetName() string {
	return "rtm"
}

// GetTools returns the list of MCP Tool definitions provided by the RTM service.
// Tool names are prefixed with "rtm_".
func (s *Service) GetTools() []mcptypes.Tool { // <--- CORRECTED: Added mcptypes. prefix.
	// Tool definitions moved here from mcp_tools.go GetTools.
	return []mcptypes.Tool{ // <--- CORRECTED: Added mcptypes. prefix.
		{
			Name:        "rtm_getTasks",
			Description: "Retrieves tasks from Remember The Milk based on an optional filter.",
			InputSchema: s.getTasksInputSchema(),                                               // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Get RTM Tasks", ReadOnlyHint: true}, // <--- CORRECTED: Added mcptypes. prefix.
		},
		{
			Name:        "rtm_createTask",
			Description: "Creates a new task in Remember The Milk using smart-add syntax.",
			InputSchema: s.createTaskInputSchema(),                           // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Create RTM Task"}, // <--- CORRECTED: Added mcptypes. prefix.
		},
		{
			Name:        "rtm_completeTask",
			Description: "Marks a specific task as complete in Remember The Milk.",
			InputSchema: s.completeTaskInputSchema(),                                                                        // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Complete RTM Task", DestructiveHint: true, IdempotentHint: true}, // <--- CORRECTED: Added mcptypes. prefix.
		},
		{
			Name:        "rtm_getAuthStatus",
			Description: "Checks and returns the current authentication status with Remember The Milk.",
			InputSchema: s.emptyInputSchema(),                                                          // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Check RTM Auth Status", ReadOnlyHint: true}, // <--- CORRECTED: Added mcptypes. prefix.
		},
		{
			Name:        "rtm_authenticate",
			Description: "Initiates or completes the authentication flow with Remember The Milk.",
			InputSchema: s.authenticationInputSchema(),                             // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Authenticate with RTM"}, // <--- CORRECTED: Added mcptypes. prefix.
		},
		{
			Name:        "rtm_clearAuth",
			Description: "Clears the stored Remember The Milk authentication token, effectively logging out.",
			InputSchema: s.emptyInputSchema(),                                                                                      // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Clear RTM Authentication", DestructiveHint: true, IdempotentHint: true}, // <--- CORRECTED: Added mcptypes. prefix.
		},
	}
}

// GetResources returns the MCP resources provided by this service.
func (s *Service) GetResources() []mcptypes.Resource {
	return []mcptypes.Resource{
		{
			Name:        "RTM Authentication Status",
			URI:         "rtm://auth",
			Description: "Provides the current authentication status with Remember The Milk (RTM).",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Lists",
			URI:         "rtm://lists",
			Description: "Lists available in your Remember The Milk account.",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Tags",
			URI:         "rtm://tags",
			Description: "Tags used in your Remember The Milk account.",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Tasks (Default Filter)",
			URI:         "rtm://tasks",
			Description: "Tasks in your Remember The Milk account (default view). Use rtm://tasks?filter=... for specific filters.",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Settings",
			URI:         "rtm://settings",
			Description: "User settings from your Remember The Milk account including timezone, date format preferences, and default list.",
			MimeType:    "application/json",
		},
	}
}

// ReadResource handles requests to read data from an RTM resource.
func (s *Service) ReadResource(ctx context.Context, uri string) ([]interface{}, error) {
	if !s.initialized {
		return nil, errors.New("RTM service is not initialized")
	}
	s.logger.Info("Handling ReadResource request.", "uri", uri)

	switch {
	case uri == "rtm://auth":
		authState, err := s.GetAuthState(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get auth state for resource")
		}
		return s.createJSONResourceContent(uri, authState)

	case uri == "rtm://lists":
		if !s.IsAuthenticated() {
			return s.notAuthenticatedResourceContent(uri), nil
		}
		lists, err := s.client.GetLists(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get lists for resource")
		}
		return s.createJSONResourceContent(uri, lists)

	case uri == "rtm://tags":
		if !s.IsAuthenticated() {
			return s.notAuthenticatedResourceContent(uri), nil
		}
		tags, err := s.client.GetTags(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get tags for resource")
		}
		return s.createJSONResourceContent(uri, tags)

	case uri == "rtm://settings":
		if !s.IsAuthenticated() {
			return s.notAuthenticatedResourceContent(uri), nil
		}
		settings, err := s.client.GetSettings(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get settings for resource")
		}
		return s.createJSONResourceContent(uri, settings)

	case uri == "rtm://tasks":
		return s.readTasksResourceWithFilter(ctx, "", uri)

	case strings.HasPrefix(uri, "rtm://tasks?"):
		filter, err := extractFilterFromURI(uri)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse filter from tasks URI: %s", uri)
		}
		return s.readTasksResourceWithFilter(ctx, filter, uri)

	default:
		return nil, mcperrors.NewResourceError(mcperrors.ErrResourceNotFound,
			fmt.Sprintf("Unknown RTM resource URI: %s", uri),
			nil,
			map[string]interface{}{"uri": uri})
	}
}

// CallTool handles incoming MCP tool execution requests directed at the RTM service.
// Delegates to specific handler functions defined in mcp_tools.go.
func (s *Service) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcptypes.CallToolResult, error) { // <--- CORRECTED: Added mcptypes. prefix.
	if !s.initialized {
		s.logger.Error("CallTool attempted before RTM service initialization.", "toolName", name)
		return s.serviceNotInitializedError(), nil // Calls helper in helpers.go.
	}

	if !strings.HasPrefix(name, "rtm_") {
		s.logger.Warn("Received tool call with unexpected prefix or format.", "toolName", name)
		return s.unknownToolError(name), nil // Calls helper in helpers.go.
	}
	baseToolName := strings.TrimPrefix(name, "rtm_")
	s.logger.Info("Routing RTM tool call.", "fullToolName", name, "baseToolName", baseToolName)

	var handlerFunc func(context.Context, json.RawMessage) (*mcptypes.CallToolResult, error) // <--- CORRECTED: Added mcptypes. prefix.

	// Route based on the base tool name, mapping to handlers in mcp_tools.go.
	switch baseToolName {
	case "getTasks":
		handlerFunc = s.handleGetTasks // Assumes exists in mcp_tools.go.
	case "createTask":
		handlerFunc = s.handleCreateTask // Assumes exists in mcp_tools.go.
	case "completeTask":
		handlerFunc = s.handleCompleteTask // Assumes exists in mcp_tools.go.
	case "getAuthStatus":
		handlerFunc = s.handleGetAuthStatus // Assumes exists in mcp_tools.go.
	case "authenticate":
		handlerFunc = s.handleAuthenticate // Assumes exists in mcp_tools.go.
	case "clearAuth":
		handlerFunc = s.handleClearAuth // Assumes exists in mcp_tools.go.
	default:
		s.logger.Warn("Received call for unknown RTM tool.", "fullToolName", name, "baseToolName", baseToolName)
		return s.unknownToolError(name), nil // Calls helper in helpers.go.
	}

	// Execute the mapped handler function.
	result, err := handlerFunc(ctx, args)
	if err != nil {
		// Internal error within the handler itself (e.g., bad marshalling).
		s.logger.Error("Internal error executing RTM tool handler.", "fullToolName", name, "error", fmt.Sprintf("%+v", err))
		return s.internalToolError(), nil // Calls helper in helpers.go.
	}

	// Return the result from the specific handler.
	return result, nil
}

// Shutdown performs cleanup tasks for the RTM service.
func (s *Service) Shutdown() error {
	s.logger.Info("Shutting down RTM service.")
	return nil
}

// IsAuthenticated returns true if the service currently has valid authentication.
func (s *Service) IsAuthenticated() bool {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authState != nil && s.authState.IsAuthenticated
}

// --- Internal Helper Functions ---.
// (Only keep helpers directly used by the interface methods above if they weren't moved).

// readTasksResourceWithFilter fetches tasks based on filter. (Internal helper for ReadResource).
func (s *Service) readTasksResourceWithFilter(ctx context.Context, filter string, resourceURI string) ([]interface{}, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent(resourceURI), nil // Calls helper in helpers.go.
	}
	tasks, err := s.client.GetTasks(ctx, filter)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get tasks for resource (filter: '%s')", filter)
	}
	return s.createJSONResourceContent(resourceURI, tasks) // Calls helper in helpers.go.
}

// extractFilterFromURI parses the 'filter' query parameter. (Internal helper for ReadResource).
func extractFilterFromURI(uriString string) (string, error) {
	parsedURL, err := url.Parse(uriString)
	if err != nil {
		return "", errors.Wrapf(err, "invalid URI format: %s", uriString)
	}
	return parsedURL.Query().Get("filter"), nil
}

// --- Auth State and Lifecycle Helpers ---.
// (Keep these as they are internal to the service's operation).
func (s *Service) checkPrerequisites() error { /* ... as before ... */
	s.logger.Info("Checking configuration (API Key/Secret)...")
	if s.config.RTM.APIKey == "" || s.config.RTM.SharedSecret == "" {
		s.logger.Error("-> Configuration Check Failed: RTM API Key or Shared Secret is missing.")
		return errors.New("RTM API key and shared secret are required")
	}
	s.logger.Info("-> Configuration OK.")
	return nil
}
func (s *Service) loadAndSetTokenFromStorage() bool { /* ... as before ... */
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
		s.client.SetAuthToken(token)
		return true
	}
	s.logger.Info("-> No saved token found.")
	return false
}
func (s *Service) checkAndHandleInitialAuthState(ctx context.Context) error { /* ... as before ... */
	s.logger.Info("Verifying saved token with RTM...")
	currentToken := s.client.GetAuthToken()
	if currentToken == "" {
		s.logger.Info("-> Skipped (No token to verify).")
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return nil
	}
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		s.logger.Warn("-> Verification API call failed.")
		s.logger.Warn("RTM token verification API call failed.", "error", err)
		s.clearTokenFromClientAndStorage("Clearing potentially invalid token due to API error.")
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return err
	}
	s.updateAuthState(authState)
	if authState.IsAuthenticated {
		s.logger.Info(fmt.Sprintf("-> Token verified successfully (User: %q).", authState.Username))
	} else {
		s.logger.Warn("-> Token reported as invalid by RTM.")
		s.clearTokenFromClientAndStorage("Clearing invalid token reported by RTM.")
	}
	return nil
}
func (s *Service) storeVerifiedTokenIfNeeded() { /* ... as before ... */
	if s.tokenStorage == nil {
		s.logger.Debug("Skipping token save check: Token storage not configured.")
		return
	}
	currentToken := s.client.GetAuthToken()
	if currentToken == "" || !s.IsAuthenticated() {
		s.logger.Debug("Skipping token save check: Not authenticated or no token set.")
		return
	}
	s.logger.Info("Checking if token needs saving...")
	storedToken, loadErr := s.tokenStorage.LoadToken()
	if loadErr != nil || storedToken != currentToken {
		s.logger.Info("-> Saving verified token to storage.")
		userID, username := s.getUserInfoFromState()
		if saveErr := s.tokenStorage.SaveToken(currentToken, userID, username); saveErr != nil {
			s.logger.Warn("-> Failed to save token.", "error", saveErr)
		} else {
			s.logger.Info("-> Successfully saved token to storage.")
		}
	} else {
		s.logger.Info("-> Token already saved correctly.")
	}
}
func (s *Service) clearTokenFromClientAndStorage(reason string) { /* ... as before ... */
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

// GetUsername returns the RTM username if authenticated, otherwise an empty string.
func (s *Service) GetUsername() string { /* ... as before ... */
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState == nil {
		return ""
	}
	return s.authState.Username
}

// GetAuthState checks the current token's validity and returns the auth state.
func (s *Service) GetAuthState(ctx context.Context) (*AuthState, error) { /* ... as before ... */
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return nil, errors.Wrap(err, "failed to get auth state from RTM client")
	}
	s.updateAuthState(authState)
	return authState, nil
}
func (s *Service) updateAuthState(newState *AuthState) { /* ... as before ... */
	s.authMutex.Lock()
	defer s.authMutex.Unlock()
	if newState == nil {
		s.authState = &AuthState{IsAuthenticated: false}
	} else {
		s.authState = newState
	}
}
func (s *Service) getUserInfoFromState() (userID, username string) { /* ... as before ... */
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState != nil {
		return s.authState.UserID, s.authState.Username
	}
	return "", ""
}

// StartAuth initiates the RTM authentication flow and returns the authorization URL.
func (s *Service) StartAuth(ctx context.Context) (string, error) { /* ... as before ... */
	s.logger.Info("Starting RTM authentication flow (getting auth URL)...")
	authURL, _, err := s.client.StartAuthFlow(ctx)
	if err != nil {
		s.logger.Error("-> Failed to start auth flow.", "error", err)
		return "", err
	}
	s.logger.Info("-> Auth URL generated.")
	return authURL, nil
}

// CompleteAuth exchanges the provided 'frob' for an authentication token.
func (s *Service) CompleteAuth(ctx context.Context, frob string) error { /* ... as before ... */
	s.logger.Info("Completing RTM authentication flow (exchanging code for token)...")
	token, err := s.client.CompleteAuthFlow(ctx, frob)
	if err != nil {
		s.logger.Error("-> Failed to complete auth flow.", "error", err)
		return err
	}
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
			} else {
				s.logger.Info("-> Successfully saved token to storage.")
			}
		} else if token != "" {
			s.logger.Warn("-> Token storage not available, cannot persist new authentication.")
		}
	} else {
		s.logger.Warn("-> Authentication flow seemed complete, but state verification failed.")
		return errors.New("authentication flow completed but state verification failed")
	}
	return nil
}

// SetAuthToken manually sets an authentication token and verifies it.
func (s *Service) SetAuthToken(token string) { /* ... as before ... */
	s.logger.Info("Explicitly setting RTM auth token.")
	s.client.SetAuthToken(token)
	if token == "" {
		s.logger.Info("-> Clearing authentication because empty token was set.")
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		if s.tokenStorage != nil {
			if err := s.tokenStorage.DeleteToken(); err != nil {
				s.logger.Warn("Failed to delete token from storage while clearing auth.", "error", err)
			}
		}
		return
	}
	s.logger.Info("-> Verifying manually set token...")
	ctx := context.Background()
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		s.logger.Warn("-> Failed to verify manually set token, clearing state.", "error", err)
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		s.client.SetAuthToken("")
		if s.tokenStorage != nil {
			_ = s.tokenStorage.DeleteToken()
		}
		return
	}
	s.updateAuthState(authState)
	if s.IsAuthenticated() {
		s.logger.Info(fmt.Sprintf("-> Manually set token verified (User: %q).", s.GetUsername()))
		s.storeVerifiedTokenIfNeeded()
	} else {
		s.logger.Warn("-> Manually set token appears invalid after check, not saving.")
	}
}

// GetAuthToken returns the current authentication token stored in the client.
func (s *Service) GetAuthToken() string { /* ... as before ... */
	return s.client.GetAuthToken()
}

// ClearAuth clears the stored RTM authentication token.
func (s *Service) ClearAuth() error { /* ... as before ... */
	s.logger.Info("Clearing RTM authentication...")
	s.client.SetAuthToken("")
	s.updateAuthState(&AuthState{IsAuthenticated: false})
	if s.tokenStorage != nil {
		if err := s.tokenStorage.DeleteToken(); err != nil {
			if !errors.Is(err, os.ErrNotExist) && !strings.Contains(strings.ToLower(err.Error()), "not found") {
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

// --- Other Public Service Methods ---.

// GetClient returns the underlying RTM API client instance.
func (s *Service) GetClient() *Client { /* ... as before ... */
	if s == nil {
		return nil
	}
	return s.client
}

// GetClientAPIEndpoint returns the API endpoint URL used by the client.
func (s *Service) GetClientAPIEndpoint() string { /* ... as before ... */
	if s == nil || s.client == nil {
		return ""
	}
	return s.client.GetAPIEndpoint()
}

// GetTokenStorageInfo returns details about the token storage mechanism being used.
func (s *Service) GetTokenStorageInfo() (method string, path string, available bool) { /* ... as before ... */
	if s.tokenStorage == nil {
		return "none", "", false
	}
	switch storage := s.tokenStorage.(type) {
	case *SecureTokenStorage:
		return "secure", "OS keychain/credentials manager", storage.IsAvailable()
	case *FileTokenStorage:
		return "file", storage.path, storage.IsAvailable()
	default:
		return "unknown", "", s.tokenStorage.IsAvailable()
	}
}

// GetPrompt handles requests to retrieve a specific prompt template.
// Currently, the RTM service does not support prompts.
func (s *Service) GetPrompt(_ context.Context, name string, _ map[string]string) (*mcptypes.GetPromptResult, error) {
	// Now s.logger refers to the logger field in your RTM Service struct.
	s.logger.Warn("GetPrompt called, but RTM service does not support prompts.", "promptName", name)

	// Return an error indicating the feature isn't supported by this service.
	// Using ErrMethodNotFound or a specific "Not Supported" error is appropriate.
	// Let's use MethodNotFound for now, aligning with how unsupported actions are handled.
	return nil, mcperrors.NewMethodNotFoundError( // mcperrors is now correctly imported and used.
		fmt.Sprintf("Prompt support (prompts/get) is not implemented by the RTM service for prompt '%s'", name),
		nil, // No underlying Go error cause.
		map[string]interface{}{"promptName": name},
	)

	// Alternatively, to return an empty success response (if the spec allows):
	// return &mcptypes.GetPromptResult{Messages: []mcptypes.PromptMessage{}}, nil.
}

// Ensure your RTM service struct (e.g., Service) correctly implements.
// the services.Service interface by having all the required methods, including GetPrompt.
// NOTE: Tool handler implementations (handleGetTasks, etc.) are expected.
// to be in mcp_tools.go.
// NOTE: Helper functions (successToolResult, etc.) are expected.
// to be in helpers.go.
// NOTE: Input schema helpers (getTasksInputSchema, etc.) are expected.
// to be in helpers.go.
