// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/handlers_tools.go.

import (
	"context"
	"encoding/json"
	"fmt" // For placeholder formatting.

	"github.com/cockroachdb/errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import the shared types package.
)

// handleToolsList handles the tools/list request.
// Official definition: Used by the client to request a list of tools the server has.
// The server should respond with a list of Tool objects that describe the available tools.
func (h *Handler) handleToolsList(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling tools/list request.")

	// Define RTM tools.
	tools := []mcptypes.Tool{ // Use mcptypes.Tool.
		// Tool: getTasks.
		{
			Name:        "getTasks",
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
			// Optional annotations to provide additional information to clients.
			Annotations: &mcptypes.ToolAnnotations{ // Use mcptypes.ToolAnnotations.
				Title:        "Get RTM Tasks",
				ReadOnlyHint: true, // This tool doesn't modify any data.
			},
		},
		// Tool: createTask.
		{
			Name:        "createTask",
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
			Annotations: &mcptypes.ToolAnnotations{ // Use mcptypes.ToolAnnotations.
				Title:           "Create RTM Task",
				ReadOnlyHint:    false, // This tool modifies data.
				DestructiveHint: false, // It's not destructive, just additive.
				IdempotentHint:  false, // Multiple calls with same args will create multiple tasks.
			},
		},
		// Tool: completeTask.
		{
			Name:        "completeTask",
			Description: "Marks a task as complete in Remember The Milk.",
			InputSchema: mustMarshalJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"taskId": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the task to mark as complete.",
					},
				},
				"required": []string{"taskId"},
			}),
			Annotations: &mcptypes.ToolAnnotations{ // Use mcptypes.ToolAnnotations.
				Title:           "Complete RTM Task",
				ReadOnlyHint:    false, // This tool modifies data.
				DestructiveHint: true,  // It changes the state of a task.
				IdempotentHint:  true,  // Multiple calls with same taskId will have same effect.
			},
		},
	}

	// Create the result containing the tool list.
	result := mcptypes.ListToolsResult{ // Use mcptypes.ListToolsResult.
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
// Official definition: Used by the client to invoke a tool provided by the server.
// The server executes the requested tool with the provided arguments and returns
// the result. If a tool execution fails, it should be reflected in the isError field
// of the result, not as a protocol-level error.
func (h *Handler) handleToolCall(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req mcptypes.CallToolRequest // Use mcptypes.CallToolRequest.
	if err := json.Unmarshal(params, &req); err != nil {
		// Error parsing the request itself (should have been caught by validation).
		return nil, errors.Wrap(err, "invalid params structure for tools/call")
	}

	h.logger.Info("Handling tool/call request.", "toolName", req.Name)

	var callResult mcptypes.CallToolResult // Only need one variable now. Use mcptypes.CallToolResult.

	// Route the call to the specific tool implementation placeholder.
	switch req.Name {
	case "getTasks":
		// Corrected: Assign single return value.
		callResult = h.executeRTMGetTasksPlaceholder(ctx, req.Arguments)
	case "createTask":
		// Corrected: Assign single return value.
		callResult = h.executeRTMCreateTaskPlaceholder(ctx, req.Arguments)
	case "completeTask":
		callResult = h.executeRTMCompleteTaskPlaceholder(ctx, req.Arguments)
	default:
		// Tool name sent by client is not recognized by the server.
		h.logger.Warn("Tool not found during tool/call.", "toolName", req.Name)
		callResult = mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
			IsError: true,
			Content: []mcptypes.Content{ // Use mcptypes.Content.
				mcptypes.TextContent{Type: "text", Text: "Error: Tool not found: " + req.Name}, // Use mcptypes.TextContent.
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

// handleToolListChanged handles the notifications/tools/list_changed notification.
// Official definition: An optional notification from the server to the client, informing
// it that the list of tools it offers has changed. This may be issued by servers
// without any previous subscription from the client.
// nolint:unused,unparam.
func (h *Handler) handleToolListChanged(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Sending tool list changed notification to client.")
	// NOTE: This would typically be sent from the server to the client, not handled by the server.
	// Included here for completeness of the protocol implementation.
	return nil, nil
}

// ------ TOOL EXECUTION LOGIC PLACEHOLDERS ------.

// executeRTMGetTasksPlaceholder handles the getTasks tool call (enhanced placeholder).
func (h *Handler) executeRTMGetTasksPlaceholder(_ context.Context, args json.RawMessage) mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	var toolArgs struct {
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &toolArgs); err != nil {
		h.logger.Warn("Invalid arguments received for getTasks tool.", "error", err, "args", string(args))
		return mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
			IsError: true,
			Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: "Error calling getTasks: Invalid arguments: " + err.Error()}}, // Use mcptypes.Content, mcptypes.TextContent.
		}
	}

	h.logger.Info("Executing getTasks tool with enhanced placeholder response.", "filter", toolArgs.Filter)

	// TODO: Replace with actual RTM API call using h.rtmClient.
	// Return a more realistic mock response with fake tasks that match the filter.
	return mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
		IsError: false,
		Content: []mcptypes.Content{ // Use mcptypes.Content.
			mcptypes.TextContent{Type: "text", Text: fmt.Sprintf("Successfully retrieved tasks matching filter: '%s'.", toolArgs.Filter)},                                                                                                         // Use mcptypes.TextContent.
			mcptypes.TextContent{Type: "text", Text: "Tasks:\n1. Write documentation for CowGnition (due: tomorrow, priority: 1)\n2. Test MCP integration (due: today, priority: 1)\n3. Implement RTM API client (due: next week, priority: 2)."}, // Placeholder data. Use mcptypes.TextContent.
		},
	}
}

// executeRTMCreateTaskPlaceholder handles the createTask tool call (enhanced placeholder).
func (h *Handler) executeRTMCreateTaskPlaceholder(_ context.Context, args json.RawMessage) mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	var toolArgs struct {
		Name string `json:"name"`
		List string `json:"list,omitempty"`
	}
	if err := json.Unmarshal(args, &toolArgs); err != nil {
		h.logger.Warn("Invalid arguments received for createTask tool.", "error", err, "args", string(args))
		return mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
			IsError: true,
			Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: "Error calling createTask: Invalid arguments: " + err.Error()}}, // Use mcptypes.Content, mcptypes.TextContent.
		}
	}

	list := toolArgs.List
	if list == "" {
		list = "Inbox" // Default list.
	}

	h.logger.Info("Executing createTask tool with enhanced placeholder response.", "name", toolArgs.Name, "list", list)

	// TODO: Replace with actual RTM API call using h.rtmClient.
	// Return a more realistic mock response pretending the task was created.
	return mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
		IsError: false,
		Content: []mcptypes.Content{ // Use mcptypes.Content.
			mcptypes.TextContent{Type: "text", Text: fmt.Sprintf("Successfully created task: '%s' in list '%s'.", toolArgs.Name, list)},                                  // Use mcptypes.TextContent.
			mcptypes.TextContent{Type: "text", Text: "Task Details:\nID: task_12345\nAdded: Just now\nURL: https://www.rememberthemilk.com/app/#list/inbox/task_12345."}, // Placeholder data. Use mcptypes.TextContent.
		},
	}
}

// executeRTMCompleteTaskPlaceholder handles the completeTask tool call.
func (h *Handler) executeRTMCompleteTaskPlaceholder(_ context.Context, args json.RawMessage) mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	var toolArgs struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(args, &toolArgs); err != nil {
		h.logger.Warn("Invalid arguments received for completeTask tool.", "error", err, "args", string(args))
		return mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
			IsError: true,
			Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: "Error calling completeTask: Invalid arguments: " + err.Error()}}, // Use mcptypes.Content, mcptypes.TextContent.
		}
	}

	h.logger.Info("Executing completeTask tool with placeholder response.", "taskId", toolArgs.TaskID)

	// TODO: Replace with actual RTM API call.
	// Return a mock response for completing the task.
	return mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
		IsError: false,
		Content: []mcptypes.Content{ // Use mcptypes.Content.
			mcptypes.TextContent{Type: "text", Text: fmt.Sprintf("Successfully completed task with ID: %s.", toolArgs.TaskID)}, // Use mcptypes.TextContent.
		},
	}
}
