// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// This file specifically handles the integration with the Model Context Protocol (MCP)
// by defining the Tools provided by the RTM service and routing incoming tool calls.
// file: internal/rtm/mcp_tools.go.
package rtm

import (
	"context"
	"encoding/json"
	"fmt"     // Added.
	"strings" // Ensure strings is imported.

	"github.com/dkoosis/cowgnition/internal/mcp"
)

// GetTools returns the list of MCP Tool definitions provided by the RTM service.
// These definitions inform MCP clients (like AI assistants) about the capabilities
// offered, including descriptions and expected input schemas.
func (s *Service) GetTools() []mcp.Tool {
	// Tool definitions using helper methods for schemas.
	return []mcp.Tool{
		{
			Name:        "getTasks",
			Description: "Retrieves tasks from Remember The Milk based on an optional filter.",
			InputSchema: s.getTasksInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Get RTM Tasks", ReadOnlyHint: true},
		},
		{
			Name:        "createTask",
			Description: "Creates a new task in Remember The Milk using smart-add syntax.",
			InputSchema: s.createTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Create RTM Task"}, // Default hints (not read-only, not idempotent).
		},
		{
			Name:        "completeTask",
			Description: "Marks a specific task as complete in Remember The Milk.",
			InputSchema: s.completeTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Complete RTM Task", DestructiveHint: true, IdempotentHint: true},
		},
		{
			Name:        "getAuthStatus",
			Description: "Checks and returns the current authentication status with Remember The Milk.",
			InputSchema: s.emptyInputSchema(), // No arguments needed.
			Annotations: &mcp.ToolAnnotations{Title: "Check RTM Auth Status", ReadOnlyHint: true},
		},
		{
			Name:        "authenticate",
			Description: "Initiates or completes the authentication flow with Remember The Milk.",
			InputSchema: s.authenticationInputSchema(), // Schema defines optional 'frob' and 'autoComplete'.
			Annotations: &mcp.ToolAnnotations{Title: "Authenticate with RTM"},
		},
		{
			Name:        "clearAuth",
			Description: "Clears the stored Remember The Milk authentication token, effectively logging out.",
			InputSchema: s.emptyInputSchema(), // No arguments needed.
			Annotations: &mcp.ToolAnnotations{Title: "Clear RTM Authentication", DestructiveHint: true, IdempotentHint: true},
		},
		// Add definitions for other implemented tools here (e.g., getLists, getTags).
	}
}

// CallTool handles incoming MCP tool execution requests directed at the RTM service.
// It identifies the requested tool name, parses the arguments, calls the appropriate
// internal handler function, and formats the result (or error) as an MCP CallToolResult.
// Errors originating *within* the tool logic are returned in the CallToolResult.IsError field,
// while errors during the handling process itself (e.g., parsing, internal state) return a Go error.
func (s *Service) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Ensure the service has been initialized before handling tool calls.
	if !s.initialized {
		s.logger.Error("CallTool attempted before RTM service initialization.", "toolName", name)
		// Return the error result directly, but nil Go error as the handling itself didn't fail.
		return s.serviceNotInitializedError(), nil
	}

	s.logger.Info("Routing MCP tool call.", "toolName", name)

	var handlerFunc func(context.Context, json.RawMessage) (*mcp.CallToolResult, error)

	// Map tool name to the corresponding handler function.
	switch name {
	case "getTasks":
		handlerFunc = s.handleGetTasks
	case "createTask":
		handlerFunc = s.handleCreateTask
	case "completeTask":
		handlerFunc = s.handleCompleteTask
	case "getAuthStatus":
		handlerFunc = s.handleGetAuthStatus
	case "authenticate":
		handlerFunc = s.handleAuthenticate
	case "clearAuth":
		handlerFunc = s.handleClearAuth
	default:
		// If the tool name isn't recognized, return a specific tool error result.
		s.logger.Warn("Received call for unknown RTM tool.", "toolName", name)
		return s.unknownToolError(name), nil // Return error result, nil Go error.
	}

	// Execute the mapped handler function.
	result, err := handlerFunc(ctx, args)
	if err != nil {
		// This indicates an *internal* error within the handler function itself
		// (e.g., failed marshalling, unexpected panic), not an error from the RTM API
		// or expected tool failure condition.
		s.logger.Error("Internal error executing RTM tool handler.", "toolName", name, "error", fmt.Sprintf("%+v", err))
		// Return the internal error result, nil Go error.
		return s.internalToolError(), nil
	}

	// Return the result obtained from the handler function.
	return result, nil
}

// --- Tool Handler Implementations ---.
// (handleGetTasks, handleCreateTask, etc. - documentation added within each function).

// handleGetTasks implements the logic for the "getTasks" MCP tool.
// It requires authentication, parses the optional filter argument, calls the RTM API,
// and formats the returned tasks into a human-readable text response.
func (s *Service) handleGetTasks(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	// Ensure user is authenticated before proceeding.
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil // Return specific error result.
	}

	// Define expected parameters and parse input arguments.
	var params struct {
		Filter string `json:"filter,omitempty"` // Filter is optional.
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("getTasks", err), nil // Return arg error result.
	}

	s.logger.Info("Executing RTM getTasks tool.", "filter", params.Filter)
	// Call the underlying RTM client method.
	tasks, err := s.client.GetTasks(ctx, params.Filter)
	if err != nil {
		// If the API call fails, return an RTM API error result.
		return s.rtmAPIErrorResult("getting tasks", err), nil
	}

	// Format the successful response.
	var responseText strings.Builder // Use strings.Builder for efficiency.
	filterDisplay := params.Filter
	if filterDisplay == "" {
		filterDisplay = "(no filter)"
	}

	if len(tasks) == 0 {
		responseText.WriteString(fmt.Sprintf("No tasks found matching filter: '%s'.", filterDisplay))
	} else {
		responseText.WriteString(fmt.Sprintf("Found %d tasks matching filter: '%s'.\n\n", len(tasks), filterDisplay))
		maxTasksToShow := 15 // Limit the number of tasks displayed for brevity.
		for i, task := range tasks {
			if i >= maxTasksToShow {
				responseText.WriteString(fmt.Sprintf("...and %d more.\n", len(tasks)-maxTasksToShow))
				break
			}
			// Format each task concisely.
			responseText.WriteString(fmt.Sprintf("%d. %s", i+1, task.Name))
			if !task.DueDate.IsZero() {
				responseText.WriteString(fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2")))
			}
			if task.Priority > 0 && task.Priority < 4 {
				responseText.WriteString(fmt.Sprintf(", P%d", task.Priority)) // Use P1, P2, P3 format.
			}
			if len(task.Tags) > 0 {
				responseText.WriteString(fmt.Sprintf(", tags:[%s]", strings.Join(task.Tags, ","))) // More compact tag format.
			}
			responseText.WriteString(".\n") // End each task line with a period and newline.
		}
	}
	// Return a successful tool result containing the formatted text.
	return s.successToolResult(responseText.String()), nil
}

// handleCreateTask implements the logic for the "createTask" MCP tool.
// It requires authentication, parses the task name (and optional list), finds the
// target list ID if specified, calls the RTM API to create the task, and returns the result.
func (s *Service) handleCreateTask(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}
	var params struct {
		Name string `json:"name"`           // Task name, potentially with smart syntax.
		List string `json:"list,omitempty"` // Optional target list name.
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("createTask", err), nil
	}

	s.logger.Info("Executing RTM createTask tool.", "name", params.Name, "list", params.List)

	listID := ""             // Default to Inbox.
	listNameToLog := "Inbox" // For logging/response clarity.

	// If a list name is provided, find its ID.
	if params.List != "" {
		listNameToLog = params.List
		// Fetch all lists to find the ID matching the name.
		lists, err := s.client.GetLists(ctx)
		if err != nil {
			return s.rtmAPIErrorResult("getting lists to find ID for task creation", err), nil
		}
		found := false
		for _, list := range lists {
			// Case-insensitive comparison for list name.
			if strings.EqualFold(list.Name, params.List) {
				listID = list.ID
				found = true
				break
			}
		}
		if !found {
			// Return specific error if the provided list name doesn't exist.
			return s.simpleToolErrorResult(fmt.Sprintf("RTM list not found: '%s'.", params.List)), nil
		}
	}

	// Call the underlying RTM client method to create the task.
	task, err := s.client.CreateTask(ctx, params.Name, listID)
	if err != nil {
		return s.rtmAPIErrorResult("creating task", err), nil
	}

	// Format the success response.
	responseText := fmt.Sprintf("Successfully created task: '%s'", task.Name) // Use task name returned by API (after parsing).
	if !task.DueDate.IsZero() {
		responseText += fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2"))
	}
	responseText += fmt.Sprintf(" in list '%s'.", listNameToLog)
	responseText += fmt.Sprintf("\nTask ID: %s.", task.ID) // Include the new task ID.

	return s.successToolResult(responseText), nil
}

// handleCompleteTask implements the logic for the "completeTask" MCP tool.
// It requires authentication, parses the list and task IDs, calls the RTM API
// to mark the task complete, and returns a success message.
func (s *Service) handleCompleteTask(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}
	var params struct {
		TaskID string `json:"taskId"` // Combined task series and instance ID.
		ListID string `json:"listId"` // ID of the list containing the task.
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("completeTask", err), nil
	}
	// Both listId and taskId are required by the schema and API.
	if params.TaskID == "" || params.ListID == "" {
		return s.simpleToolErrorResult("Both taskId and listId are required to complete a task."), nil
	}

	s.logger.Info("Executing RTM completeTask tool.", "taskId", params.TaskID, "listId", params.ListID)

	// Call the underlying RTM client method.
	err := s.client.CompleteTask(ctx, params.ListID, params.TaskID)
	if err != nil {
		return s.rtmAPIErrorResult("completing task", err), nil
	}

	// Return simple success message.
	return s.successToolResult(fmt.Sprintf("Successfully completed task with ID: %s.", params.TaskID)), nil
}

// handleGetAuthStatus implements the logic for the "getAuthStatus" MCP tool.
// It checks the current authentication state using the service's cached/refreshed data
// and returns a message indicating the status and username, or instructions on how to authenticate.
func (s *Service) handleGetAuthStatus(ctx context.Context, _ json.RawMessage) (*mcp.CallToolResult, error) {
	s.logger.Info("Executing RTM getAuthStatus tool.")
	// Use the service's GetAuthState method which handles caching and verification.
	authState, err := s.GetAuthState(ctx)
	if err != nil {
		// If the state check itself fails, return that error.
		return s.rtmAPIErrorResult("getting auth status", err), nil
	}

	var responseText string
	if authState.IsAuthenticated {
		// Format authenticated message.
		responseText = fmt.Sprintf("Authenticated with Remember The Milk as user: %s", authState.Username)
		if authState.FullName != "" {
			responseText += fmt.Sprintf(" (%s)", authState.FullName)
		}
		responseText += "."
	} else {
		// Format not authenticated message with instructions.
		// Get a fresh auth URL and frob for the instructions.
		authURL, frob, startAuthErr := s.client.StartAuthFlow(ctx)
		if startAuthErr != nil {
			responseText = fmt.Sprintf("Not authenticated. Failed to generate RTM auth URL for instructions: %v.", startAuthErr)
		} else {
			responseText = "Not authenticated with Remember The Milk.\n\n"
			responseText += "To authenticate, please visit this URL:\n" + authURL + "\n\n"
			responseText += "After authorizing in the browser, use the 'authenticate' tool with the 'frob' code found in the URL's query parameters.\n"
			if frob != "" { // Only show example if frob was obtained.
				responseText += fmt.Sprintf("Example: authenticate(frob: \"%s\")", frob)
			}
		}
	}
	// Return success result containing the status message.
	return s.successToolResult(responseText), nil
}

// handleAuthenticate implements the logic for the "authenticate" MCP tool.
// It can either start a new authentication flow (returning instructions) or
// complete an existing flow if a 'frob' code is provided.
func (s *Service) handleAuthenticate(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		Frob string `json:"frob,omitempty"`
		// AutoComplete is currently unused in this implementation but kept for schema compatibility.
		// AutoComplete bool   `json:"autoComplete,omitempty"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("authenticate", err), nil
	}

	if params.Frob != "" {
		// --- Complete Authentication Flow ---.
		s.logger.Info("Attempting to complete RTM authentication with provided frob.", "tool", "authenticate")
		// Call the service's CompleteAuth method which handles token exchange and storage.
		err := s.CompleteAuth(ctx, params.Frob)
		if err != nil {
			// If completion fails (e.g., invalid frob, API error).
			return s.rtmAPIErrorResult("completing authentication", err), nil
		}

		// Verify authentication succeeded after completion.
		if !s.IsAuthenticated() {
			// This case indicates the frob might have been invalid or expired.
			return s.simpleToolErrorResult("Authentication failed. The provided 'frob' might be invalid or expired. Please try starting the authentication process again."), nil
		}

		// Return success message with username.
		return s.successToolResult(fmt.Sprintf("Successfully authenticated as user: %s.", s.GetUsername())), nil
	}
	// <<< FIX: Refactored else block according to revive suggestion >>>.

	// --- Start New Authentication Flow ---.
	s.logger.Info("Starting new RTM authentication flow.", "tool", "authenticate")
	// Get the auth URL and frob from the client.
	authURL, frob, err := s.client.StartAuthFlow(ctx)
	if err != nil {
		return s.rtmAPIErrorResult("starting authentication", err), nil
	}

	// Provide instructions back to the user/client on how to proceed.
	responseText := "To authenticate with Remember The Milk:\n\n"
	responseText += "1. Visit this URL in your browser: " + authURL + "\n\n"
	responseText += "2. Click 'Allow Access' or 'OK, I'll Allow It' to grant permission.\n\n"
	responseText += "3. After authorizing, complete the authentication by calling this tool again with the 'frob' parameter found in the URL you visited.\n"
	responseText += fmt.Sprintf("   Example: authenticate(frob: \"%s\")\n", frob) // Include the actual frob.

	return s.successToolResult(responseText), nil
}

// handleClearAuth implements the logic for the "clearAuth" MCP tool.
// It clears the stored authentication token and updates the service state.
func (s *Service) handleClearAuth(_ context.Context, _ json.RawMessage) (*mcp.CallToolResult, error) {
	s.logger.Info("Executing RTM clearAuth tool.")
	if !s.IsAuthenticated() {
		// If already not authenticated, it's still a success.
		return s.successToolResult("Not currently authenticated with RTM."), nil
	}
	// Get username before clearing for the success message.
	username := s.GetUsername()
	// Call the service's ClearAuth method.
	err := s.ClearAuth()
	if err != nil {
		// Handle potential errors during token deletion from storage.
		return s.rtmAPIErrorResult("clearing authentication", err), nil
	}
	// Return success message.
	return s.successToolResult(fmt.Sprintf("Successfully cleared RTM authentication for user: %s.", username)), nil
}

// --- Input Schema Definitions Helpers ---.
// These functions generate the JSON schema definitions for tool inputs.

// getTasksInputSchema defines the schema for the "getTasks" tool.
func (s *Service) getTasksInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Optional. RTM filter expression (e.g., 'list:Inbox status:incomplete dueBefore:tomorrow'). If omitted, retrieves default tasks.",
			},
		},
		// Filter is optional, so no 'required' field.
	}
	// Marshal and ignore error (should panic on invalid static schema).
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// createTaskInputSchema defines the schema for the "createTask" tool.
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
				"description": "Optional. The name or ID of the list to add the task to. Defaults to Inbox if not specified.",
			},
		},
		"required": []string{"name"}, // Task name is required.
	}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// completeTaskInputSchema defines the schema for the "completeTask" tool.
func (s *Service) completeTaskInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"taskId": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the task to mark as complete (format: seriesID_taskID).",
			},
			"listId": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the list containing the task.",
			},
		},
		"required": []string{"taskId", "listId"}, // Both IDs are required by the RTM API.
	}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// authenticationInputSchema defines the schema for the "authenticate" tool.
func (s *Service) authenticationInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"frob": map[string]interface{}{
				"type":        "string",
				"description": "Optional. The 'frob' code obtained from the RTM authentication URL after user authorization. Providing this completes the authentication flow.",
			},
			// AutoComplete removed as it's not currently used in the handler logic.
			// "autoComplete": map[string]interface{}{
			// 	"type":        "boolean",
			// 	"description": "Optional (defaults to false). If true and 'frob' is omitted, attempt to automatically handle the browser-based flow (requires user interaction).",
			// 	"default":     false,
			// },
		},
		// No fields are strictly required; behavior depends on presence of 'frob'.
	}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

// emptyInputSchema defines a schema for tools that take no arguments.
func (s *Service) emptyInputSchema() json.RawMessage {
	// An empty object schema signifies no parameters are expected.
	schema := map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}
