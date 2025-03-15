// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

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
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Construct server information
	serverInfo := mcp.ServerInfo{
		Name:    s.config.Server.Name,
		Version: "1.0.0", // TODO: Use version from build
	}

	// Define capabilities
	capabilities := map[string]interface{}{
		"resources": map[string]interface{}{
			"list": true,
			"read": true,
		},
		"tools": map[string]interface{}{
			"list": true,
			"call": true,
		},
		"logging": map[string]interface{}{
			"log":     true,
			"warning": true,
			"error":   true,
		},
	}

	// Construct response
	response := mcp.InitializeResponse{
		ServerInfo:   serverInfo,
		Capabilities: capabilities,
	}

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
		writeErrorResponse(w, http.StatusBadRequest, "Missing resource name")
		return
	}

	// Handle authentication resource
	if name == "auth://rtm" {
		if s.rtmService.IsAuthenticated() {
			// Already authenticated
			response := mcp.ResourceResponse{
				Content:  "Authentication successful. You are already authenticated with Remember The Milk.",
				MimeType: "text/plain",
			}
			writeJSONResponse(w, http.StatusOK, response)
			return
		}

		// Start authentication flow
		authURL, _, err := s.rtmService.StartAuthFlow()
		if err != nil {
			log.Printf("Error starting auth flow: %v", err)
			writeErrorResponse(w, http.StatusInternalServerError, "Error starting authentication flow")
			return
		}

		// Return auth URL
		content := fmt.Sprintf(
			"Please authorize CowGnition to access your Remember The Milk account by visiting the following URL: %s\n\n"+
				"After authorizing, you will be given a frob. Use the 'authenticate' tool with this frob to complete the authentication.",
			authURL,
		)

		response := mcp.ResourceResponse{
			Content:  content,
			MimeType: "text/plain",
		}

		writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// Check authentication for other resources
	if !s.rtmService.IsAuthenticated() {
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk")
		return
	}

	// Handle different resource types
	var content string
	var err error
	mimeType := "text/plain"

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
		writeErrorResponse(w, http.StatusNotFound, "Resource not found")
		return
	}

	if err != nil {
		log.Printf("Error handling resource: %v", err)
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
		tools = []mcp.ToolDefinition{
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
			// Add more tools as needed
		}
	}

	response := mcp.ListToolsResponse{
		Tools: tools,
	}

	writeJSONResponse(w, http.StatusOK, response)
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
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Handle authentication tool
	if req.Name == "authenticate" {
		if s.rtmService.IsAuthenticated() {
			writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
				Result: "Already authenticated with Remember The Milk.",
			})
			return
		}

		// Get frob from arguments
		frob, ok := req.Arguments["frob"].(string)
		if !ok || frob == "" {
			writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid 'frob' argument")
			return
		}

		// Complete authentication flow
		if err := s.rtmService.CompleteAuthFlow(frob); err != nil {
			log.Printf("Error completing auth flow: %v", err)
			writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error completing authentication: %v", err))
			return
		}

		writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
			Result: "Authentication successful! You can now use all features of Remember The Milk.",
		})
		return
	}

	// Check authentication for other tools
	if !s.rtmService.IsAuthenticated() {
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk")
		return
	}

	// Handle different tools
	var result string
	var err error

	switch req.Name {
	case "add_task":
		result, err = s.handleAddTaskTool(req.Arguments)
	case "complete_task":
		result, err = s.handleCompleteTaskTool(req.Arguments)
	// Add more tools as needed
	default:
		writeErrorResponse(w, http.StatusNotFound, "Tool not found")
		return
	}

	if err != nil {
		log.Printf("Error handling tool: %v", err)
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error handling tool: %v", err))
		return
	}

	writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
		Result: result,
	})
}
