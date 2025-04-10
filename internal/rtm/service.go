// file: internal/rtm/service.go
package rtm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
)

// Service provides Remember The Milk functionality to the MCP server.
// It implements the necessary interfaces to provide MCP tools and resources.
type Service struct {
	client       *Client
	config       *config.Config
	logger       logging.Logger
	authState    *AuthState
	authMutex    sync.RWMutex
	tokenStorage *TokenStorage
	initialized  bool
}

// NewService creates a new RTM service with the given configuration.
func NewService(cfg *config.Config, logger logging.Logger) *Service {
	// Use no-op logger if not provided
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	serviceLogger := logger.WithField("component", "rtm_service")

	rtmConfig := Config{
		APIKey:       cfg.RTM.APIKey,
		SharedSecret: cfg.RTM.SharedSecret,
	}

	client := NewClient(rtmConfig, logger)

	// Set up token storage path
	tokenPath := cfg.Auth.TokenPath
	if tokenPath == "" {
		// Default to home directory if not specified
		homeDir, err := os.UserHomeDir()
		if err == nil {
			tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json")
		} else {
			// Fallback to current directory if home not available
			tokenPath = "rtm_token.json"
			serviceLogger.Warn("Could not determine home directory for token storage",
				"error", err,
				"fallbackPath", tokenPath)
		}
	}

	// Initialize token storage
	tokenStorage, err := NewTokenStorage(tokenPath, logger)
	if err != nil {
		serviceLogger.Warn("Failed to initialize token storage", "error", err)
		// Continue without token storage (authentication will be temporary)
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

// Initialize initializes the RTM service.
// It checks authentication status and loads the auth token if available.
func (s *Service) Initialize(ctx context.Context) error {
	s.logger.Info("Initializing RTM service")

	// Check for required configuration
	if s.config.RTM.APIKey == "" || s.config.RTM.SharedSecret == "" {
		return errors.New("RTM API key and shared secret are required")
	}

	// Try to load token from storage
	if s.tokenStorage != nil {
		token, err := s.tokenStorage.LoadToken()
		if err != nil {
			s.logger.Warn("Failed to load auth token from storage", "error", err)
			// Continue initialization even if token loading fails
		} else if token != "" {
			s.logger.Info("Loaded auth token from storage")
			s.client.SetAuthToken(token)
		}
	}

	// Check auth state regardless of token source
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		s.logger.Warn("Failed to get auth state", "error", err)
		// If we failed with a loaded token, clear it
		if s.client.GetAuthToken() != "" {
			s.logger.Info("Clearing invalid auth token")
			s.client.SetAuthToken("")
			if s.tokenStorage != nil {
				if err := s.tokenStorage.DeleteToken(); err != nil {
					s.logger.Warn("Failed to delete invalid token from storage", "error", err)
				}
			}
		}
	} else {
		s.authMutex.Lock()
		s.authState = authState
		s.authMutex.Unlock()

		// If we have a valid token that's not stored yet, store it
		if s.IsAuthenticated() && s.tokenStorage != nil && s.client.GetAuthToken() != "" {
			// Check if we need to store the token
			storedToken, err := s.tokenStorage.LoadToken()
			if err != nil || storedToken != s.client.GetAuthToken() {
				s.logger.Info("Storing valid auth token")
				err := s.tokenStorage.SaveToken(
					s.client.GetAuthToken(),
					s.authState.UserID,
					s.authState.Username)
				if err != nil {
					s.logger.Warn("Failed to save auth token to storage", "error", err)
				}
			}
		}
	}

	s.initialized = true
	s.logger.Info("RTM service initialized",
		"authenticated", s.IsAuthenticated(),
		"username", s.GetUsername())

	return nil
}

// IsAuthenticated returns whether the service is authenticated with RTM.
func (s *Service) IsAuthenticated() bool {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authState.IsAuthenticated
}

// GetUsername returns the username of the authenticated user.
func (s *Service) GetUsername() string {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authState.Username
}

// GetAuthState returns the current authentication state.
func (s *Service) GetAuthState(ctx context.Context) (*AuthState, error) {
	// Refresh the auth state from the API
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get auth state from RTM")
	}

	// Update our cached state
	s.authMutex.Lock()
	s.authState = authState
	s.authMutex.Unlock()

	return authState, nil
}

// StartAuth begins the authentication flow.
// It returns a URL that the user needs to visit to authorize the application.
func (s *Service) StartAuth(ctx context.Context) (string, error) {
	s.logger.Info("Starting RTM auth flow")
	return s.client.StartAuthFlow(ctx)
}

// CompleteAuth completes the authentication flow using the frob.
func (s *Service) CompleteAuth(ctx context.Context, frob string) error {
	s.logger.Info("Completing RTM auth flow", "frob", frob)

	// Complete the auth flow
	if err := s.client.CompleteAuthFlow(ctx, frob); err != nil {
		return errors.Wrap(err, "failed to complete auth flow")
	}

	// Update auth state
	authState, err := s.client.GetAuthState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get auth state after auth flow")
	}

	s.authMutex.Lock()
	s.authState = authState
	s.authMutex.Unlock()

	// Save auth token to secure storage
	if s.tokenStorage != nil && s.IsAuthenticated() {
		token := s.client.GetAuthToken()
		if token != "" {
			s.logger.Info("Saving auth token to storage")
			err := s.tokenStorage.SaveToken(token, authState.UserID, authState.Username)
			if err != nil {
				s.logger.Warn("Failed to save auth token to storage", "error", err)
				// Continue even if token saving fails
			}
		}
	}

	return nil
}

// SetAuthToken explicitly sets the auth token.
func (s *Service) SetAuthToken(token string) {
	s.client.SetAuthToken(token)

	// Update storage if available
	if s.tokenStorage != nil && token != "" {
		// Try to get user info with this token
		ctx := context.Background()
		authState, err := s.client.GetAuthState(ctx)
		if err != nil {
			s.logger.Warn("Failed to get auth state for token", "error", err)
			return
		}

		s.authMutex.Lock()
		s.authState = authState
		s.authMutex.Unlock()

		if s.IsAuthenticated() {
			s.logger.Info("Saving manually set auth token to storage")
			err := s.tokenStorage.SaveToken(token, authState.UserID, authState.Username)
			if err != nil {
				s.logger.Warn("Failed to save auth token to storage", "error", err)
			}
		}
	}
}

// GetAuthToken returns the current auth token for storage.
func (s *Service) GetAuthToken() string {
	return s.client.GetAuthToken()
}

// ClearAuth clears the current authentication.
func (s *Service) ClearAuth() error {
	s.logger.Info("Clearing RTM authentication")

	// Clear client token
	s.client.SetAuthToken("")

	// Clear auth state
	s.authMutex.Lock()
	s.authState = &AuthState{IsAuthenticated: false}
	s.authMutex.Unlock()

	// Clear token from storage
	if s.tokenStorage != nil {
		err := s.tokenStorage.DeleteToken()
		if err != nil {
			return errors.Wrap(err, "failed to delete token from storage")
		}
	}

	return nil
}

// Shutdown performs any cleanup needed.
func (s *Service) Shutdown() error {
	s.logger.Info("Shutting down RTM service")
	// Nothing to clean up for now
	return nil
}

// GetName returns the name of this service.
func (s *Service) GetName() string {
	return "rtm"
}

// --------------------------------
// MCP Tools Implementation
// --------------------------------

// GetTools returns the MCP tools provided by this service.
func (s *Service) GetTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "getTasks",
			Description: "Retrieves tasks from Remember The Milk based on a specified filter.",
			InputSchema: s.getTasksInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:        "Get RTM Tasks",
				ReadOnlyHint: true, // This tool doesn't modify any data
			},
		},
		{
			Name:        "createTask",
			Description: "Creates a new task in Remember The Milk.",
			InputSchema: s.createTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Create RTM Task",
				ReadOnlyHint:    false, // This tool modifies data
				DestructiveHint: false, // It's not destructive, just additive
				IdempotentHint:  false, // Multiple calls with same args will create multiple tasks
			},
		},
		{
			Name:        "completeTask",
			Description: "Marks a task as complete in Remember The Milk.",
			InputSchema: s.completeTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Complete RTM Task",
				ReadOnlyHint:    false, // This tool modifies data
				DestructiveHint: true,  // It changes the state of a task
				IdempotentHint:  true,  // Multiple calls with same taskId will have same effect
			},
		},
		{
			Name:        "getAuthStatus",
			Description: "Gets the authentication status with Remember The Milk.",
			InputSchema: s.emptyInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:        "Check RTM Auth Status",
				ReadOnlyHint: true, // This tool doesn't modify any data
			},
		},
		{
			Name:        "authenticate",
			Description: "Initiates or completes the authentication flow with Remember The Milk.",
			InputSchema: s.authenticationInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Authenticate with RTM",
				ReadOnlyHint:    false, // This tool modifies data (auth state)
				DestructiveHint: false, // Not destructive
				IdempotentHint:  false, // Each call may generate a new auth URL
			},
		},
		{
			Name:        "clearAuth",
			Description: "Clears the current Remember The Milk authentication.",
			InputSchema: s.emptyInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Clear RTM Authentication",
				ReadOnlyHint:    false, // This tool modifies data (auth state)
				DestructiveHint: true,  // It removes authentication
				IdempotentHint:  true,  // Multiple calls have same effect
			},
		},
	}
}

// CallTool handles MCP tool calls for this service.
func (s *Service) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Make sure we're initialized
	if !s.initialized {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "RTM service is not initialized",
				},
			},
		}, nil
	}

	// Route the call to the appropriate handler
	switch name {
	case "getTasks":
		return s.handleGetTasks(ctx, args)
	case "createTask":
		return s.handleCreateTask(ctx, args)
	case "completeTask":
		return s.handleCompleteTask(ctx, args)
	case "getAuthStatus":
		return s.handleGetAuthStatus(ctx, args)
	case "authenticate":
		return s.handleAuthenticate(ctx, args)
	case "clearAuth":
		return s.handleClearAuth(ctx, args)
	default:
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Unknown tool: %s", name),
				},
			},
		}, nil
	}
}

// handleGetTasks handles the getTasks tool call.
func (s *Service) handleGetTasks(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Check authentication
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}

	// Parse the arguments
	var params struct {
		Filter string `json:"filter"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid arguments: %v", err),
				},
			},
		}, nil
	}

	// Call the RTM API
	tasks, err := s.client.GetTasks(ctx, params.Filter)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error getting tasks: %v", err),
				},
			},
		}, nil
	}

	// Format the response
	var responseText string
	if len(tasks) == 0 {
		responseText = fmt.Sprintf("No tasks found matching filter: '%s'", params.Filter)
	} else {
		responseText = fmt.Sprintf("Found %d tasks matching filter: '%s'\n\n", len(tasks), params.Filter)
		for i, task := range tasks {
			responseText += fmt.Sprintf("%d. %s", i+1, task.Name)

			// Add due date if available
			if !task.DueDate.IsZero() {
				responseText += fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2"))
			}

			// Add priority if available
			if task.Priority > 0 && task.Priority < 4 {
				responseText += fmt.Sprintf(", priority: %d", task.Priority)
			}

			// Add tags if available
			if len(task.Tags) > 0 {
				responseText += fmt.Sprintf(", tags: %s", strings.Join(task.Tags, ", "))
			}

			responseText += "\n"
		}
	}

	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: responseText,
			},
		},
	}, nil
}

// handleCreateTask handles the createTask tool call.
func (s *Service) handleCreateTask(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Check authentication
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}

	// Parse the arguments
	var params struct {
		Name string `json:"name"`
		List string `json:"list,omitempty"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid arguments: %v", err),
				},
			},
		}, nil
	}

	// Get list ID if a list name was provided
	var listID string
	if params.List != "" {
		lists, err := s.client.GetLists(ctx)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Error getting lists: %v", err),
					},
				},
			}, nil
		}

		// Find the list by name (case insensitive)
		for _, list := range lists {
			if strings.EqualFold(list.Name, params.List) {
				listID = list.ID
				break
			}
		}

		// If list not found, return error
		if listID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("List not found: %s", params.List),
					},
				},
			}, nil
		}
	}

	// Call the RTM API
	task, err := s.client.CreateTask(ctx, params.Name, listID)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error creating task: %v", err),
				},
			},
		}, nil
	}

	// Format the response
	responseText := fmt.Sprintf("Successfully created task: '%s'", task.Name)
	if !task.DueDate.IsZero() {
		responseText += fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2"))
	}
	responseText += fmt.Sprintf("\nTask ID: %s", task.ID)

	// Add list info if available
	if listID != "" {
		responseText += fmt.Sprintf("\nList: %s", params.List)
	}

	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: responseText,
			},
		},
	}, nil
}

// handleCompleteTask handles the completeTask tool call.
func (s *Service) handleCompleteTask(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Check authentication
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}

	// Parse the arguments
	var params struct {
		TaskID string `json:"taskId"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid arguments: %v", err),
				},
			},
		}, nil
	}

	// Call the RTM API
	err := s.client.CompleteTask(ctx, params.TaskID)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error completing task: %v", err),
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Successfully completed task with ID: %s", params.TaskID),
			},
		},
	}, nil
}

// handleGetAuthStatus handles the getAuthStatus tool call.
func (s *Service) handleGetAuthStatus(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Get current auth state
	authState, err := s.GetAuthState(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error getting auth status: %v", err),
				},
			},
		}, nil
	}

	var responseText string
	if authState.IsAuthenticated {
		responseText = fmt.Sprintf("Authenticated with Remember The Milk as user: %s", authState.Username)
		if authState.FullName != "" {
			responseText += fmt.Sprintf(" (%s)", authState.FullName)
		}
	} else {
		// Generate auth URL
		authURL, err := s.StartAuth(ctx)
		if err != nil {
			responseText = fmt.Sprintf("Not authenticated. Failed to generate auth URL: %v", err)
		} else {
			responseText = "Not authenticated with Remember The Milk.\n\n"
			responseText += "To authenticate, please visit the following URL and authorize CowGnition:\n"
			responseText += authURL
		}
	}

	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: responseText,
			},
		},
	}, nil
}

// --------------------------------
// MCP Resources Implementation
// --------------------------------

// GetResources returns the MCP resources provided by this service.
func (s *Service) GetResources() []mcp.Resource {
	return []mcp.Resource{
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
			Name:        "RTM Tasks",
			URI:         "rtm://tasks",
			Description: "Tasks in your Remember The Milk account.",
			MimeType:    "application/json",
		},
	}
}

// ReadResource handles MCP resource read requests for this service.
func (s *Service) ReadResource(ctx context.Context, uri string) ([]interface{}, error) {
	// Make sure we're initialized
	if !s.initialized {
		return nil, errors.New("RTM service is not initialized")
	}

	// Route based on URI
	switch uri {
	case "rtm://auth":
		return s.readAuthResource(ctx)
	case "rtm://lists":
		return s.readListsResource(ctx)
	case "rtm://tags":
		return s.readTagsResource(ctx)
	case "rtm://tasks":
		return s.readTasksResource(ctx)
	default:
		// Check if it's a filtered tasks URI
		if strings.HasPrefix(uri, "rtm://tasks?filter=") {
			filter := strings.TrimPrefix(uri, "rtm://tasks?filter=")
			return s.readTasksResourceWithFilter(ctx, filter)
		}
		return nil, errors.Newf("unknown resource URI: %s", uri)
	}
}

// readAuthResource provides the authentication resource.
func (s *Service) readAuthResource(ctx context.Context) ([]interface{}, error) {
	// Get current auth state
	authState, err := s.GetAuthState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get auth state")
	}

	// Convert to TextResourceContents
	authJSON, err := json.MarshalIndent(authState, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal auth state")
	}

	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      "rtm://auth",
				MimeType: "application/json",
			},
			Text: string(authJSON),
		},
	}, nil
}

// readListsResource provides the lists resource.
func (s *Service) readListsResource(ctx context.Context) ([]interface{}, error) {
	// Check authentication
	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent("rtm://lists"), nil
	}

	// Get lists
	lists, err := s.client.GetLists(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get lists")
	}

	// Convert to TextResourceContents
	listsJSON, err := json.MarshalIndent(lists, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal lists")
	}

	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      "rtm://lists",
				MimeType: "application/json",
			},
			Text: string(listsJSON),
		},
	}, nil
}

// readTagsResource provides the tags resource.
func (s *Service) readTagsResource(ctx context.Context) ([]interface{}, error) {
	// Check authentication
	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent("rtm://tags"), nil
	}

	// Get tags
	tags, err := s.client.GetTags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tags")
	}

	// Convert to TextResourceContents
	tagsJSON, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal tags")
	}

	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      "rtm://tags",
				MimeType: "application/json",
			},
			Text: string(tagsJSON),
		},
	}, nil
}

// readTasksResource provides the tasks resource (all tasks).
func (s *Service) readTasksResource(ctx context.Context) ([]interface{}, error) {
	return s.readTasksResourceWithFilter(ctx, "")
}

// readTasksResourceWithFilter provides filtered tasks.
func (s *Service) readTasksResourceWithFilter(ctx context.Context, filter string) ([]interface{}, error) {
	// Check authentication
	if !s.IsAuthenticated() {
		uri := "rtm://tasks"
		if filter != "" {
			uri += "?filter=" + filter
		}
		return s.notAuthenticatedResourceContent(uri), nil
	}

	// Get tasks
	tasks, err := s.client.GetTasks(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tasks")
	}

	// Convert to TextResourceContents
	tasksJSON, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal tasks")
	}

	uri := "rtm://tasks"
	if filter != "" {
		uri += "?filter=" + filter
	}

	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      uri,
				MimeType: "application/json",
			},
			Text: string(tasksJSON),
		},
	}, nil
}

// --------------------------------
// Helper Methods
// --------------------------------

// notAuthenticatedError returns a standard error for unauthenticated calls.
func (s *Service) notAuthenticatedError() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Not authenticated with Remember The Milk. Use the getAuthStatus tool to get an authentication URL.",
			},
		},
	}
}

// notAuthenticatedResourceContent returns standard content for unauthenticated resource requests.
func (s *Service) notAuthenticatedResourceContent(uri string) []interface{} {
	content := map[string]interface{}{
		"error":   "not_authenticated",
		"message": "Not authenticated with Remember The Milk",
	}

	contentJSON, _ := json.MarshalIndent(content, "", "  ")

	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      uri,
				MimeType: "application/json",
			},
			Text: string(contentJSON),
		},
	}
}

// --------------------------------
// Input Schema Definitions
// --------------------------------

// getTasksInputSchema returns the schema for the getTasks tool.
func (s *Service) getTasksInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "RTM filter expression (e.g., 'list:Inbox status:incomplete dueBefore:tomorrow'). See RTM documentation for filter syntax.",
			},
		},
		"required": []string{"filter"},
	}

	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// createTaskInputSchema returns the schema for the createTask tool.
func (s *Service) createTaskInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the task, including any smart syntax (e.g., 'Buy milk ^tomorrow #groceries !1').",
			},
			"list": map[string]interface{}{
				"type":        "string",
				"description": "Optional. The name of the list to add the task to. Defaults to Inbox if not specified.",
			},
		},
		"required": []string{"name"},
	}

	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// completeTaskInputSchema returns the schema for the completeTask tool.
func (s *Service) completeTaskInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"taskId": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the task to mark as complete.",
			},
		},
		"required": []string{"taskId"},
	}

	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// handleAuthenticate handles the authenticate tool call.
func (s *Service) handleAuthenticate(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Parse the arguments
	var params struct {
		Frob string `json:"frob"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid arguments: %v", err),
				},
			},
		}, nil
	}

	// If frob is provided, complete authentication
	if params.Frob != "" {
		err := s.CompleteAuth(ctx, params.Frob)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Authentication completion failed: %v", err),
					},
				},
			}, nil
		}

		// Check auth state
		if !s.IsAuthenticated() {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Authentication completed, but verification failed. Please try again.",
					},
				},
			}, nil
		}

		return &mcp.CallToolResult{
			IsError: false,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Successfully authenticated with Remember The Milk as user: %s", s.GetUsername()),
				},
			},
		}, nil
	}

	// Otherwise, start authentication
	authURL, err := s.StartAuth(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to start authentication: %v", err),
				},
			},
		}, nil
	}

	// Extract frob from URL (for convenience)
	frobParam := ""
	if parts := strings.Split(authURL, "&frob="); len(parts) > 1 {
		frobParam = parts[1]
	}

	responseText := "To authenticate with Remember The Milk, please follow these steps:\n\n"
	responseText += "1. Visit this URL to authorize CowGnition:\n" + authURL + "\n\n"
	responseText += "2. After authorization, return here and use the authenticate tool again with the frob parameter:\n"

	if frobParam != "" {
		responseText += "   - Frob: " + frobParam + "\n\n"
		responseText += "You can use this command: authenticate(frob: \"" + frobParam + "\")"
	} else {
		responseText += "   - Use the frob parameter that will be provided in the URL"
	}

	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: responseText,
			},
		},
	}, nil
}

// handleClearAuth handles the clearAuth tool call.
func (s *Service) handleClearAuth(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Check if we're authenticated first
	if !s.IsAuthenticated() {
		return &mcp.CallToolResult{
			IsError: false,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Not currently authenticated with Remember The Milk.",
				},
			},
		}, nil
	}

	// Get username for the response message
	username := s.GetUsername()

	// Clear auth
	err := s.ClearAuth()
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to clear authentication: %v", err),
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Successfully cleared authentication for user: %s", username),
			},
		},
	}, nil
}

// authenticationInputSchema returns the schema for the authenticate tool.
func (s *Service) authenticationInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"frob": map[string]interface{}{
				"type":        "string",
				"description": "Optional. The frob code from RTM to complete authentication. If not provided, authentication will be initiated.",
			},
		},
	}

	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// emptyInputSchema returns a schema for tools that take no input.
func (s *Service) emptyInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}
