package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/rtm"
)

// MCPServer represents an MCP server for RTM
type MCPServer struct {
	config     *config.Config
	rtmService *rtm.Service
	httpServer *http.Server
	mu         sync.Mutex
	authFlows  map[string]string // Map of frobs to pending auth flows
}

// NewMCPServer creates a new MCP server
func NewMCPServer(cfg *config.Config) (*MCPServer, error) {
	// Create token directory if it doesn't exist
	tokenDir := filepath.Dir(cfg.Auth.TokenPath)
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return nil, fmt.Errorf("error creating token directory: %w", err)
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
		config:     cfg,
		rtmService: rtmService,
		authFlows:  make(map[string]string),
	}, nil
}

// Start starts the MCP server
func (s *MCPServer) Start() error {
	// Create HTTP server
	mux := http.NewServeMux()

	// Set up handlers
	mux.HandleFunc("/mcp/initialize", s.handleInitialize)
	mux.HandleFunc("/mcp/list_resources", s.handleListResources)
	mux.HandleFunc("/mcp/read_resource", s.handleReadResource)
	mux.HandleFunc("/mcp/list_tools", s.handleListTools)
	mux.HandleFunc("/mcp/call_tool", s.handleCallTool)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Server.Port),
		Handler: mux,
	}

	// Start HTTP server
	log.Printf("Starting MCP server on port %d", s.config.Server.Port)
	return s.httpServer.ListenAndServe()
}

// Stop stops the MCP server
func (s *MCPServer) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleInitialize handles the initialize request
func (s *MCPServer) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		ServerName    string `json:"server_name"`
		ServerVersion string `json:"server_version"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Define server capabilities
	response := map[string]interface{}{
		"server_info": map[string]interface{}{
			"name":    s.config.Server.Name,
			"version": "1.0.0", // TODO: Use version from build
		},
		"capabilities": map[string]interface{}{
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
		},
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleListResources handles the list_resources request
func (s *MCPServer) handleListResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if !s.rtmService.IsAuthenticated() {
		// Return authentication resources only
		authResource := map[string]interface{}{
			"name":        "auth://rtm",
			"description": "Authentication for Remember The Milk",
			"arguments":   []map[string]interface{}{},
		}

		response := map[string]interface{}{
			"resources": []interface{}{authResource},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Define resources
	resources := []map[string]interface{}{
		{
			"name":        "tasks://all",
			"description": "All tasks across all lists",
			"arguments":   []map[string]interface{}{},
		},
		{
			"name":        "tasks://today",
			"description": "Tasks due today",
			"arguments":   []map[string]interface{}{},
		},
		{
			"name":        "tasks://tomorrow",
			"description": "Tasks due tomorrow",
			"arguments":   []map[string]interface{}{},
		},
		{
			"name":        "tasks://week",
			"description": "Tasks due within the next 7 days",
			"arguments":   []map[string]interface{}{},
		},
		{
			"name":        "tasks://list/{list_id}",
			"description": "Tasks within a specific list",
			"arguments": []map[string]interface{}{
				{
					"name":        "list_id",
					"description": "The ID of the list",
					"required":    true,
				},
			},
		},
		{
			"name":        "lists://all",
			"description": "All task lists",
			"arguments":   []map[string]interface{}{},
		},
		{
			"name":        "tags://all",
			"description": "All tags used in the system",
			"arguments":   []map[string]interface{}{},
		},
	}

	response := map[string]interface{}{
		"resources": resources,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleReadResource handles the read_resource request
func (s *MCPServer) handleReadResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get resource name from query parameters
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing resource name", http.StatusBadRequest)
		return
	}

	// Handle authentication resource
	if name == "auth://rtm" {
		if s.rtmService.IsAuthenticated() {
			// Already authenticated
			response := map[string]interface{}{
				"content": "Authentication successful. You are already authenticated with Remember The Milk.",
				"mime_type": "text/plain",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		// Start authentication flow
		authURL, frob, err := s.rtmService.StartAuthFlow()
		if err != nil {
			log.Printf("Error starting auth flow: %v", err)
			http.Error(w, "Error starting auth flow", http.StatusInternalServerError)
			return
		}

		// Store frob for later use
		s.mu.Lock()
		s.authFlows[frob] = frob
		s.mu.Unlock()

		// Return auth URL
		content := fmt.Sprintf(
			"Please authorize CowGnition to access your Remember The Milk account by visiting the following URL: %s\n\n"+
				"After authorizing, you will be given a frob. Use the 'authenticate' tool with this frob to complete the authentication.",
			authURL,
		)

		response := map[string]interface{}{
			"content":   content,
			"mime_type": "text/plain",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Check authentication for other resources
	if !s.rtmService.IsAuthenticated() {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Handle other resources
	var content string
	var mimeType string
	var err error

	if name == "lists://all" {
		content, err = s.handleListsResource()
	} else if name == "tasks://all" {
		content, err = s.handleTasksResource("")
	} else if name == "tasks://today" {
		content, err = s.handleTasksResource("due:today")
	} else if name == "tasks://tomorrow" {
		content, err = s.handleTasksResource("due:tomorrow")
	} else if name == "tasks://week" {
		content, err = s.handleTasksResource("due:\"within 7 days\"")
	} else if strings.HasPrefix(name, "tasks://list/") {
		// Extract list ID from resource name
		listID := strings.TrimPrefix(name, "tasks://list/")
		content, err = s.handleTasksResource("list:" + listID)
	} else if name == "tags://all" {
		content, err = s.handleTagsResource()
	} else {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	if err != nil {
		log.Printf("Error handling resource: %v", err)
		http.Error(w, "Error handling resource", http.StatusInternalServerError)
		return
	}

	mimeType = "text/plain"

	response := map[string]interface{}{
		"content":   content,
		"mime_type": mimeType,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleListsResource handles the lists resource
func (s *MCPServer) handleListsResource() (string, error) {
	lists, err := s.rtmService.GetLists()
	if err != nil {
		return "", fmt.Errorf("error getting lists: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Remember The Milk Lists:\n\n")

	for _, list := range lists {
		if list.Deleted == "0" && list.Archived == "0" {
			sb.WriteString(fmt.Sprintf("- %s (ID: %s)\n", list.Name, list.ID))
		}
	}

	return sb.String(), nil
}

// handleTasksResource handles the tasks resource
func (s *MCPServer) handleTasksResource(filter string) (string, error) {
	tasksResp, err := s.rtmService.GetTasks(filter)
	if err != nil {
		return "", fmt.Errorf("error getting tasks: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Remember The Milk Tasks:\n\n")

	for _, list := range tasksResp.Tasks.List {
		for _, taskSeries := range list.TaskSeries {
			for _, task := range taskSeries.Tasks {
				// Skip completed or deleted tasks
				if task.Completed != "" || task.Deleted != "" {
					continue
				}

				// Format due date
				dueStr := "No due date"
				if task.Due != "" {
					dueStr = fmt.Sprintf("Due: %s", task.Due)
				}

				// Format priority
				priorityStr := ""
				if task.Priority != "N" {
					priorityStr = fmt.Sprintf(" (Priority: %s)", task.Priority)
				}

				// Format tags
				tagsStr := ""
				if len(taskSeries.Tags) > 0 {
					tagsStr = fmt.Sprintf(" [Tags: %s]", strings.Join(taskSeries.Tags, ", "))
				}

				sb.WriteString(fmt.Sprintf("- %s%s - %s%s\n", taskSeries.Name, priorityStr, dueStr, tagsStr))

				// Add task ID for reference
				sb.WriteString(fmt.Sprintf("  List ID: %s, TaskSeries ID: %s, Task ID: %s\n", list.ID, taskSeries.ID, task.ID))

				// Add notes if any
				if len(taskSeries.Notes) > 0 {
					sb.WriteString("  Notes:\n")
					for _, note := range taskSeries.Notes {
						sb.WriteString(fmt.Sprintf("  - %s\n", note.Content))
					}
				}

				sb.WriteString("\n")
			}
		}
	}

	return sb.String(), nil
}

// handleTagsResource handles the tags resource
func (s *MCPServer) handleTagsResource() (string, error) {
	// Get all tasks to extract tags
	tasksResp, err := s.rtmService.GetTasks("")
	if err != nil {
		return "", fmt.Errorf("error getting tasks: %w", err)
	}

	// Collect unique tags
	tagsMap := make(map[string]bool)
	for _, list := range tasksResp.Tasks.List {
		for _, taskSeries := range list.TaskSeries {
			for _, tag := range taskSeries.Tags {
				tagsMap[tag] = true
			}
		}
	}

	// Convert to slice and sort
	var tags []string
	for tag := range tagsMap {
		tags = append(tags, tag)
	}

	var sb strings.Builder
	sb.WriteString("Remember The Milk Tags:\n\n")

	if len(tags) == 0 {
		sb.WriteString("No tags found.")
	} else {
		for _, tag := range tags {
			sb.WriteString(fmt.Sprintf("- %s\n", tag))
		}
	}

	return sb.String(), nil
}

// handleListTools handles the list_tools request
func (s *MCPServer) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Define authentication tool
	authTool := map[string]interface{}{
		"name":        "authenticate",
		"description": "Complete authentication with Remember The Milk using the provided frob",
		"arguments": []map[string]interface{}{
			{
				"name":        "frob",
				"description": "The frob provided by Remember The Milk after authorization",
				"required":    true,
			},
		},
	}

	// Define tools
	var tools []map[string]interface{}

	// Add authentication tool if not authenticated
	if !s.rtmService.IsAuthenticated() {
		tools = []map[string]interface{}{authTool}
	} else {
		// Add RTM tools
		tools = []map[string]interface{}{
			{
				"name":        "add_task",
				"description": "Create a new task",
				"arguments": []map[string]interface{}{
					{
						"name":        "name",
						"description": "The name of the task",
						"required":    true,
					},
					{
						"name":        "list_id",
						"description": "The ID of the list to add the task to (default: Inbox)",
						"required":    false,
					},
					{
						"name":        "due_date",
						"description": "The due date of the task (e.g., 'today', 'tomorrow', 'next wednesday')",
						"required":    false,
					},
				},
			},
			{
				"name":        "complete_task",
				"description": "Mark a task as completed",
				"arguments": []map[string]interface{}{
					{
						"name":        "list_id",
						"description": "The ID of the list containing the task",
						"required":    true,
					},
					{
						"name":        "taskseries_id",
						"description": "The ID of the task series",
						"required":    true,
					},
					{
						"name":        "task_id",
						"description": "The ID of the task",
						"required":    true,
					},
				},
			},
			{
				"name":        "add_tags",
				"description": "Add tags to a task",
				"arguments": []map[string]interface{}{
					{
						"name":        "list_id",
						"description": "The ID of the list containing the task",
						"required":    true,
					},
					{
						"name":        "taskseries_id",
						"description": "The ID of the task series",
						"required":    true,
					},
					{
						"name":        "task_id",
						"description": "The ID of the task",
						"required":    true,
					},
					{
						"name":        "tags",
						"description": "The tags to add to the task (comma-separated)",
						"required":    true,
					},
				},
			},
		}
	}

	response := map[string]interface{}{
		"tools": tools,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleCallTool handles the call_tool request
func (s *MCPServer) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Handle authentication
	if req.Name == "authenticate" {
		frob, ok := req.Arguments["frob"].(string)
		if !ok || frob == "" {
			http.Error(w, "Missing frob argument", http.StatusBadRequest)
			return
		}

		// Complete authentication flow
		if err := s.rtmService.CompleteAuthFlow(frob); err != nil {
			log.Printf("Error completing auth flow: %v", err)
			http.Error(w, "Error completing auth flow", http.StatusInternalServerError)
			return
		}

		// Remove frob from pending auth flows
		s.mu.Lock()
		delete(s.authFlows, frob)
		s.mu.Unlock()

		// Return success response
		response := map[string]interface{}{
			"result": "Authentication successful. You can now use Remember The Milk with Claude.",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Check authentication for other tools
	if !s.rtmService.IsAuthenticated() {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Create timeline for operations that need it
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		log.Printf("Error creating timeline: %v", err)
		http.Error(w, "Error creating timeline", http.StatusInternalServerError)
		return
	}

	// Handle different tools
	var result string

	switch req.Name {
	case "add_task":
		result, err = s.handleAddTask(timeline, req.Arguments)
	case "complete_task":
		result, err = s.handleCompleteTask(timeline, req.Arguments)
	case "add_tags":
		result, err = s.handleAddTags(timeline, req.Arguments)
	default:
		http.Error(w, "Tool not found", http.StatusNotFound)
		return
	}

	if err != nil {
		log.Printf("Error handling tool: %v", err)
		http.Error(w, "Error handling tool: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"result": result,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleAddTask handles the add_task tool
func (s *MCPServer) handleAddTask(timeline string, args map[string]interface{}) (string, error) {
	// Get arguments
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing or invalid name argument")
	}

	// Default list ID is the inbox
	listID := "0" // Inbox
	if listIDArg, ok := args["list_id"].(string); ok && listIDArg != "" {
		listID = listIDArg
	}

	// Due date is optional
	dueDate := ""
	if dueDateArg, ok := args["due_date"].(string); ok {
		dueDate = dueDateArg
	}

	// Add task
	if err := s.rtmService.AddTask(timeline, listID, name, dueDate); err != nil {
		return "", fmt.Errorf("error adding task: %w", err)
	}

	return fmt.Sprintf("Task '%s' added successfully.", name), nil
}

// handleCompleteTask handles the complete_task tool
func (s *MCPServer) handleCompleteTask(timeline string, args map[string]interface{}) (string, error) {
	// Get arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id argument")
	}

	// Complete task
	if err := s.rtmService.CompleteTask(timeline, listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("error completing task: %w", err)
	}

	return "Task completed successfully.", nil
}

// handleAddTags handles the add_tags tool
func (s *MCPServer) handleAddTags(timeline string, args map[string]interface{}) (string, error) {
	// Get arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid list_id argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid taskseries_id argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid task_id argument")
	}

	tagsArg, ok := args["tags"].(string)
	if !ok || tagsArg == "" {
		return "", fmt.Errorf("missing or invalid tags argument")
	}

	// Split tags by comma
	tags := strings.Split(tagsArg, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}

	// Add tags
	if err := s.rtmService.AddTags(timeline, listID, taskseriesID, taskID, tags); err != nil {
		return "", fmt.Errorf("error adding tags: %w", err)
	}

	return fmt.Sprintf("Tags '%s' added to task successfully.", tagsArg), nil
}
