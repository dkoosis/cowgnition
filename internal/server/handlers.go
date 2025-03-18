// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cowgnition/cowgnition/pkg/mcp"
)

// handleInitialize handles the MCP initialize request.
// It returns server information and capabilities.
func (s *MCPServer) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse request
	var req mcp.InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// SUGGESTION (Ambiguous): Improve error message to indicate decoding failure
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("handleInitialize: failed to decode request body: %v", err))
		return
	}

	// Log initialization attempt
	log.Printf("MCP initialization requested by: %s (version: %s)",
		req.ServerName, req.ServerVersion)

	// Construct server information
	serverInfo := mcp.ServerInfo{
		Name:    s.config.Server.Name,
		Version: s.version,
	}

	// Define capabilities
	capabilities := map[string]interface{}{
		"resources": map[string]interface{}{
			"list": true,
			"read": true,
			// We don't support resource subscriptions yet
			"subscribe":   false,
			"listChanged": false,
		},
		"tools": map[string]interface{}{
			"list":        true,
			"call":        true,
			"listChanged": false,
		},
		"logging": map[string]interface{}{
			"log":     true,
			"warning": true,
			"error":   true,
		},
		// We don't support prompts yet
		"prompts": map[string]interface{}{
			"list":        false,
			"get":         false,
			"listChanged": false,
		},
		// We don't support completion yet
		"completion": false,
	}

	// Construct response
	response := mcp.InitializeResponse{
		ServerInfo:   serverInfo,
		Capabilities: capabilities,
	}

	// Log successful initialization
	log.Printf("MCP initialization successful for: %s", req.ServerName)

	writeJSONResponse(w, http.StatusOK, response)
}

// handleListResources handles the MCP list_resources request.
// It returns a list of available resources based on authentication status.
func (s *MCPServer) handleListResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var resources mcp.ResourceDefinition

	// Check authentication
	if !s.rtmService.IsAuthenticated() {
		// Return authentication resource only
		resources := mcp.ResourceDefinition{
			{
				Name:        "auth://rtm",
				Description: "Authentication for Remember The Milk",
				Arguments:   mcp.ResourceArgument{},
			},
		}
	} else {
		// Return all available resources
		resources =mcp.ResourceDefinition{
			{
				Name:        "tasks://all",
				Description: "All tasks across all lists",
				Arguments:  mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://today",
				Description: "Tasks due today",
				Arguments:  mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://tomorrow",
				Description: "Tasks due tomorrow",
				Arguments:  mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://week",
				Description: "Tasks due within the next 7 days",
				Arguments:  mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://list/{list_id}",
				Description: "Tasks within a specific list",
				Arguments:mcp.ResourceArgument{
					{
						Name:        "list_id",
						Description: "The ID of the list",
						Required:    true,
					},
				},
			},
			{
				Name:        "lists://all",
				Description: "All task lists",
				Arguments:  mcp.ResourceArgument{},
			},
			{
				Name:        "tags://all",
				Description: "All tags used in the system",
				Arguments:  mcp.ResourceArgument{},
			},
		}
	}

	response := mcp.ListResourcesResponse{
		Resources: resources,
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// handleReadResource handles the MCP read_resource request.
// It fetches and returns the content of the requested resource.
func (s *MCPServer) handleReadResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get resource name from query parameters
	name := r.URL.Query().Get("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Missing resource name parameter")
		return
	}

	log.Printf("Resource request: %s", name)

	// Handle authentication resource
	if name == "auth://rtm" {
		s.handleAuthResource(w)
		return
	}

	// Check authentication for other resources
	if !s.rtmService.IsAuthenticated() {
		// SUGGESTION (Readability): Clarify authentication error message
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk. Please authenticate via the auth://rtm resource first.")
		return
	}

	// Handle different resource types
	var content string
	var err error
	mimeType := "text/plain"

	// Track resource fetch timing
	startTime := time.Now()

	switch {
	case name == "lists://all":
		content, err = s.handleListsResource()
	case name == "tasks://all":
		content, err = s.handleTasksResource("")
	case name == "tasks://today":
		content, err = s.handleTasksResource("due:today")
	case name == "tasks://tomorrow":
		content, err = s.handleTasksResource("due:tomorrow")
	case name == "tasks://week":
		content, err = s.handleTasksResource("due:\"within 7 days\"")
	case strings.HasPrefix(name, "tasks://list/"):
		// Extract list ID from resource name
		listID := strings.TrimPrefix(name, "tasks://list/")
		content, err = s.handleTasksResource("list:" + listID)
	case name == "tags://all":
		content, err = s.handleTagsResource()
	default:
		// SUGGESTION (Ambiguous): Improve error message to include valid resource examples
		writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Resource not found: %s. Valid resource examples: lists://all, tasks://today, etc.", name))
		return
	}

	// Log fetch timing
	duration := time.Since(startTime)
	log.Printf("Resource %s fetched in %v", name, duration)

	if err != nil {
		// SUGGESTION (Ambiguous): Add context to error message
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("handleReadResource: failed to handle resource %s: %v", name, err))
		return
	}

	response := mcp.ResourceResponse{
		Content:  content,
		MimeType: mimeType,
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// handleListTools handles the MCP list_tools request.
// It returns a list of available tools based on authentication status.
func (s *MCPServer) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var toolsmcp.ToolDefinition

	// Check authentication
	if !s.rtmService.IsAuthenticated() {
		// Return authentication tool only
		tools =mcp.ToolDefinition{
			{
				Name:        "authenticate",
				Description: "Complete authentication with Remember The Milk",
				Arguments:mcp.ToolArgument{
					{
						Name:        "frob",
						Description: "The frob obtained after authorizing the application",
						Required:    true,
					},
				},
			},
		}
	} else {
		// Return all available tools
		tools = getAuthenticatedTools()
	}

	response := mcp.ListToolsResponse{
		Tools: tools,
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// getAuthenticatedTools returns the list of available tools when authenticated.
// This is extracted from handleListTools to reduce complexity.
func getAuthenticatedTools()mcp.ToolDefinition {
	returnmcp.ToolDefinition{
		{
			Name:        "add_task",
			Description: "Add a new task",
			Arguments:mcp.ToolArgument{
				{
					Name:        "name",
					Description: "The name of the task",
					Required:    true,
				},
				{
					Name:        "list_id",
					Description: "The ID of the list to add the task to",
					Required:    false,
				},
				{
					Name:        "due_date",
					Description: "The due date of the task (e.g. 'today', 'tomorrow', '2023-12-31')",
					Required:    false,
				},
			},
		},
		{
			Name:        "complete_task",
			Description: "Mark a task as completed",
			Arguments:mcp.ToolArgument{
				{
					Name:        "list_id",
					Description: "The ID of the list containing the task",
					Required:    true,
				},
				{
					Name:        "taskseries_id",
					Description: "The ID of the task series",
					Required:    true,
				},
				{
					Name:        "task_id",
					Description: "The ID of the task",
					Required:    true,
				},
			},
		},
		{
			Name:        "uncomplete_task",
			Description: "Mark a completed task as incomplete",
			Arguments:mcp.ToolArgument{
				{
					Name:        "list_id",
					Description: "The ID of the list containing the task",
					Required:    true,
				},
				{
					Name:        "taskseries_id",
					Description: "The ID of the task series",
					Required:    true,
				},
				{
					Name:        "task_id",
					Description: "The ID of the task",
					Required:    true,
				},
			},
		},
		{
			Name:        "delete_task",
			Description: "Delete a task",
			Arguments:mcp.ToolArgument{
				{
					Name:        "list_id",
					Description: "The ID of the list containing the task",
					Required:    true,
				},
				{
					Name:        "taskseries_id",
					Description: "The ID of the task series",
					Required:    true,
				},
				{
					Name:        "task_id",
					Description: "The ID of the task",
					Required:    true,
				},
			},
		},
		{
			Name:        "set_due_date",
			Description: "Set or update a task's due date",
			Arguments:mcp.ToolArgument{
				{
					Name:        "list_id",
					Description: "The ID of the list containing the task",
					Required:    true,
				},
				{
					Name:        "taskseries_id",
					Description: "The ID of the task series",
					Required:    true,
				},
				{
					Name:        "task_id",
					Description: "The ID of the task",
					Required:    true,
				},
				{
					Name:        "due_date",
					Description: "The due date (leave empty to clear)",
					Required:    false,
				},
				{
					Name:        "has_due_time",
					Description: "Whether the due date includes a time component",
					Required:    false,
				},
			},
		},
		{
			Name:        "set_priority",
			Description: "Set a task's priority",
			Arguments:mcp.ToolArgument{
				{
					Name:        "list_id",
					Description: "The ID of the list containing the task",
					Required:    true,
				},
				{
					Name:        "taskseries_id",
					Description: "The ID of the task series",
					Required:    true,
				},
				{
					Name:        "task_id",
					Description: "The ID of the task",
					Required:    true,
				},
				{
					Name:        "priority",
					Description: "The priority (1=high, 2=medium, 3=low, 0=none)",
					Required:    true,
				},
			},
		},
		{
			Name:        "add_tags",
			Description: "Add tags to a task",
			Arguments:mcp.ToolArgument{
				{
					Name:        "list_id",
					Description: "The ID of the list containing the task",
					Required:    true,
				},
				{
					Name:        "taskseries_id",
					Description: "The ID of the task series",
					Required:    true,
				},
				{
					Name:        "task_id",
					Description: "The ID of the task",
					Required:    true,
				},
				{
					Name:        "tags",
					Description: "The tags to add (string or array of strings)",
					Required:    true,
				},
			},
		},
		{
			Name:        "auth_status",
			Description: "Check authentication status with Remember The Milk",
			Arguments:  mcp.ToolArgument{},
		},
		{
			Name:        "logout",
			Description: "Log out from Remember The Milk",
			Arguments:mcp.ToolArgument{
				{
					Name:        "confirm",
					Description: "Confirm logout to prevent accidental disconnection",
					Required:    true,
				},
			},
		},
	}
}

// handleCallTool handles the MCP call_tool request.
// It executes the requested tool and returns the result.
func (s *MCPServer) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse request
	var req mcp.CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// SUGGESTION (Ambiguous): Improve error message to indicate decoding failure
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("handleCallTool: failed to decode request body: %v", err))
		return
	}

	log.Printf("Tool call request: %s", req.Name)
	startTime := time.Now()

	// Handle authentication tool
	if req.Name == "authenticate" {
		s.handleAuthenticationTool(w, req.Arguments)
		return
	}

	// Check authentication for other tools
	if !s.rtmService.IsAuthenticated() {
		// SUGGESTION (Readability): Clarify authentication error message
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk. Please authenticate first via the authenticate tool.")
		return
	}

	// Handle the appropriate tool
	result, err := s.dispatchToolRequest(req.Name, req.Arguments)

	// Log tool execution timing
	duration := time.Since(startTime)
	log.Printf("Tool %s executed in %v", req.Name, duration)

	if err != nil {
		// SUGGESTION (Ambiguous): Add context to error message
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("handleCallTool: failed to handle tool %s: %v", req.Name, err))
		return
	}

	writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
		Result: result,
	})
}

// dispatchToolRequest routes the tool request to the appropriate handler.
// This is extracted from handleCallTool to reduce complexity.
func (s *MCPServer) dispatchToolRequest(toolName string, args map[string]interface{}){
	switch toolName {
	case "add_task":
		return s.handleAddTaskTool(args)
	case "complete_task":
		return s.handleCompleteTaskTool(args)
	case "uncomplete_task":
		return s.handleUncompleteTaskTool(args)
	case "delete_task":
		return s.handleDeleteTaskTool(args)
	case "set_due_date":
		return s.handleSetDueDateTool(args)
	case "set_priority":
		return s.handleSetPriorityTool(args)
	case "add_tags":
		return s.handleAddTagsTool(args)
	case "logout":
		return s.handleLogoutTool(args)
	case "auth_status":
		return s.handleAuthStatusTool(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// handleAuthResource handles the auth://rtm resource.
// It redirects the user to the RTM authentication URL if not authenticated,
// or returns a success message if already authenticated.
func (s *MCPServer) handleAuthResource(w http.ResponseWriter, r *http.Request) {
	if !s.rtmService.IsAuthenticated() {
		// Generate the RTM authentication URL
		authURL, err := s.rtmService.GetAuthURL()
		if err != nil {
			log.Printf("Error generating RTM auth URL: %v", err)
			writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate RTM authentication URL")
			return
		}

		// Redirect the user to the authentication URL
		http.Redirect(w, r, authURL, http.StatusFound)
	} else {
		// Return a success message if already authenticated
		writeJSONResponse(w, http.StatusOK, mcp.ResourceResponse{
			Content:  "Already authenticated with Remember The Milk.",
			MimeType: "text/plain",
		})
	}
}

// handleAuthenticationTool handles the authenticate tool.
// It completes the RTM authentication process using the provided frob.
func (s *MCPServer) handleAuthenticationTool(w http.ResponseWriter, args map[string]interface{}) {
	frob, ok := args["frob"].(string)
	if !ok || frob == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid argument: 'frob' is required")
		return
	}

	// Complete the authentication process
	if err := s.rtmService.CompleteAuth(frob); err != nil {
		log.Printf("Error completing authentication: %v", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to complete authentication with Remember The Milk")
		return
	}

	writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
		Result: "Successfully authenticated with Remember The Milk.",
	})
}

// handleListsResource retrieves and formats all task lists from RTM.
func (s *MCPServer) handleListsResource() (string, error) {
	lists, err := s.rtmService.GetLists()
	if err != nil {
		return "", fmt.Errorf("rtmService.GetLists: %w", err)
	}
	return formatLists(lists), nil
}

// handleTasksResource retrieves and formats tasks from RTM based on the filter.
func (s *MCPServer) handleTasksResource(filter string) (string, error) {
	tasks, err := s.rtmService.GetTasks(filter)
	if err != nil {
		return "", fmt.Errorf("rtmService.GetTasks with filter '%s': %w", filter, err)
	}
	return formatTasks(tasks), nil
}

// handleTagsResource retrieves and formats all tags from RTM.
func (s *MCPServer) handleTagsResource() (string, error) {
	tags, err := s.rtmService.GetTags()
	if err != nil {
		return "", fmt.Errorf("rtmService.GetTags: %w", err)
	}
	return formatTags(tags), nil
}

// handleAddTaskTool handles the add_task tool.
// It adds a new task to RTM.
func (s *MCPServer) handleAddTaskTool(args map[string]interface{}) (string, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("invalid argument: 'name' is required")
	}

	listID, _ := args["list_id"].(string) // Optional
	dueDate, _ := args["due_date"].(string) // Optional

	if err := s.rtmService.AddTask(name, listID, dueDate); err != nil {
		return "", fmt.Errorf("rtmService.AddTask: %w", err)
	}
	return "Task added successfully.", nil
}

// handleCompleteTaskTool handles the complete_task tool.
// It marks a task as completed in RTM.
func (s *MCPServer) handleCompleteTaskTool(args map[string]interface{}) (string, error) {
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("invalid argument: 'list_id' is required")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("invalid argument: 'taskseries_id' is required")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("invalid argument: 'task_id' is required")
	}

	if err := s.rtmService.CompleteTask(listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("rtmService.CompleteTask: %w", err)
	}
	return "Task completed successfully.", nil
}

// handleUncompleteTaskTool handles the uncomplete_task tool.
// It marks a task as incomplete in RTM.
func (s *MCPServer) handleUncompleteTaskTool(args map[string]interface{}) (string, error) {
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("invalid argument: 'list_id' is required")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("invalid argument: 'taskseries_id' is required")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("invalid argument: 'task_id' is required")
	}

	if err := s.rtmService.UncompleteTask(listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("rtmService.UncompleteTask: %w", err)
	}
	return "Task marked as incomplete successfully.", nil
}

// handleDeleteTaskTool handles the delete_task tool.
// It deletes a task from RTM.
func (s *MCPServer) handleDeleteTaskTool(args map[string]interface{}) (string, error) {
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("invalid argument: 'list_id' is required")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("invalid argument: 'taskseries_id' is required")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("invalid argument: 'task_id' is required")
	}

	if err := s.rtmService.DeleteTask(listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("rtmService.DeleteTask: %w", err)
	}
	return "Task deleted successfully.", nil
}

// handleSetDueDateTool handles the set_due_date tool.
// It sets or updates a task's due date in RTM.
func (s *MCPServer) handleSetDueDateTool(args map[string]interface{}) (string, error) {
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("invalid argument: 'list_id' is required")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("invalid argument: 'taskseries_id' is required")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("invalid argument: 'task_id' is required")
	}

	dueDate, _ := args["due_date"].(string)       // Optional
	hasDueTime, _ := args["has_due_time"].(bool) // Optional

	if err := s.rtmService.SetTaskDueDate(listID, taskseriesID, taskID, dueDate, hasDueTime); err != nil {
		return "", fmt.Errorf("rtmService.SetTaskDueDate: %w", err)
	}
	return "Task due date updated successfully.", nil
}

// handleSetPriorityTool handles the set_priority tool.
// It sets a task's priority in RTM.
func (s *MCPServer) handleSetPriorityTool(args map[string]interface{}) (string, error) {
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("invalid argument: 'list_id' is required")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("invalid argument: 'taskseries_id' is required")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("invalid argument: 'task_id' is required")
	}

	priority, ok := args["priority"].(float64)
	if !ok {
		return "", fmt.Errorf("invalid argument: 'priority' is required")
	}

	if err := s.rtmService.SetTaskPriority(listID, taskseriesID, taskID, int(priority)); err != nil {
		return "", fmt.Errorf("rtmService.SetTaskPriority: %w", err)
	}
	return "Task priority updated successfully.", nil
}

// handleAddTagsTool handles the add_tags tool.
// It adds tags to a task in RTM.
func (s *MCPServer) handleAddTagsTool(args map[string]interface{}) (string, error) {
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("invalid argument: 'list_id' is required")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("invalid argument: 'taskseries_id' is required")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("invalid argument: 'task_id' is required")
	}

	tags, ok := args["tags"].(string)
	if !ok || tags == "" {
		return "", fmt.Errorf("invalid argument: 'tags' is required")
	}

	if err := s.rtmService.AddTaskTags(listID, taskseriesID, taskID, tags); err != nil {
		return "", fmt.Errorf("rtmService.AddTaskTags: %w", err)
	}
	return "Task tags added successfully.", nil
}

// handleLogoutTool handles the logout tool.
// It logs the user out from RTM.
func (s *MCPServer) handleLogoutTool(args map[string]interface{}) (string, error) {
	confirm, ok := args["confirm"].(bool)
	if !ok || !confirm {
		return "", fmt.Errorf("invalid argument: 'confirm' is required and must be true")
	}

	s.rtmService.Logout()
	return "Logged out successfully.", nil
}

// handleAuthStatusTool handles the auth_status tool.
// It checks and returns the authentication status with RTM.
func (s *MCPServer) handleAuthStatusTool(args map[string]interface{}) (string, error) {
	if s.rtmService.IsAuthenticated() {
		return "Authenticated with Remember The Milk.", nil
	}
	return "Not authenticated with Remember The Milk.", nil
}

// writeErrorResponse writes an error response to the HTTP writer.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	log.Printf("Error: %s (Status: %d)", message, statusCode)
	http.Error(w, message, statusCode)
}

// writeJSONResponse writes a JSON response to the HTTP writer.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Error encoding JSON response", http.StatusInternalServerError)
	}
}

// ErrorMsgEnhanced:2024-02-29
