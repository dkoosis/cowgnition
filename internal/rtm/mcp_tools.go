// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// file: internal/rtm/mcp_tools.go
// This file contains the internal handler functions (handleGetTasks, handleCreateTask, etc.).
// called by the service's CallTool method.
package rtm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	// Keep time import for date formatting.
	// --- FIX: Ensure correct import ---.
	// Use mcptypes alias.
)

// --- Tool Handler Implementations ---.
// These functions are called internally by the rtm.Service.CallTool method.

// handleGetTasks implements the logic for the "getTasks" MCP tool.
// --- FIX: Use mcptypes prefix ---.
func (s *Service) handleGetTasks(ctx context.Context, args json.RawMessage) (*mcptypes.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil // Calls helper in helpers.go.
	}
	var params struct {
		Filter string `json:"filter,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("getTasks", err), nil // Calls helper in helpers.go.
	}
	s.logger.Info("Executing RTM getTasks tool.", "filter", params.Filter)
	tasks, err := s.client.GetTasks(ctx, params.Filter)
	if err != nil {
		return s.rtmAPIErrorResult("getting tasks", err), nil // Calls helper in helpers.go.
	}
	var responseText strings.Builder
	filterDisplay := params.Filter
	if filterDisplay == "" {
		filterDisplay = "(no filter)"
	}
	if len(tasks) == 0 {
		responseText.WriteString(fmt.Sprintf("No tasks found matching filter: '%s'.", filterDisplay))
	} else {
		responseText.WriteString(fmt.Sprintf("Found %d tasks matching filter: '%s'.\n\n", len(tasks), filterDisplay))
		maxTasksToShow := 15 // Example limit.
		for i, task := range tasks {
			if i >= maxTasksToShow {
				responseText.WriteString(fmt.Sprintf("...and %d more.\n", len(tasks)-maxTasksToShow))
				break
			}
			responseText.WriteString(fmt.Sprintf("%d. %s", i+1, task.Name))
			if !task.DueDate.IsZero() {
				responseText.WriteString(fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2")))
			}
			if task.Priority > 0 && task.Priority < 4 {
				responseText.WriteString(fmt.Sprintf(", P%d", task.Priority))
			}
			if len(task.Tags) > 0 {
				responseText.WriteString(fmt.Sprintf(", tags:[%s]", strings.Join(task.Tags, ",")))
			}
			responseText.WriteString(".\n")
		}
	}
	return s.successToolResult(responseText.String()), nil // Calls helper in helpers.go.
}

// handleCreateTask implements the logic for the "createTask" tool.
// --- FIX: Use mcptypes prefix ---.
func (s *Service) handleCreateTask(ctx context.Context, args json.RawMessage) (*mcptypes.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil // Calls helper in helpers.go.
	}
	var params struct {
		Name string `json:"name"`
		List string `json:"list,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("createTask", err), nil // Calls helper in helpers.go.
	}
	s.logger.Info("Executing RTM createTask tool.", "name", params.Name, "list", params.List)
	listID := ""
	listNameToLog := "Inbox"
	if params.List != "" {
		listNameToLog = params.List
		lists, err := s.client.GetLists(ctx)
		if err != nil {
			return s.rtmAPIErrorResult("getting lists to find ID for task creation", err), nil // Calls helper in helpers.go.
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
			return s.simpleToolErrorResult(fmt.Sprintf("RTM list not found: '%s'.", params.List)), nil // Calls helper in helpers.go.
		}
	}
	task, err := s.client.CreateTask(ctx, params.Name, listID)
	if err != nil {
		return s.rtmAPIErrorResult("creating task", err), nil // Calls helper in helpers.go.
	}
	responseText := fmt.Sprintf("Successfully created task: '%s'", task.Name)
	if !task.DueDate.IsZero() {
		responseText += fmt.Sprintf(" (due: %s)", task.DueDate.Format("Jan 2"))
	}
	responseText += fmt.Sprintf(" in list '%s'.", listNameToLog)
	responseText += fmt.Sprintf("\nTask ID: %s.", task.ID)
	return s.successToolResult(responseText), nil // Calls helper in helpers.go.
}

// handleCompleteTask implements the logic for the "completeTask" tool.
// --- FIX: Use mcptypes prefix ---.
func (s *Service) handleCompleteTask(ctx context.Context, args json.RawMessage) (*mcptypes.CallToolResult, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedError(), nil // Calls helper in helpers.go.
	}
	var params struct {
		TaskID string `json:"taskId"`
		ListID string `json:"listId"` // RTM API requires listId for completion.
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("completeTask", err), nil // Calls helper in helpers.go.
	}
	// --- FIX: Validate required params ---.
	if params.TaskID == "" || params.ListID == "" {
		return s.simpleToolErrorResult("Both taskId and listId are required to complete a task."), nil // Calls helper in helpers.go.
	}
	// --- END FIX ---.
	s.logger.Info("Executing RTM completeTask tool.", "taskId", params.TaskID, "listId", params.ListID)
	err := s.client.CompleteTask(ctx, params.ListID, params.TaskID)
	if err != nil {
		return s.rtmAPIErrorResult("completing task", err), nil // Calls helper in helpers.go.
	}
	return s.successToolResult(fmt.Sprintf("Successfully completed task with ID: %s.", params.TaskID)), nil // Calls helper in helpers.go.
}

// handleGetAuthStatus implements the logic for the "getAuthStatus" tool.
// --- FIX: Use mcptypes prefix ---.
func (s *Service) handleGetAuthStatus(ctx context.Context, _ json.RawMessage) (*mcptypes.CallToolResult, error) {
	s.logger.Info("Executing RTM getAuthStatus tool.")
	authState, err := s.GetAuthState(ctx)
	if err != nil {
		return s.rtmAPIErrorResult("getting auth status", err), nil // Calls helper in helpers.go.
	}
	var responseText string
	if authState.IsAuthenticated {
		responseText = fmt.Sprintf("Authenticated with Remember The Milk as user: %s", authState.Username)
		if authState.FullName != "" {
			responseText += fmt.Sprintf(" (%s)", authState.FullName)
		}
		responseText += "."
	} else {
		authURL, frob, startAuthErr := s.client.StartAuthFlow(ctx)
		if startAuthErr != nil {
			responseText = fmt.Sprintf("Not authenticated. Failed to generate RTM auth URL for instructions: %v.", startAuthErr)
		} else {
			responseText = "Not authenticated with Remember The Milk.\n\n"
			responseText += "To authenticate, please visit this URL:\n" + authURL + "\n\n"
			// --- FIX: Use prefixed tool name ---.
			responseText += "After authorizing in the browser, use the 'rtm_authenticate' tool with the 'frob' code found in the URL's query parameters.\n"
			if frob != "" {
				responseText += fmt.Sprintf("Example: rtm_authenticate(frob: \"%s\")", frob)
			}
			// --- END FIX ---.
		}
	}
	return s.successToolResult(responseText), nil // Calls helper in helpers.go.
}

// handleAuthenticate implements the logic for the "authenticate" tool.
// --- FIX: Use mcptypes prefix ---.
func (s *Service) handleAuthenticate(ctx context.Context, args json.RawMessage) (*mcptypes.CallToolResult, error) {
	var params struct {
		Frob string `json:"frob,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return s.invalidToolArgumentsError("authenticate", err), nil // Calls helper in helpers.go.
	}
	if params.Frob != "" {
		s.logger.Info("Attempting to complete RTM authentication with provided frob.", "tool", "authenticate")
		err := s.CompleteAuth(ctx, params.Frob) // Calls internal service method.
		if err != nil {
			return s.rtmAPIErrorResult("completing authentication", err), nil // Calls helper in helpers.go.
		}
		if !s.IsAuthenticated() {
			return s.simpleToolErrorResult("Authentication failed. The provided 'frob' might be invalid or expired. Please try starting the authentication process again."), nil // Calls helper in helpers.go.
		}
		return s.successToolResult(fmt.Sprintf("Successfully authenticated as user: %s.", s.GetUsername())), nil // Calls helper in helpers.go.
	}
	s.logger.Info("Starting new RTM authentication flow.", "tool", "authenticate")
	authURL, frob, err := s.client.StartAuthFlow(ctx)
	if err != nil {
		return s.rtmAPIErrorResult("starting authentication", err), nil // Calls helper in helpers.go.
	}
	responseText := "To authenticate with Remember The Milk:\n\n"
	responseText += "1. Visit this URL in your browser: " + authURL + "\n\n"
	responseText += "2. Click 'Allow Access' or 'OK, I'll Allow It' to grant permission.\n\n"
	// --- FIX: Use prefixed tool name ---.
	responseText += "3. After authorizing, complete the authentication by calling this tool again with the 'frob' parameter found in the URL you visited.\n"
	responseText += fmt.Sprintf("   Example: rtm_authenticate(frob: \"%s\")\n", frob)
	// --- END FIX ---.
	return s.successToolResult(responseText), nil // Calls helper in helpers.go.
}

// handleClearAuth implements the logic for the "clearAuth" tool.
// --- FIX: Use mcptypes prefix ---.
func (s *Service) handleClearAuth(_ context.Context, _ json.RawMessage) (*mcptypes.CallToolResult, error) {
	s.logger.Info("Executing RTM clearAuth tool.")
	if !s.IsAuthenticated() {
		return s.successToolResult("Not currently authenticated with RTM."), nil // Calls helper in helpers.go.
	}
	username := s.GetUsername()
	err := s.ClearAuth() // Calls internal service method.
	if err != nil {
		return s.rtmAPIErrorResult("clearing authentication", err), nil // Calls helper in helpers.go.
	}
	return s.successToolResult(fmt.Sprintf("Successfully cleared RTM authentication for user: %s.", username)), nil // Calls helper in helpers.go.
}
