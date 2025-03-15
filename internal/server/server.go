// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cowgnition/cowgnition/internal/auth"
	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/rtm"
	"github.com/cowgnition/cowgnition/pkg/mcp"
)

// MCPServer represents an MCP server for RTM integration.
type MCPServer struct {
	config       *config.Config
	rtmService   *rtm.Service
	httpServer   *http.Server
	tokenManager *auth.TokenManager
}

// NewMCPServer creates a new MCP server with the provided configuration.
// It initializes the RTM service and authentication token manager.
func NewMCPServer(cfg *config.Config) (*MCPServer, error) {
	// Create token manager
	tokenManager, err := auth.NewTokenManager(cfg.Auth.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("error creating token manager: %w", err)
	}

	// Create RTM service
	rtmService := rtm.NewService(
		cfg.RTM.APIKey,
		cfg.RTM.SharedSecret,
		cfg.Auth.TokenPath,
	)

	// Initialize RTM service
	if err := rtmService.Initialize(); err != nil {
		return nil, fmt.Errorf("error initializing RTM service: %w", err)
	}

	return &MCPServer{
		config:       cfg,
		rtmService:   rtmService,
		tokenManager: tokenManager,
	}, nil
}

// Start starts the MCP server and returns an error if it fails to start.
func (s *MCPServer) Start() error {
	// Create router
	mux := http.NewServeMux()

	// Register handlers
	s.registerHandlers(mux)

	// Add logging middleware
	handler := logMiddleware(mux)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	log.Printf("Starting MCP server on port %d", s.config.Server.Port)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

// Register all handlers and routes for the server
func (s *MCPServer) registerHandlers(mux *http.ServeMux) {
	// Register MCP protocol handlers
	mux.HandleFunc("/mcp/initialize", s.handleInitialize)
	mux.HandleFunc("/mcp/list_resources", s.handleListResources)
	mux.HandleFunc("/mcp/read_resource", s.handleReadResource)
	mux.HandleFunc("/mcp/list_tools", s.handleListTools)
	mux.HandleFunc("/mcp/call_tool", s.handleCallTool)
}

// Stop gracefully stops the MCP server with the given context timeout.
func (s *MCPServer) Stop(ctx context.Context) error {
	log.Println("Shutting down MCP server...")
	return s.httpServer.Shutdown(ctx)
}

// logMiddleware adds simple request logging to the server.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("Request: %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("Response: %s %s completed in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// writeJSONResponse writes a JSON response with the given status code and data.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes a JSON error response with the given status code and message.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := map[string]interface{}{
		"error": message,
	}
	writeJSONResponse(w, statusCode, response)
}

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
		authURL, frob, err := s.rtmService.StartAuthFlow()
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

// handleListsResource retrieves and formats the list of RTM lists.
func (s *MCPServer) handleListsResource() (string, error) {
	// Fetch lists from RTM service
	lists, err := s.rtmService.GetLists()
	if err != nil {
		return "", fmt.Errorf("error fetching lists: %w", err)
	}

	// Format the lists
	var sb strings.Builder
	sb.WriteString("# Remember The Milk Lists\n\n")
	
	// Track categories
	var smartLists, inboxLists, sentLists, regularLists []rtm.List
	
	// Categorize lists
	for _, list := range lists {
		switch {
		case list.Smart == "1":
			smartLists = append(smartLists, list)
		case list.Name == "Inbox":
			inboxLists = append(inboxLists, list)
		case list.Name == "Sent":
			sentLists = append(sentLists, list)
		default:
			regularLists = append(regularLists, list)
		}
	}
	
	// Show inbox lists
	if len(inboxLists) > 0 {
		sb.WriteString("## System Lists\n")
		for _, list := range inboxLists {
			sb.WriteString(fmt.Sprintf("- Inbox (ID: %s)\n", list.ID))
		}
		
		for _, list := range sentLists {
			sb.WriteString(fmt.Sprintf("- Sent (ID: %s)\n", list.ID))
		}
		sb.WriteString("\n")
	}
	
	// Show regular lists
	if len(regularLists) > 0 {
		sb.WriteString("## Regular Lists\n")
		for _, list := range regularLists {
			sb.WriteString(fmt.Sprintf("- %s (ID: %s)\n", list.Name, list.ID))
		}
		sb.WriteString("\n")
	}
	
	// Show smart lists
	if len(smartLists) > 0 {
		sb.WriteString("## Smart Lists\n")
		for _, list := range smartLists {
			filter := ""
			if list.Filter != "" {
				filter = fmt.Sprintf(" - Filter: %s", list.Filter)
			}
			sb.WriteString(fmt.Sprintf("- %s (ID: %s)%s\n", list.Name, list.ID, filter))
		}
		sb.WriteString("\n")
	}
	
	// Show total count
	sb.WriteString(fmt.Sprintf("Total: %d lists\n", len(lists)))
	
	return sb.String(), nil
}

// handleTasksResource retrieves and formats tasks according to the provided filter.
func (s *MCPServer) handleTasksResource(filter string) (string, error) {
	// Fetch tasks from RTM service
	tasks, err := s.rtmService.GetTasks(filter)
	if err != nil {
		return "", fmt.Errorf("error fetching tasks: %w", err)
	}
	
	// Format tasks
	var sb strings.Builder
	
	// Add header based on filter
	switch filter {
	case "":
		sb.WriteString("# All Remember The Milk Tasks\n\n")
	case "due:today":
		sb.WriteString("# Tasks Due Today\n\n")
	case "due:tomorrow":
		sb.WriteString("# Tasks Due Tomorrow\n\n")
	case "due:\"within 7 days\"":
		sb.WriteString("# Tasks Due This Week\n\n")
	default:
		if strings.HasPrefix(filter, "list:") {
			listID := strings.TrimPrefix(filter, "list:")
			sb.WriteString(fmt.Sprintf("# Tasks in List %s\n\n", listID))
		} else {
			sb.WriteString(fmt.Sprintf("# Tasks Matching: %s\n\n", filter))
		}
	}
	
	// Format tasks
	formattedTasks := tasks.GetFormattedTasks()
	sb.WriteString(formattedTasks)
	
	return sb.String(), nil
}

// handleTagsResource retrieves and formats all tags in use.
func (s *MCPServer) handleTagsResource() (string, error) {
	// Fetch all tasks from RTM service
	tasks, err := s.rtmService.GetTasks("")
	if err != nil {
		return "", fmt.Errorf("error fetching tasks: %w", err)
	}
	
	// Extract and deduplicate tags
	tagMap := make(map[string]int)
	
	for _, list := range tasks.Tasks.List {
		for _, ts := range list.TaskSeries {
			for _, tag := range ts.Tags.Tag {
				if tag != "" {
					tagMap[tag]++
				}
			}
		}
	}
	
	// Sort tags by name
	var tags []string
	for tag := range tagMap {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	
	// Format tags
	var sb strings.Builder
	sb.WriteString("# Remember The Milk Tags\n\n")
	
	if len(tags) == 0 {
		sb.WriteString("No tags found.")
		return sb.String(), nil
	}
	
	for _, tag := range tags {
		count := tagMap[tag]
		if count == 1 {
			sb.WriteString(fmt.Sprintf("- %s (1 task)\n", tag))
		} else {
			sb.WriteString(fmt.Sprintf("- %s (%d tasks)\n", tag, count))
		}
	}
	
	sb.WriteString(fmt.Sprintf("\nTotal: %d unique tags\n", len(tags)))
	
	return sb.String(), nil
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
						Description: "The frob received after visiting the authentication URL",
						Required:    true,
					},
				},
			},
		}
	} else {
		// Return all available tools for authenticated users
		tools = []mcp.ToolDefinition{
			{
				Name:        "add_task",
				Description: "Create a new task",
				Arguments: []mcp.ToolArgument{
					{
						Name:        "name",
						Description: "The task name",
						Required:    true,
					},
					{
						Name:        "list_id",
						Description: "The list ID to add the task to (defaults to Inbox)",
						Required:    false,
					},
					{
						Name:        "due_date",
						Description: "The due date (e.g., 'today', 'tomorrow', 'next friday')",
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
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
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
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
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
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
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
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
						Required:    true,
					},
					{
						Name:        "due_date",
						Description: "The due date (e.g., 'today', 'tomorrow', 'next friday')",
						Required:    true,
					},
					{
						Name:        "has_due_time",
						Description: "Whether the due date includes a time (true/false)",
						Required:    false,
					},
				},
			},
			{
				Name:        "set_priority",
				Description: "Set a task's priority level",
				Arguments: []mcp.ToolArgument{
					{
						Name:        "list_id",
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
						Required:    true,
					},
					{
						Name:        "priority",
						Description: "The priority level (1=high, 2=medium, 3=low, N=none)",
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
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
						Required:    true,
					},
					{
						Name:        "tags",
						Description: "Comma-separated list of tags to add",
						Required:    true,
					},
				},
			},
			{
				Name:        "remove_tags",
				Description: "Remove tags from a task",
				Arguments: []mcp.ToolArgument{
					{
						Name:        "list_id",
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
						Required:    true,
					},
					{
						Name:        "tags",
						Description: "Comma-separated list of tags to remove",
						Required:    true,
					},
				},
			},
			{
				Name:        "add_note",
				Description: "Add a note to a task",
				Arguments: []mcp.ToolArgument{
					{
						Name:        "list_id",
						Description: "The list ID containing the task",
						Required:    true,
					},
					{
						Name:        "taskseries_id",
						Description: "The task series ID",
						Required:    true,
					},
					{
						Name:        "task_id",
						Description: "The task ID",
						Required:    true,
					},
					{
						Name:        "title",
						Description: "The note title",
						Required:    false,
					},
					{
						Name:        "text",
						Description: "The note content",
						Required:    true,
					},
				},
			},
		}
	}

	response := mcp.ListToolsResponse{
		Tools: tools,
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// handleCallTool handles the MCP call_tool request.
// It executes the specified tool with the provided arguments.
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
		// Check if authenticated already
		if s.rtmService.IsAuthenticated() {
			writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
				Result: "You are already authenticated with Remember The Milk.",
			})
			return
		}

		// Get frob from arguments
		frob, ok := req.Arguments["frob"].(string)
		if !ok || frob == "" {
			writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid frob parameter")
			return
		}

		// Complete authentication flow
		if err := s.rtmService.CompleteAuthFlow(frob); err != nil {
			log.Printf("Error completing auth flow: %v", err)
			writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Authentication failed: %v", err))
			return
		}

		// Return success response
		writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
			Result: "Authentication successful! You can now use Remember The Milk tasks.",
		})
		return
	}

	// For all other tools, check authentication
	if !s.rtmService.IsAuthenticated() {
		writeErrorResponse(w, http.StatusUnauthorized, "Not authenticated with Remember The Milk")
		return
	}

	// Handle tool calls
	var result string
	var err error

	// Create a timeline for operations that support undo
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		log.Printf("Error creating timeline: %v", err)
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error creating timeline: %v", err))
		return
	}

	switch req.Name {
	case "add_task":
		result, err = s.handleAddTask(timeline, req.Arguments)
	case "complete_task":
		result, err = s.handleCompleteTask(timeline, req.Arguments)
	case "uncomplete_task":
		result, err = s.handleUncompleteTask(timeline, req.Arguments)
	case "delete_task":
		result, err = s.handleDeleteTask(timeline, req.Arguments)
	case "set_due_date":
		result, err = s.handleSetDueDate(timeline, req.Arguments)
	case "set_priority":
		result, err = s.handleSetPriority(timeline, req.Arguments)
	case "add_tags":
		result, err = s.handleAddTags(timeline, req.Arguments)
	case "remove_tags":
		result, err = s.handleRemoveTags(timeline, req.Arguments)
	case "add_note":
		result, err = s.handleAddNote(timeline, req.Arguments)
	default:
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Unknown tool: %s", req.Name))
		return
	}

	if err != nil {
		log.Printf("Error calling tool %s: %v", req.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error calling tool: %v", err))
		return
	}

	writeJSONResponse(w, http.StatusOK, mcp.ToolResponse{
		Result: result,
	})
}

// handleAddTask handles the add_task tool.
func (s *MCPServer) handleAddTask(timeline string, args map[string]interface{}) (string, error) {
	// Get name parameter
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing or invalid name parameter")
	}

	// Get optional list_id parameter (default to Inbox)
	listID, _ := args["list_id"].(string)
	if listID == "" {
		// Get Inbox list ID
		lists, err := s.rtmService.GetLists()
		if err != nil {
			return "", fmt.Errorf("error getting lists: %w", err)
		}

		for _, list := range lists {
			if list.Name == "Inbox" {
				listID = list.ID
				break
			}
		}

		if listID == "" {
			return "", fmt.Errorf("couldn't find Inbox list")
		}
	}

	// Get optional due_date parameter
	dueDate, _ := args["due_date"].(string)

	// Add task
	err := s.rtmService.AddTask(timeline, listID, name, dueDate)
	if err != nil {
		return "", fmt.Errorf("error adding task: %w", err)
	}

	result := fmt.Sprintf("Successfully added task: %s", name)
	if dueDate != "" {
		result += fmt.Sprintf(" (Due: %s)", dueDate)
	}

	return result, nil
}

// handleCompleteTask handles the complete_task tool.
func (s *MCPServer) handleCompleteTask(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	// Complete task
	err := s.rtmService.CompleteTask(timeline, listID, taskseriesID, taskID)
	if err != nil {
		return "", fmt.Errorf("error completing task: %w", err)
	}

	return "Task marked as completed", nil
}

// handleUncompleteTask handles the uncomplete_task tool.
func (s *MCPServer) handleUncompleteTask(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	// Uncomplete task
	err := s.rtmService.UncompleteTask(timeline, listID, taskseriesID, taskID)
	if err != nil {
		return "", fmt.Errorf("error uncompleting task: %w", err)
	}

	return "Task marked as incomplete", nil
}

// handleDeleteTask handles the delete_task tool.
func (s *MCPServer) handleDeleteTask(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	// Delete task
	err := s.rtmService.DeleteTask(timeline, listID, taskseriesID, taskID)
	if err != nil {
		return "", fmt.Errorf("error deleting task: %w", err)
	}

	return "Task deleted successfully", nil
}

// handleSetDueDate handles the set_due_date tool.
func (s *MCPServer) handleSetDueDate(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	dueDate, ok := args["due_date"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid due_date parameter")
	}

	// Get optional has_due_time parameter
	hasDueTime := false
	if hasDueTimeStr, ok := args["has_due_time"].(string); ok {
		hasDueTime = hasDueTimeStr == "true" || hasDueTimeStr == "1"
	} else if hasDueTimeBool, ok := args["has_due_time"].(bool); ok {
		hasDueTime = hasDueTimeBool
	}

	// Set due date
	err := s.rtmService.SetDueDate(timeline, listID, taskseriesID, taskID, dueDate, hasDueTime)
	if err != nil {
		return "", fmt.Errorf("error setting due date: %w", err)
	}

	if dueDate == "" {
		return "Due date cleared successfully", nil
	}
	return fmt.Sprintf("Due date set to %s", dueDate), nil
}

// handleSetPriority handles the set_priority tool.
func (s *MCPServer) handleSetPriority(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	priority, ok := args["priority"].(string)
	if !ok || priority == "" {
		return "", fmt.Errorf("missing or invalid priority parameter")
	}

	// Validate priority
	validPriorities := map[string]bool{"1": true, "2": true, "3": true, "N": true}
	if !validPriorities[priority] {
		return "", fmt.Errorf("invalid priority: must be 1 (high), 2 (medium), 3 (low), or N (none)")
	}

	// Set priority
	err := s.rtmService.SetPriority(timeline, listID, taskseriesID, taskID, priority)
	if err != nil {
		return "", fmt.Errorf("error setting priority: %w", err)
	}

	priorityNames := map[string]string{
		"1": "high",
		"2": "medium",
		"3": "low",
		"N": "none",
	}

	return fmt.Sprintf("Priority set to %s", priorityNames[priority]), nil
}

// handleAddTags handles the add_tags tool.
func (s *MCPServer) handleAddTags(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	tagsStr, ok := args["tags"].(string)
	if !ok || tagsStr == "" {
		return "", fmt.Errorf("missing or invalid tags parameter")
	}

	// Parse tags
	tags := parseTags(tagsStr)
	if len(tags) == 0 {
		return "", fmt.Errorf("no valid tags provided")
	}

	// Add tags
	err := s.rtmService.AddTags(timeline, listID, taskseriesID, taskID, tags)
	if err != nil {
		return "", fmt.Errorf("error adding tags: %w", err)
	}

	return fmt.Sprintf("Added tags: %s", strings.Join(tags, ", ")), nil
}

// handleRemoveTags handles the remove_tags tool.
func (s *MCPServer) handleRemoveTags(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	tagsStr, ok := args["tags"].(string)
	if !ok || tagsStr == "" {
		return "", fmt.Errorf("missing or invalid tags parameter")
	}

	// Parse tags
	tags := parseTags(tagsStr)
	if len(tags) == 0 {
		return "", fmt.Errorf("no valid tags provided")
	}

	// Remove tags
	err := s.rtmService.RemoveTags(timeline, listID, taskseriesID, taskID, tags)
	if err != nil {
		return "", fmt.Errorf("error removing tags: %w", err)
	}

	return fmt.Sprintf("Removed tags: %s", strings.Join(tags, ", ")), nil
}

// handleAddNote handles the add_note tool.
func (s *MCPServer) handleAddNote(timeline string, args map[string]interface{}) (string, error) {
	// Get required parameters
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id parameter")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id parameter")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id parameter")
	}

	text, ok := args["text"].(string)
	if !ok || text == "" {
		return "", fmt.Errorf("missing or invalid text parameter")
	}

	// Get optional title parameter
	title, _ := args["title"].(string)

	// Add note
	err := s.rtmService.AddNote(timeline, listID, taskseriesID, taskID, title, text)
	if err != nil {
		return "", fmt.Errorf("error adding note: %w", err)
	}

	return "Note added successfully", nil
}

// parseTags splits a comma-separated list of tags into a slice of strings.
func parseTags(tagsStr string) []string {
	// Split by comma
	tagsSplit := strings.Split(tagsStr, ",")
	
	// Trim whitespace and filter empty tags
	var tags []string
	for _, tag := range tagsSplit {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	
	return tags
}