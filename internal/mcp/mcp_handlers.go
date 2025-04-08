// file: internal/mcp/mcp_handlers.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt" // Import fmt for formatting.

	"github.com/cockroachdb/errors" // Using cockroachdb/errors for wrapping.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	// Import RTM client package here when created.
	// "github.com/dkoosis/cowgnition/internal/rtm".
)

// Handler holds dependencies for MCP method handlers.
type Handler struct {
	logger logging.Logger
	config *config.Config
	// Add RTM client instance here when available:
	// rtmClient *rtm.Client.
}

// NewHandler creates a new Handler.
func NewHandler(cfg *config.Config, logger logging.Logger) *Handler {
	// TODO: Initialize RTM client here when implemented, passing cfg.RTM.APIKey etc.
	return &Handler{
		logger: logger.WithField("component", "mcp_handler"),
		config: cfg,
		// rtmClient: rtm.NewClient(cfg.RTM.APIKey, cfg.RTM.SharedSecret, logger), // Example.
	}
}

// handleInitialize handles the initialize request. (Unchanged from previous version).
func (h *Handler) handleInitialize(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for initialize")
	}
	h.logger.Info("Handling initialize request.", "clientVersion", req.ProtocolVersion, "clientName", req.ClientInfo.Name)

	// Enable tools capability.
	caps := ServerCapabilities{
		Tools: &ToolsCapability{ListChanged: false},
		// Optionally enable Resources capability later:
		// Resources: &ResourcesCapability{ListChanged: false, Subscribe: false},.
	}

	appVersion := "0.1.0-dev" // TODO: Get from build flags.
	serverInfo := Implementation{Name: h.config.Server.Name, Version: appVersion}
	res := InitializeResult{
		ServerInfo:      serverInfo,
		ProtocolVersion: "2024-11-05",
		Capabilities:    caps,
	}

	resultBytes, err := json.Marshal(res)
	if err != nil {
		h.logger.Error("Failed to marshal InitializeResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal InitializeResult")
	}
	return resultBytes, nil
}

// handlePing handles the ping request. (Unchanged from previous version).
func (h *Handler) handlePing(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Debug("Handling ping request.")
	resultBytes, err := json.Marshal(map[string]interface{}{})
	if err != nil {
		h.logger.Error("Failed to marshal empty ping result.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ping response")
	}
	return resultBytes, nil
}

// handleToolsList handles the tools/list request.
// Defines actual RTM tools.
func (h *Handler) handleToolsList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling tools/list request.")

	// Define RTM tools.
	tools := []Tool{
		// Tool: rtm/getTasks
		{
			Name:        "rtm/getTasks",
			Description: "Retrieves tasks from Remember The Milk based on a specified filter.",
			InputSchema: mustMarshalJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter": map[string]interface{}{
						"type":        "string",
						"description": "RTM filter expression (e.g., 'list:Inbox status:incomplete dueBefore:tomorrow'). See RTM documentation for filter syntax.",
					},
				},
				"required": []string{"filter"},
			}),
		},
		// Tool: rtm/createTask
		{
			Name:        "rtm/createTask",
			Description: "Creates a new task in Remember The Milk.",
			InputSchema: mustMarshalJSON(map[string]interface{}{
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
			}),
		},
		// Tool: rtm/completeTask
		// TODO: Define completeTask tool later.
	}

	// Create the result containing the tool list.
	result := ListToolsResult{
		Tools: tools,
		// NextCursor can be added here for pagination if needed.
	}

	// Marshal the result.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListToolsResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ListToolsResult") // Internal error.
	}

	h.logger.Info("Handled tools/list request.", "toolsCount", len(result.Tools))
	return resultBytes, nil
}

// handleToolCall handles the tools/call request.
// Handles RTM tool names, returns placeholder results.
func (h *Handler) handleToolCall(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req CallToolRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Error parsing the request itself (should have been caught by validation).
		return nil, errors.Wrap(err, "invalid params structure for tools/call")
	}

	h.logger.Info("Handling tool/call request.", "toolName", req.Name)

	var callResult CallToolResult // Only need one variable now.

	// Route the call to the specific tool implementation placeholder.
	switch req.Name {
	case "rtm/getTasks":
		// Corrected: Assign single return value.
		callResult = h.executeRTMGetTasksPlaceholder(ctx, req.Arguments)
	case "rtm/createTask":
		// Corrected: Assign single return value.
		callResult = h.executeRTMCreateTaskPlaceholder(ctx, req.Arguments)
	// case "rtm/completeTask":
	// 	callResult = h.executeRTMCompleteTaskPlaceholder(ctx, req.Arguments) // Update when implemented.
	default:
		// Tool name sent by client is not recognized by the server.
		h.logger.Warn("Tool not found during tool/call.", "toolName", req.Name)
		callResult = CallToolResult{
			IsError: true,
			Content: []Content{
				TextContent{Type: "text", Text: "Error: Tool not found: " + req.Name},
			},
		}
	}

	// Marshal the CallToolResult (which might contain an error or placeholder success).
	resultBytes, marshalErr := json.Marshal(callResult)
	if marshalErr != nil {
		// This is an internal server error during result marshaling.
		h.logger.Error("Failed to marshal CallToolResult.", "toolName", req.Name, "error", marshalErr)
		// Return a wrapped internal error to be handled by createErrorResponse.
		return nil, errors.Wrap(marshalErr, "internal error: Failed to marshal CallToolResult")
	}

	// Return success at JSON-RPC level (resultBytes contains tool success/error details).
	return resultBytes, nil
}

// handleResourcesList handles the resources/list request. (Unchanged placeholder).
func (h *Handler) handleResourcesList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling resources/list request (currently placeholder).")
	result := ListResourcesResult{
		Resources: []Resource{}, // Return empty list for now.
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListResourcesResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ListResourcesResult")
	}
	return resultBytes, nil
}

// handleResourcesRead handles the resources/read request. (Unchanged placeholder).
func (h *Handler) handleResourcesRead(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req ReadResourceRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for resources/read")
	}
	h.logger.Info("Handling resources/read request (currently placeholder).", "uri", req.URI)
	// Return resource not found error using specific mcperrors type.
	return nil, mcperrors.NewResourceError("Resource not found: "+req.URI, nil, map[string]interface{}{"uri": req.URI})
}

// --- Tool Execution Logic Placeholders ---

// executeRTMGetTasksPlaceholder handles the rtm/getTasks tool call (enhanced placeholder).
func (h *Handler) executeRTMGetTasksPlaceholder(_ context.Context, args json.RawMessage) CallToolResult {
	var toolArgs struct {
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &toolArgs); err != nil {
		h.logger.Warn("Invalid arguments received for rtm/getTasks tool.", "error", err, "args", string(args))
		return CallToolResult{
			IsError: true,
			Content: []Content{TextContent{Type: "text", Text: "Error calling rtm/getTasks: Invalid arguments: " + err.Error()}},
		}
	}

	h.logger.Info("Executing rtm/getTasks tool with enhanced placeholder response.", "filter", toolArgs.Filter)

	// Return a more realistic mock response with fake tasks that match the filter
	return CallToolResult{
		IsError: false,
		Content: []Content{
			TextContent{Type: "text", Text: fmt.Sprintf("Successfully retrieved tasks matching filter: '%s'", toolArgs.Filter)},
			TextContent{Type: "text", Text: "Tasks:\n1. Write documentation for CowGnition (due: tomorrow, priority: 1)\n2. Test MCP integration (due: today, priority: 1)\n3. Implement RTM API client (due: next week, priority: 2)"},
		},
	}
}

// executeRTMCreateTaskPlaceholder handles the rtm/createTask tool call (enhanced placeholder).
func (h *Handler) executeRTMCreateTaskPlaceholder(_ context.Context, args json.RawMessage) CallToolResult {
	var toolArgs struct {
		Name string `json:"name"`
		List string `json:"list,omitempty"`
	}
	if err := json.Unmarshal(args, &toolArgs); err != nil {
		h.logger.Warn("Invalid arguments received for rtm/createTask tool.", "error", err, "args", string(args))
		return CallToolResult{
			IsError: true,
			Content: []Content{TextContent{Type: "text", Text: "Error calling rtm/createTask: Invalid arguments: " + err.Error()}},
		}
	}

	list := toolArgs.List
	if list == "" {
		list = "Inbox"
	}

	h.logger.Info("Executing rtm/createTask tool with enhanced placeholder response.", "name", toolArgs.Name, "list", list)

	// Return a more realistic mock response pretending the task was created
	return CallToolResult{
		IsError: false,
		Content: []Content{
			TextContent{Type: "text", Text: fmt.Sprintf("Successfully created task: '%s' in list '%s'", toolArgs.Name, list)},
			TextContent{Type: "text", Text: "Task Details:\nID: task_12345\nAdded: Just now\nURL: https://www.rememberthemilk.com/app/#list/inbox/task_12345"},
		},
	}
}

// --- Helper Functions ---

// mustMarshalJSON marshals v to JSON and panics on error. Used for static schemas.
func mustMarshalJSON(v interface{}) json.RawMessage {
	bytes, err := json.Marshal(v)
	if err != nil {
		// Panic is acceptable here because it indicates a programming error.
		// (invalid static schema definition) during initialization.
		panic(fmt.Sprintf("failed to marshal static JSON schema: %v", err))
	}
	return json.RawMessage(bytes)
}

// Add other tool execution functions placeholders here (e.g., executeRTMCompleteTaskPlaceholder).
