// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// This file defines the rtm.Service struct and makes it implement the services.Service interface,
// coordinating authentication, API calls, and MCP interactions.
package rtm

// file: internal/rtm/service.go (Lint fixes applied)
// TODO: #3 Respect RTM rate limits and handle retries.
// file: internal/rtm/service.go (Final lint fixes for ineffassign)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/services"
)

// Define a constant for the maximum tasks to return in a single response.
const maxTasksToReturn = 100 // Adjust this limit as needed.

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
func (s *Service) GetTools() []mcptypes.Tool {
	return []mcptypes.Tool{
		{
			Name:        "rtm_getTasks",
			Description: "Retrieves tasks from Remember The Milk based on an optional filter.",
			InputSchema: s.getTasksInputSchema(), // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Get RTM Tasks", ReadOnlyHint: true},
		},
		{
			Name:        "rtm_createTask",
			Description: "Creates a new task in Remember The Milk using smart-add syntax.",
			InputSchema: s.createTaskInputSchema(), // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Create RTM Task"},
		},
		{
			Name:        "rtm_completeTask",
			Description: "Marks a specific task as complete in Remember The Milk.",
			InputSchema: s.completeTaskInputSchema(), // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Complete RTM Task", DestructiveHint: true, IdempotentHint: true},
		},
		{
			Name:        "rtm_getAuthStatus",
			Description: "Checks and returns the current authentication status with Remember The Milk.",
			InputSchema: s.emptyInputSchema(), // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Check RTM Auth Status", ReadOnlyHint: true},
		},
		{
			Name:        "rtm_authenticate",
			Description: "Initiates or completes the authentication flow with Remember The Milk.",
			InputSchema: s.authenticationInputSchema(), // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Authenticate with RTM"},
		},
		{
			Name:        "rtm_clearAuth",
			Description: "Clears the stored Remember The Milk authentication token, effectively logging out.",
			InputSchema: s.emptyInputSchema(), // Calls helper in helpers.go.
			Annotations: &mcptypes.ToolAnnotations{Title: "Clear RTM Authentication", DestructiveHint: true, IdempotentHint: true},
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
		return s.createJSONResourceContent(uri, authState) // Calls helper in helpers.go

	case uri == "rtm://lists":
		if !s.IsAuthenticated() {
			return s.notAuthenticatedResourceContent(uri), nil // Calls helper in helpers.go
		}
		lists, err := s.client.GetLists(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get lists for resource")
		}
		return s.createJSONResourceContent(uri, lists) // Calls helper in helpers.go

	case uri == "rtm://tags":
		if !s.IsAuthenticated() {
			return s.notAuthenticatedResourceContent(uri), nil // Calls helper in helpers.go
		}
		tags, err := s.client.GetTags(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get tags for resource")
		}
		return s.createJSONResourceContent(uri, tags) // Calls helper in helpers.go

	case uri == "rtm://settings":
		if !s.IsAuthenticated() {
			return s.notAuthenticatedResourceContent(uri), nil // Calls helper in helpers.go
		}
		settings, err := s.client.GetSettings(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get settings for resource")
		}
		return s.createJSONResourceContent(uri, settings) // Calls helper in helpers.go

	case uri == "rtm://tasks":
		// Call the function that handles default/fallback filter logic
		return s.readTasksResourceWithFilter(ctx, "", uri) // Pass empty filter

	case strings.HasPrefix(uri, "rtm://tasks?"):
		// Extract the filter from the URI
		filter, err := extractFilterFromURI(uri)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse filter from tasks URI: %s", uri)
		}
		// Call the function with the extracted filter
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
func (s *Service) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcptypes.CallToolResult, error) {
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

	var handlerFunc func(context.Context, json.RawMessage) (*mcptypes.CallToolResult, error)

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
	// Add cleanup logic here if needed (e.g., closing network connections if client used them)
	return nil
}

// IsAuthenticated returns true if the service currently has valid authentication.
func (s *Service) IsAuthenticated() bool {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authState != nil && s.authState.IsAuthenticated
}

// GetPrompt handles requests to retrieve a specific prompt template.
// Currently, the RTM service does not support prompts.
func (s *Service) GetPrompt(_ context.Context, name string, _ map[string]string) (*mcptypes.GetPromptResult, error) {
	s.logger.Warn("GetPrompt called, but RTM service does not support prompts.", "promptName", name)
	return nil, mcperrors.NewMethodNotFoundError(
		fmt.Sprintf("Prompt support (prompts/get) is not implemented by the RTM service for prompt '%s'", name),
		nil,
		map[string]interface{}{"promptName": name},
	)
}

// --- Internal Helper Functions ---.

// --- Helper method to fetch and process tasks with a given filter ---
// This centralizes the task fetching and result formatting logic, including truncation.
func (s *Service) fetchAndProcessTasks(ctx context.Context, filter string, resourceURI string) ([]interface{}, error) {
	s.logger.Debug("Fetching tasks with filter.", "filter", filter)
	tasks, err := s.client.GetTasks(ctx, filter) // Fetch tasks using the RTM client

	if err != nil {
		s.logger.Error("Failed to get tasks with filter.", "filter", filter, "error", err)
		// Return the RTM API error directly, wrapped for context
		return nil, errors.Wrapf(err, "failed to get tasks for resource (filter: '%s')", filter)
	}

	// Log the number of tasks retrieved
	totalFound := 0
	if tasks != nil {
		totalFound = len(tasks)
	}
	s.logger.Info("Tasks retrieved from RTM.", "filter", filter, "count", totalFound, "resourceURI", resourceURI)

	// Initialize tasks slice if nil
	if tasks == nil {
		tasks = []Task{}
	}

	// --- Truncation Logic ---
	returnedTasks := tasks
	truncated := false
	if totalFound > maxTasksToReturn {
		returnedTasks = tasks[:maxTasksToReturn] // Take only the first N tasks
		truncated = true
		s.logger.Warn("Task list truncated due to size limit.",
			"filter", filter,
			"totalFound", totalFound,
			"returned", maxTasksToReturn)
	}
	// --- End Truncation Logic ---

	// Create the response payload, including truncation info
	responsePayload := map[string]interface{}{
		"tasks":      returnedTasks, // Use the potentially truncated list
		"filter":     filter,
		"totalFound": totalFound,
		"returned":   len(returnedTasks),
		"truncated":  truncated,
		"isEmpty":    totalFound == 0, // isEmpty is true only if totalFound is 0
		"message":    "",              // Initialize message field
	}

	// Add a message based on the result
	if totalFound == 0 {
		responsePayload["message"] = "No tasks found matching the filter."
	} else if truncated {
		responsePayload["message"] = fmt.Sprintf("Found %d tasks, returning the first %d.", totalFound, len(returnedTasks))
	} else {
		responsePayload["message"] = fmt.Sprintf("Found %d tasks.", totalFound)
	}

	// Use the JSON resource content helper (defined in helpers.go) to wrap this payload
	return s.createJSONResourceContent(resourceURI, responsePayload)
}

// --- Helper method to create a response for empty task lists ---
// Ensures a consistent structure when no tasks are found by any strategy.
func (s *Service) createEmptyTasksResponse(filter string, resourceURI string) ([]interface{}, error) {
	responsePayload := map[string]interface{}{
		"tasks":      []Task{}, // Empty array, not null
		"filter":     filter,
		"message":    "No tasks found matching the filter or default strategies.",
		"isEmpty":    true,
		"totalFound": 0, // Explicitly set counts for empty case
		"returned":   0,
		"truncated":  false,
	}
	s.logger.Debug("Creating empty tasks response.", "filter", filter, "uri", resourceURI)
	// Use the JSON resource content helper (defined in helpers.go) to wrap this payload
	return s.createJSONResourceContent(resourceURI, responsePayload)
}

// --- Helper function to check if the result from fetchAndProcessTasks is effectively empty ---.
func (s *Service) isResultEmpty(results []interface{}) bool {
	if len(results) == 0 {
		return true
	}
	// Check the payload structure created by fetchAndProcessTasks/createEmptyTasksResponse
	contentMap, ok1 := results[0].(mcptypes.TextResourceContents)
	if !ok1 {
		s.logger.Warn("isResultEmpty: Unexpected result structure, assuming not empty.", "type", fmt.Sprintf("%T", results[0]))
		return false // Unexpected structure, assume not empty
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(contentMap.Text), &data); err != nil {
		s.logger.Warn("isResultEmpty: Failed to unmarshal result content, assuming not empty.", "error", err)
		return false // Cannot parse, assume not empty
	}

	// Check for the isEmpty flag or zero returned count
	if isEmpty, ok2 := data["isEmpty"].(bool); ok2 && isEmpty {
		return true
	}
	// JSON numbers unmarshal as float64 by default
	if returnedFloat, ok2 := data["returned"].(float64); ok2 && returnedFloat == 0 {
		return true
	}
	// Handle potential integer type if JSON number handling changes
	if returnedInt, ok2 := data["returned"].(int); ok2 && returnedInt == 0 {
		return true
	}

	return false // Default assumption
}

// nolint: gocyclo // Acknowledging complexity due to multiple strategies
// readTasksResourceWithFilter fetches tasks, trying multiple filter strategies if no explicit filter is provided.
func (s *Service) readTasksResourceWithFilter(ctx context.Context, filter string, resourceURI string) ([]interface{}, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent(resourceURI), nil // Calls helper from helpers.go
	}

	// --- Explicit Filter Provided ---
	if filter != "" {
		s.logger.Info("Using explicitly provided filter.", "filter", filter, "resourceURI", resourceURI)
		// Always add status:incomplete if no status is specified in the filter
		effectiveFilter := filter
		if !strings.Contains(filter, "status:") {
			effectiveFilter = fmt.Sprintf("(%s) AND status:incomplete", filter)
			s.logger.Debug("Adding 'status:incomplete' to explicit filter.", "originalFilter", filter, "effectiveFilter", effectiveFilter)
		}
		return s.fetchAndProcessTasks(ctx, effectiveFilter, resourceURI) // Calls updated helper
	}

	// --- No Explicit Filter - Trying Strategies ---
	s.logger.Debug("No filter provided, iterating through strategies.", "resourceURI", resourceURI)

	var settings *Settings
	var lists []TaskList
	var settingsErr, listsErr error

	// Fetch settings and lists once upfront if needed by strategies
	needsSettings := true // Assume needed for Strategy 1
	needsLists := true    // Assume needed for Strategy 2 & 3

	if needsSettings {
		settings, settingsErr = s.client.GetSettings(ctx)
		if settingsErr != nil {
			s.logger.Warn("Failed to get user settings for filter strategies.", "error", settingsErr)
		}
	}
	if needsLists {
		lists, listsErr = s.client.GetLists(ctx)
		if listsErr != nil {
			s.logger.Warn("Failed to get lists for filter strategies.", "error", listsErr)
		}
	}

	var lastError error // Stores error from the last *attempted* strategy

	// --- Strategy 1: Try with default list ID AND status:incomplete ---
	if settings != nil && settings.DefaultListID != "" {
		filter1 := fmt.Sprintf("list:%s AND status:incomplete", settings.DefaultListID) // ADDED status filter
		s.logger.Info("Strategy 1: Trying default list ID filter (incomplete only).", "filter", filter1, "listId", settings.DefaultListID)
		results, err := s.fetchAndProcessTasks(ctx, filter1, resourceURI)
		if err == nil && !s.isResultEmpty(results) {
			s.logger.Info("Strategy 1 successful - returned tasks.")
			return results, nil // Return results if not empty
		}
		if err != nil {
			s.logger.Warn("Strategy 1 failed with error.", "error", err)
			// Don't assign lastError here, only log
		} else {
			s.logger.Info("Strategy 1 found no tasks.")
		}
	} else {
		s.logger.Debug("Strategy 1: Skipped.")
	}

	// --- Strategy 2: Try with default list name AND status:incomplete ---
	if settings != nil && settings.DefaultListID != "" && lists != nil {
		var defaultListName string
		for _, list := range lists {
			if list.ID == settings.DefaultListID {
				defaultListName = list.Name
				break
			}
		}
		if defaultListName != "" {
			filter2 := fmt.Sprintf("list:\"%s\" AND status:incomplete", defaultListName) // ADDED status filter
			s.logger.Info("Strategy 2: Trying default list name filter (incomplete only).", "filter", filter2, "listName", defaultListName)
			results, err := s.fetchAndProcessTasks(ctx, filter2, resourceURI)
			if err == nil && !s.isResultEmpty(results) {
				s.logger.Info("Strategy 2 successful - returned tasks.")
				return results, nil // Return results if not empty
			}
			if err != nil {
				s.logger.Warn("Strategy 2 failed with error.", "error", err)
				// Don't assign lastError here
			} else {
				s.logger.Info("Strategy 2 found no tasks.")
			}
		} else {
			s.logger.Debug("Strategy 2: Skipped (name not found).")
		}
	} else {
		s.logger.Debug("Strategy 2: Skipped.")
	}

	// --- Strategy 3: Try with smart list "A-List" AND status:incomplete (ID first, then name) ---
	if lists != nil {
		var aListID, aListName string
		for _, list := range lists {
			if list.Name == "A-List" && list.SmartList {
				aListID = list.ID
				aListName = list.Name
				break
			}
		}
		if aListID != "" {
			filter3id := fmt.Sprintf("list:%s AND status:incomplete", aListID) // ADDED status filter
			s.logger.Info("Strategy 3 (ID): Trying A-List smart list ID (incomplete only).", "filter", filter3id)
			results, err := s.fetchAndProcessTasks(ctx, filter3id, resourceURI)
			if err == nil && !s.isResultEmpty(results) {
				s.logger.Info("Strategy 3 (ID) successful - returned tasks.")
				return results, nil
			}
			if err != nil {
				s.logger.Warn("Strategy 3 (ID) failed with error.", "error", err)
				// Don't assign lastError here
			} else {
				s.logger.Info("Strategy 3 (ID) found no tasks, trying name.")
			}
		}
		// Try name only if ID didn't yield results or wasn't found, AND aListName is valid
		if aListName != "" && (aListID == "" || s.isResultEmpty(nil)) { // Crude check needed refinement if possible
			filter3name := fmt.Sprintf("list:\"%s\" AND status:incomplete", aListName) // ADDED status filter
			s.logger.Info("Strategy 3 (Name): Trying A-List smart list name (incomplete only).", "filter", filter3name)
			results, err := s.fetchAndProcessTasks(ctx, filter3name, resourceURI)
			if err == nil && !s.isResultEmpty(results) {
				s.logger.Info("Strategy 3 (Name) successful - returned tasks.")
				return results, nil
			}
			if err != nil {
				s.logger.Warn("Strategy 3 (Name) failed with error.", "error", err)
				// Don't assign lastError here
			} else {
				s.logger.Info("Strategy 3 (Name) found no tasks.")
			}
		} else if aListID == "" {
			s.logger.Debug("Strategy 3: Skipped (A-List not found).")
		}
	} else {
		s.logger.Debug("Strategy 3: Skipped.")
	}

	// --- Strategy 4: Try status:incomplete search (This is now somewhat redundant but kept as final fallback) ---
	filter4 := "status:incomplete"
	s.logger.Info("Strategy 4: Trying status:incomplete filter (only).", "filter", filter4)
	results4, err4 := s.fetchAndProcessTasks(ctx, filter4, resourceURI)
	if err4 == nil && !s.isResultEmpty(results4) {
		s.logger.Info("Strategy 4 successful - returned tasks.")
		return results4, nil // Return results if not empty
	}
	// Don't assign lastError here

	if err4 != nil {
		s.logger.Warn("Strategy 4 failed with error.", "error", err4)
	} else {
		s.logger.Info("Strategy 4 found no tasks.")
	}

	// --- Strategy 5: Try with no filter BUT ADD status:incomplete ---
	filter5 := "status:incomplete" // Ensure we only get incomplete tasks even when checking all lists
	s.logger.Info("Strategy 5: Trying no list filter (incomplete only).", "filter", filter5)
	results5, err5 := s.fetchAndProcessTasks(ctx, filter5, resourceURI)
	if err5 == nil && !s.isResultEmpty(results5) {
		s.logger.Info("Strategy 5 successful - returned tasks.")
		return results5, nil // Return results if not empty
	}
	lastError = err5 // Store error from the final attempt

	// --- All Strategies Exhausted ---
	s.logger.Info("All strategies exhausted, no incomplete tasks found or final strategy failed.")
	if lastError != nil {
		s.logger.Error("Final strategy failed.", "error", lastError)
		// Fall through to return empty response for user-friendliness
	}

	// Return the custom empty response structure
	return s.createEmptyTasksResponse(filter, resourceURI) // Use the original requested filter (which was empty)
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
func (s *Service) checkPrerequisites() error {
	s.logger.Info("Checking configuration (API Key/Secret)...")
	if s.config.RTM.APIKey == "" || s.config.RTM.SharedSecret == "" {
		s.logger.Error("-> Configuration Check Failed: RTM API Key or Shared Secret is missing.")
		return errors.New("RTM API key and shared secret are required")
	}
	s.logger.Info("-> Configuration OK.")
	return nil
}
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
		s.client.SetAuthToken(token)
		return true
	}
	s.logger.Info("-> No saved token found.")
	return false
}
func (s *Service) checkAndHandleInitialAuthState(ctx context.Context) error {
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
func (s *Service) storeVerifiedTokenIfNeeded() {
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
	// Save if load failed OR stored token differs from current token
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

// GetUsername returns the RTM username if authenticated, otherwise an empty string.
func (s *Service) GetUsername() string {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState == nil {
		return ""
	}
	return s.authState.Username
}

// GetAuthState checks the current token's validity and returns the auth state.
func (s *Service) GetAuthState(ctx context.Context) (*AuthState, error) {
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		s.updateAuthState(&AuthState{IsAuthenticated: false})
		return nil, errors.Wrap(err, "failed to get auth state from RTM client")
	}
	s.updateAuthState(authState)
	return authState, nil
}
func (s *Service) updateAuthState(newState *AuthState) {
	s.authMutex.Lock()
	defer s.authMutex.Unlock()
	if newState == nil {
		s.authState = &AuthState{IsAuthenticated: false}
	} else {
		s.authState = newState
	}
}
func (s *Service) getUserInfoFromState() (userID, username string) {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	if s.authState != nil {
		return s.authState.UserID, s.authState.Username
	}
	return "", ""
}

// StartAuth initiates the RTM authentication flow and returns the authorization URL.
func (s *Service) StartAuth(ctx context.Context) (string, error) {
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
func (s *Service) CompleteAuth(ctx context.Context, frob string) error {
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
func (s *Service) SetAuthToken(token string) {
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
func (s *Service) GetAuthToken() string {
	return s.client.GetAuthToken()
}

// ClearAuth clears the stored RTM authentication token.
func (s *Service) ClearAuth() error {
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
func (s *Service) GetClient() *Client {
	if s == nil {
		return nil
	}
	return s.client
}

// GetClientAPIEndpoint returns the API endpoint URL used by the client.
func (s *Service) GetClientAPIEndpoint() string {
	if s == nil || s.client == nil {
		return ""
	}
	return s.client.GetAPIEndpoint()
}

// GetTokenStorageInfo returns details about the token storage mechanism being used.
func (s *Service) GetTokenStorageInfo() (method string, path string, available bool) {
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

// NOTE: Tool handler implementations (handleGetTasks, etc.) are expected
// to be in mcp_tools.go.
// NOTE: Helper functions (successToolResult, etc.) are expected
// to be in helpers.go.
// NOTE: Input schema helpers (getTasksInputSchema, etc.) are expected
// to be in helpers.go.
// NOTE: Helper functions createJSONResourceContent and notAuthenticatedResourceContent
// are expected to be in helpers.go.
