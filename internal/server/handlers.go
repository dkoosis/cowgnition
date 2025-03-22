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

// internal/server/handlers.go - focusing on the initialization part

// handleInitialize handles the MCP initialize request.
// It returns server information and capabilities according to MCP specifications.
func (s *MCPServer) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed. Initialize endpoint requires POST.")
		return
	}

	// Parse request
	var req mcp.InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to decode initialize request: %v", err))
		return
	}

	// Log initialization request with client info
	log.Printf("MCP initialization requested by: %s (version: %s)",
		req.ServerName, req.ServerVersion)

	// Construct server information
	serverInfo := mcp.ServerInfo{
		Name:    s.config.Server.Name,
		Version: s.version,
	}

	// Define capabilities based on authentication state
	resourcesCapability := map[string]interface{}{
		"list":        true,
		"read":        true,
		"subscribe":   false,
		"listChanged": true,
	}

	toolsCapability := map[string]interface{}{
		"list":        true,
		"call":        true,
		"listChanged": true,
	}

	loggingCapability := map[string]interface{}{
		"log":     true,
		"warning": true,
		"error":   true,
	}

	// Comprehensive capabilities map following MCP protocol specification
	capabilities := map[string]interface{}{
		"resources": resourcesCapability,
		"tools":     toolsCapability,
		"logging":   loggingCapability,
		"prompts": map[string]interface{}{
			"list":        false, // We don't currently support prompts
			"get":         false,
			"listChanged": false,
		},
		"roots": map[string]interface{}{
			"set": false, // We don't currently support roots management
		},
		"completion": false, // We don't support completion
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
		writeStandardErrorResponse(w, MethodNotFound,
			"Method not allowed. Read resource endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Get resource name from query parameters
	name := r.URL.Query().Get("name")
	if name == "" {
		writeStandardErrorResponse(w, InvalidParams,
			"Missing required resource name parameter.",
			map[string]string{"required_parameter": "name"})
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
		writeStandardErrorResponse(w, AuthError,
			"Not authenticated with Remember The Milk. Please authenticate via the auth://rtm resource first.",
			map[string]string{"auth_resource": "auth://rtm"})
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
		validResources := []string{"lists://all", "tasks://today", "tasks://tomorrow", "tasks://week", "tasks://list/{list_id}", "tags://all"}
		writeStandardErrorResponse(w, ResourceError,
			fmt.Sprintf("Resource not found: %s", name),
			map[string]interface{}{
				"resource_uri":    name,
				"valid_resources": validResources,
			})
		return
	}

	// Log fetch timing
	duration := time.Since(startTime)
	log.Printf("Resource %s fetched in %v", name, duration)

	if err != nil {
		writeStandardErrorResponse(w, RTMServiceError,
			fmt.Sprintf("Failed to handle resource %s", name),
			map[string]interface{}{
				"resource_uri":  name,
				"error_details": err.Error(),
				"timing":        duration.String(),
			})
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

// handleHealthCheck provides a simple health check endpoint.
func (s *MCPServer) handleHealthCheck(w http.ResponseWriter, _ *http.Request) {
	// Check if RTM service is healthy
	if s.rtmService == nil {
		// SUGGESTION (Readability): Improved health check error message.
		http.Error(w, "handleHealthCheck: RTM service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Return simple health check response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Check the Write error to satisfy linter
	if _, err := w.Write([]byte(`{"status":"healthy"}`)); err != nil {
		log.Printf("Error writing health check response: %v", err)
	}
}

// handleStatusCheck provides detailed status information for monitoring.
func (s *MCPServer) handleStatusCheck(w http.ResponseWriter, r *http.Request) {
	// Only allow access from localhost or if a special header is present
	clientIP := r.RemoteAddr
	if !strings.HasPrefix(clientIP, "127.0.0.1") && !strings.HasPrefix(clientIP, "[::1]") &&
		r.Header.Get("X-Status-Secret") != s.config.Server.StatusSecret {
		// SUGGESTION (Readability): Improved status check error message.
		http.Error(w, "handleStatusCheck: Forbidden", http.StatusForbidden)
		return
	}

	// Gather status information
	status := map[string]interface{}{
		"server": map[string]interface{}{
			"name":        s.config.Server.Name,
			"version":     s.version,
			"uptime":      s.GetUptime().String(),
			"started_at":  s.startTime.Format(time.RFC3339),
			"instance_id": s.instanceID,
		},
		"auth": map[string]interface{}{
			"status":        s.rtmService.GetAuthStatus(),
			"authenticated": s.rtmService.IsAuthenticated(),
			"pending_flows": s.rtmService.GetActiveAuthFlows(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("Error encoding status response: %v", err)
		// SUGGESTION (Readability): Improved encoding error message.
		http.Error(w, "handleStatusCheck: Error encoding JSON response", http.StatusInternalServerError)
	}
}

// handleSendNotification is a placeholder for notification support.
// The MCP spec may evolve to include proper notification support.
func (s *MCPServer) handleSendNotification(w http.ResponseWriter, r *http.Request) {
	// Currently, we don't support notifications, so return appropriate error
	if r.Method != http.MethodPost {
		// SUGGESTION (Readability): Clarified method not allowed message.
		writeErrorResponse(w, http.StatusMethodNotAllowed, "handleSendNotification: Method not allowed. Must use POST for this endpoint.")
		return
	}

	// Return unsupported operation for now
	writeErrorResponse(w, http.StatusNotImplemented, "Notifications not yet supported")
}

// handleTagsResource retrieves and formats all tags from RTM.
func (s *MCPServer) handleTagsResource() (string, error) {
	tags, err := s.rtmService.GetTags()
	if err != nil {
		return "", fmt.Errorf("rtmService.GetTags: %w", err)
	}
	return formatTags(tags), nil
}
