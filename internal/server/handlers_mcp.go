// file: internal/server/handlers_mcp.go
// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dkoosis/cowgnition/internal/server/middleware"
	"github.com/dkoosis/cowgnition/pkg/mcp"
)

// handleMCPInitialize handles the MCP initialize request.
// It returns server information and capabilities according to MCP specifications.
func (s *Server) handleMCPInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeStandardErrorResponse(w, MethodNotFound,
			"Method not allowed. Initialize endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Parse request
	var req mcp.InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStandardErrorResponse(w, ParseError,
			fmt.Sprintf("Failed to decode initialize request: %v", err),
			map[string]string{"request_url": r.URL.Path})
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

	// Define capabilities
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

	capabilities := map[string]interface{}{
		"resources": resourcesCapability,
		"tools":     toolsCapability,
		"logging":   loggingCapability,
		"prompts": map[string]interface{}{
			"list":        false,
			"get":         false,
			"listChanged": false,
		},
		"roots": map[string]interface{}{
			"set": false,
		},
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

// handleMCPListResources handles the MCP list_resources request.
// It returns a list of available resources based on authentication status.
func (s *Server) handleMCPListResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStandardErrorResponse(w, MethodNotFound,
			"Method not allowed. List resources endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
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
		resources = getAuthenticatedResources()
	}

	response := mcp.ListResourcesResponse{
		Resources: resources,
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// getAuthenticatedResources returns the complete list of resources available when authenticated.
// Extracted from handleMCPListResources to reduce complexity.
func getAuthenticatedResources() []mcp.ResourceDefinition {
	return []mcp.ResourceDefinition{
		{
			Name:        "auth://rtm",
			Description: "Authentication for Remember The Milk",
			Arguments:   []mcp.ResourceArgument{},
		},
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

// handleMCPReadResource handles the MCP read_resource request.
// It fetches and returns the content of the requested resource.
func (s *Server) handleMCPReadResource(w http.ResponseWriter, r *http.Request) {
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
		s.handleAuthResource(w, r)
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
	mimeType := "text/markdown"

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

// handleMCPListTools handles the MCP list_tools request.
// It returns a list of available tools based on authentication status.
func (s *Server) handleMCPListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStandardErrorResponse(w, MethodNotFound,
			"Method not allowed. List tools endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
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

// handleMCPCallTool handles the MCP call_tool request.
// It executes the requested tool and returns the result.
func (s *Server) handleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeStandardErrorResponse(w, MethodNotFound,
			"Method not allowed. Call tool endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Parse request
	var req mcp.CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStandardErrorResponse(w, ParseError,
			fmt.Sprintf("Failed to decode call_tool request: %v", err),
			map[string]string{"request_path": r.URL.Path})
		return
	}

	log.Printf("Tool call request: %s", req.Name)
	startTime := time.Now()

	// Handle authentication tool
	if req.Name == "authenticate" {
		authHandler := middleware.NewAuthHandler(s)
		authHandler.HandleAuthenticationTool(w, req.Arguments)
		return
	}

	// Check authentication for other tools
	if !s.rtmService.IsAuthenticated() {
		writeStandardErrorResponse(w, AuthError,
			"Not authenticated with Remember The Milk. Please authenticate first via the authenticate tool.",
			map[string]string{"auth_tool": "authenticate"})
		return
	}

	// Handle the appropriate tool
	result, err := s.dispatchToolRequest(req.Name, req.Arguments)

	// Log tool execution timing
	duration := time.Since(startTime)
	log.Printf("Tool %s executed in %v", req.Name, duration)

	if err != nil {
		writeStandardErrorResponse(w, ToolError,
			fmt.Sprintf("Failed to execute tool %s: %v", req.Name, err),
			map[string]interface{}{
				"tool_name":    req.Name,
				"error_reason": err.Error(),
				"timing":       duration.String(),
			})
		return
	}

	writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
		Result: result,
	})
}

// handleMCPSendNotification is a placeholder for notification support.
// The MCP spec may evolve to include proper notification support.
func (s *Server) handleMCPSendNotification(w http.ResponseWriter, r *http.Request) {
	// Currently, we don't support notifications, so return appropriate error
	if r.Method != http.MethodPost {
		writeStandardErrorResponse(w, MethodNotFound,
			"Method not allowed. Notification endpoint requires POST",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Return unsupported operation for now
	writeStandardErrorResponse(w, InternalError,
		"Notifications not yet supported",
		map[string]string{"feature_status": "planned"})
}

// handleTagsResource retrieves and formats all tags from RTM.
func (s *Server) handleTagsResource() (string, error) {
	tags, err := s.rtmService.GetTags()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve tags: %w", err)
	}
	return formatTags(tags), nil
}

// getAuthenticatedTools returns the list of available tools when authenticated.
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
		// Add more tool definitions as needed...
	}
}

// handleAuthResource han
