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
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
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

	var resources []mcp.ResourceDefinition

	// Check authentication
	if !s.rtmService.IsAuthenticated() {
		// Return authentication resource only
		resources = []mcp.ResourceDefinition{
			{
				Name:        "auth://rtm",
				Description: "Authentication for Remember The Milk",
				Arguments:   []mcp.ResourceArgument{},
			},
		}
	} else {
		// Return all available resources
		resources = []mcp.ResourceDefinition{
			{
				Name:        "tasks://all",
				Description: "All tasks across all lists",
				Arguments:   []mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://today",
				Description: "Tasks due today",
				Arguments:   []mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://tomorrow",
				Description: "Tasks due tomorrow",
				Arguments:   []mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://week",
				Description: "Tasks due within the next 7 days",
				Arguments:   []mcp.ResourceArgument{},
			},
			{
				Name:        "tasks://list/{list_id}",
				Description: "Tasks within a specific list",
				Arguments: []mcp.ResourceArgument{
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
				Arguments:   []mcp.ResourceArgument{},
			},
			{
				Name:        "tags://all",
				Description: "All tags used in the system",
				Arguments:   []mcp.ResourceArgument{},
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
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk. Please access auth://rtm resource first.")
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
		writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Resource not found: %s", name))
		return
	}

	// Log fetch timing
	duration := time.Since(startTime)
	log.Printf("Resource %s fetched in %v", name, duration)

	if err != nil {
		log.Printf("Error handling resource %s: %v", name, err)
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error handling resource: %v", err))
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

	var tools []mcp.ToolDefinition

	// Check authentication
	if !s.rtmService.IsAuthenticated() {
		// Return authentication tool only
		tools = []mcp.ToolDefinition{
			{
				Name:        "authenticate",
				Description: "Complete authentication with Remember The Milk",
				Arguments: []mcp.ToolArgument{
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
func getAuthenticatedTools() []mcp.ToolDefinition {
	return []mcp.ToolDefinition{
		{
			Name:        "add_task",
			Description: "Add a new task",
			Arguments: []mcp.ToolArgument{
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
			Arguments: []mcp.ToolArgument{
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
			Arguments: []mcp.ToolArgument{
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
			Arguments: []mcp.ToolArgument{
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
			Arguments: []mcp.ToolArgument{
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
			Arguments: []mcp.ToolArgument{
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
			Arguments: []mcp.ToolArgument{
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
			Arguments:   []mcp.ToolArgument{},
		},
		{
			Name:        "logout",
			Description: "Log out from Remember The Milk",
			Arguments: []mcp.ToolArgument{
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
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
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
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk. Please authenticate first.")
		return
	}

	// Handle the appropriate tool
	result, err := s.dispatchToolRequest(req.Name, req.Arguments)

	// Log tool execution timing
	duration := time.Since(startTime)
	log.Printf("Tool %s executed in %v", req.Name, duration)

	if err != nil {
		log.Printf("Error handling tool %s: %v", req.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error handling tool: %v", err))
		return
	}

	writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
		Result: result,
	})
}

// dispatchToolRequest routes the tool request to the appropriate handler.
// This is extracted from handleCallTool to reduce complexity.
func (s *MCPServer) dispatchToolRequest(toolName string, args map[string]interface{}) (string, error) {
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
