// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/mcp_tools.go

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings" // Ensure strings is imported.

	"github.com/dkoosis/cowgnition/internal/mcp"
)

// GetTools returns the MCP tools provided by this service.
func (s *Service) GetTools() []mcp.Tool {
	// Tool definitions using helper methods for schemas.
	return []mcp.Tool{
		{
			Name:        "getTasks",
			Description: "Retrieves tasks from Remember The Milk based on a specified filter.",
			InputSchema: s.getTasksInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Get RTM Tasks", ReadOnlyHint: true},
		},
		{
			Name:        "createTask",
			Description: "Creates a new task in Remember The Milk.",
			InputSchema: s.createTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Create RTM Task"}, // Default hints are appropriate.
		},
		{
			Name:        "completeTask",
			Description: "Marks a task as complete in Remember The Milk.",
			InputSchema: s.completeTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Complete RTM Task", DestructiveHint: true, IdempotentHint: true},
		},
		{
			Name:        "getAuthStatus",
			Description: "Gets the authentication status with Remember The Milk.",
			InputSchema: s.emptyInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Check RTM Auth Status", ReadOnlyHint: true},
		},
		{
			Name:        "authenticate",
			Description: "Initiates or completes the authentication flow with Remember The Milk.",
			InputSchema: s.authenticationInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Authenticate with RTM"},
		},
		{
			Name:        "clearAuth",
			Description: "Clears the current Remember The Milk authentication.",
			InputSchema: s.emptyInputSchema(),
			Annotations: &mcp.ToolAnnotations{Title: "Clear RTM Authentication", DestructiveHint: true, IdempotentHint: true},
		},
	}
}

// CallTool routes MCP tool calls to the appropriate handler.
func (s *Service) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.initialized {
		return s.serviceNotInitializedError(), nil // Use helper.
	}

	var handlerFunc func(context.Context, json.RawMessage) (*mcp.CallToolResult, error)

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
		handlerFunc = s.handleClearAuth // Removed ctx unused param here.
	default:
		return s.unknownToolError(name), nil // Use helper.
	}

	result, err := handlerFunc(ctx, args)
	if err != nil {
		// This indicates an *internal* error within the handler itself, not a tool execution error.
		s.logger.Error("Internal error executing RTM tool handler.", "toolName", name, "error", err)
		return s.internalToolError(), nil // Use helper.
	}
	return result, nil
}

// --- Tool Handlers ---

func (s *Service) handleGetTasks(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil // Use helper.
	}
	var params struct {
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("getTasks", err), nil // Use helper.
	}

	tasks, err := s.client.GetTasks(ctx, params.Filter) // Assumes client.GetTasks is correct.
	if err != nil {
		return s.rtmAPIErrorResult("getting tasks", err), nil // Use helper.
	}

	// Format response.
	var responseText string
	if len(tasks) == 0 {
		responseText = fmt.Sprintf("No tasks found matching filter: '%s'.", params.Filter)
	} else {
		responseText = fmt.Sprintf("Found %d tasks matching filter: '%s'.\n\n", len(tasks), params.Filter)
		maxTasksToShow := 15
		for i, task := range tasks {
			if i >= maxTasksToShow {
				responseText += fmt.Sprintf("...and %d more.\n", len(tasks)-maxTasksToShow)
				break
			}
			responseText += fmt.Sprintf("%d. %s", i+1, task.Name)
			if !task.DueDate.IsZero() {
				responseText += fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2"))
			}
			if task.Priority > 0 && task.Priority < 4 {
				responseText += fmt.Sprintf(", priority: %d", task.Priority)
			}
			if len(task.Tags) > 0 {
				responseText += fmt.Sprintf(", tags: [%s]", strings.Join(task.Tags, ", "))
			}
			responseText += ".\n"
		}
	}
	return s.successToolResult(responseText), nil // Use helper.
}

func (s *Service) handleCreateTask(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}
	var params struct {
		Name string `json:"name"`
		List string `json:"list,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("createTask", err), nil
	}

	listID := ""
	listNameToLog := "Inbox"
	if params.List != "" {
		listNameToLog = params.List
		lists, err := s.client.GetLists(ctx) // Assumes client.GetLists is correct.
		if err != nil {
			return s.rtmAPIErrorResult("getting lists to find ID", err), nil
		}
		found := false
		for _, list := range lists {
			if strings.EqualFold(list.Name, params.List) {
				listID = list.ID
				found = true
				break
			}
		}
		if !found {
			return s.simpleToolErrorResult(fmt.Sprintf("RTM list not found: %s.", params.List)), nil
		}
	}

	task, err := s.client.CreateTask(ctx, params.Name, listID) // Assumes client.CreateTask is correct.
	if err != nil {
		return s.rtmAPIErrorResult("creating task", err), nil
	}

	responseText := fmt.Sprintf("Successfully created task: '%s'.", task.Name)
	if !task.DueDate.IsZero() {
		responseText += fmt.Sprintf(" (due: %s).", task.DueDate.Format("Jan 2"))
	} else {
		responseText += "."
	}
	responseText += fmt.Sprintf("\nList: %s.\nTask ID: %s.", listNameToLog, task.ID)
	return s.successToolResult(responseText), nil
}

func (s *Service) handleCompleteTask(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil
	}
	var params struct {
		TaskID string `json:"taskId"`
		ListID string `json:"listId"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("completeTask", err), nil
	}
	if params.TaskID == "" || params.ListID == "" {
		return s.simpleToolErrorResult("Both taskId and listId are required."), nil
	}

	err := s.client.CompleteTask(ctx, params.ListID, params.TaskID) // Assumes client.CompleteTask is correct.
	if err != nil {
		return s.rtmAPIErrorResult("completing task", err), nil
	}

	return s.successToolResult(fmt.Sprintf("Successfully completed task with ID: %s.", params.TaskID)), nil
}

func (s *Service) handleGetAuthStatus(ctx context.Context, _ json.RawMessage) (*mcp.CallToolResult, error) { // Use _ for unused args.
	authState, err := s.GetAuthState(ctx) // Use service method to get cached/refreshed state.
	if err != nil {
		return s.rtmAPIErrorResult("getting auth status", err), nil
	}

	var responseText string
	if authState.IsAuthenticated {
		responseText = fmt.Sprintf("Authenticated with Remember The Milk as user: %s", authState.Username)
		if authState.FullName != "" {
			responseText += fmt.Sprintf(" (%s)", authState.FullName)
		}
		responseText += "."
	} else {
		// Get auth URL, but handle potential error from StartAuth.
		authURL, startAuthErr := s.StartAuth(ctx) // Use service method.
		if startAuthErr != nil {
			responseText = fmt.Sprintf("Not authenticated. Failed to generate RTM auth URL: %v.", startAuthErr)
		} else {
			// Extract frob from URL for user convenience (assuming StartAuth returns URL + frob).
			frobParam := ""
			parsedURL, _ := url.Parse(authURL) // Ignore error, simple split as fallback.
			if parsedURL != nil {
				frobParam = parsedURL.Query().Get("frob") // More robust way if StartAuth adds it to query.
			}
			if frobParam == "" && strings.Contains(authURL, "&frob=") { // Fallback split.
				if parts := strings.Split(authURL, "&frob="); len(parts) > 1 {
					frobParam = parts[1]
				}
			}

			responseText = "Not authenticated with Remember The Milk.\n\n"
			responseText += "To authenticate, please visit this URL:\n" + authURL + "\n\n"
			responseText += "Then use the 'authenticate' tool with the 'frob' code from the URL.\n"
			if frobParam != "" {
				responseText += fmt.Sprintf("Example: authenticate(frob: \"%s\")", frobParam)
			}
		}
	}
	return s.successToolResult(responseText), nil
}

func (s *Service) handleAuthenticate(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		Frob string `json:"frob,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("authenticate", err), nil
	}

	if params.Frob != "" {
		// Complete Auth Flow.
		err := s.CompleteAuth(ctx, params.Frob) // Use service method.
		if err != nil {
			return s.rtmAPIErrorResult("completing authentication", err), nil
		}
		if !s.IsAuthenticated() {
			return s.simpleToolErrorResult("Authentication completed, but verification failed. Try 'getAuthStatus'."), nil
		}
		return s.successToolResult(fmt.Sprintf("Successfully authenticated as user: %s.", s.GetUsername())), nil
	}

	// Start Auth Flow (No 'else' needed as the 'if' block returns).
	authURL, startAuthErr := s.StartAuth(ctx) // Use service method.
	if startAuthErr != nil {
		return s.rtmAPIErrorResult("starting authentication", startAuthErr), nil
	}

	frobParam := ""
	parsedURL, _ := url.Parse(authURL)
	if parsedURL != nil {
		frobParam = parsedURL.Query().Get("frob")
	}
	if frobParam == "" && strings.Contains(authURL, "&frob=") { // Fallback split.
		if parts := strings.Split(authURL, "&frob="); len(parts) > 1 {
			frobParam = parts[1]
		}
	}

	responseText := "To authenticate:\n1. Visit URL: " + authURL + "\n2. Authorize CowGnition."
	responseText += "\n3. Use authenticate tool with the 'frob' code from the URL."
	if frobParam != "" {
		responseText += fmt.Sprintf("\n   Example: authenticate(frob: \"%s\")", frobParam)
	}

	return s.successToolResult(responseText), nil
}

// handleClearAuth handles the clearAuth tool call.
func (s *Service) handleClearAuth(_ context.Context, _ json.RawMessage) (*mcp.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.successToolResult("Not currently authenticated."), nil // Not an error.
	}
	username := s.GetUsername()
	err := s.ClearAuth() // Use service method.
	if err != nil {
		return s.rtmAPIErrorResult("clearing authentication", err), nil
	}
	return s.successToolResult(fmt.Sprintf("Successfully cleared RTM authentication for user: %s.", username)), nil
}

// --- Input Schema Definitions ---
// (Moved to separate functions for clarity).

func (s *Service) getTasksInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "RTM filter expression (e.g., 'list:Inbox status:incomplete dueBefore:tomorrow'). See RTM documentation for filter syntax.",
			},
		},
		// Make filter optional for getTasks.
		// "required": []string{"filter"},
	}
	schemaJSON, _ := json.Marshal(schema) // Handle potential panic if needed.
	return schemaJSON
}

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
		"required": []string{"name"},
	}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

func (s *Service) completeTaskInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"taskId": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the task to mark as complete (format: seriesID_taskID).",
			},
			"listId": map[string]interface{}{ // Added listId based on client implementation.
				"type":        "string",
				"description": "The ID of the list containing the task.",
			},
		},
		"required": []string{"taskId", "listId"}, // Made listId required.
	}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

func (s *Service) authenticationInputSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"frob": map[string]interface{}{
				"type":        "string",
				"description": "Optional. The 'frob' code obtained from the RTM authentication URL. If provided, completes the auth flow. If omitted, starts the auth flow.",
			},
		},
		// Frob is optional.
		// "required": []string{},
	}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}

func (s *Service) emptyInputSchema() json.RawMessage {
	schema := map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	schemaJSON, _ := json.Marshal(schema)
	return schemaJSON
}
