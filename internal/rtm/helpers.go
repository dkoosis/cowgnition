// file: internal/rtm/helpers.go
// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

import (
	"encoding/json"
	"fmt"

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import the shared types package.
	// Ensure strings is imported if needed (it is by other files in package).
	// "strings"
)

// --- Tool Result Helpers ---

// successToolResult creates a standard successful tool result with text content.
// --- FIX: Use mcptypes prefix ---
func (s *Service) successToolResult(text string) *mcptypes.CallToolResult {
	return &mcptypes.CallToolResult{
		IsError: false,
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: text}},
	}
}

// simpleToolErrorResult creates a standard error tool result with simple text message.
// --- FIX: Use mcptypes prefix ---
func (s *Service) simpleToolErrorResult(errorMessage string) *mcptypes.CallToolResult {
	return &mcptypes.CallToolResult{
		IsError: true,
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: errorMessage}},
	}
}

// invalidToolArgumentsError creates a result for invalid tool arguments.
// --- FIX: Use mcptypes prefix ---
func (s *Service) invalidToolArgumentsError(toolName string, err error) *mcptypes.CallToolResult {
	msg := fmt.Sprintf("Invalid arguments for tool '%s': %v.", toolName, err)
	s.logger.Warn(msg, "toolName", toolName, "error", err) // Log the error too.
	return s.simpleToolErrorResult(msg)
}

// rtmAPIErrorResult creates a result for errors returned from the RTM API client.
// --- FIX: Use mcptypes prefix ---
func (s *Service) rtmAPIErrorResult(action string, err error) *mcptypes.CallToolResult {
	msg := fmt.Sprintf("Error %s: %v.", action, err)
	// Error should have been logged in the client, but maybe log here too?.
	// s.logger.Warn("RTM API call failed.", "action", action, "error", err).
	return s.simpleToolErrorResult(msg)
}

// notAuthenticatedError creates a result for when authentication is required but missing.
// --- FIX: Use mcptypes prefix ---
func (s *Service) notAuthenticatedError() *mcptypes.CallToolResult {
	return s.simpleToolErrorResult("Not authenticated with Remember The Milk. Use 'rtm_getAuthStatus' or 'rtm_authenticate' tool.")
}

// serviceNotInitializedError creates a result when the service hasn't been initialized.
// --- FIX: Use mcptypes prefix ---
func (s *Service) serviceNotInitializedError() *mcptypes.CallToolResult {
	return s.simpleToolErrorResult("RTM service is not initialized.")
}

// unknownToolError creates a result for when an unknown tool is called.
// --- FIX: Use mcptypes prefix ---
func (s *Service) unknownToolError(toolName string) *mcptypes.CallToolResult {
	return s.simpleToolErrorResult(fmt.Sprintf("Unknown RTM tool requested: %s.", toolName))
}

// internalToolError creates a result for unexpected internal errors during tool handling.
// --- FIX: Use mcptypes prefix ---
func (s *Service) internalToolError() *mcptypes.CallToolResult {
	// Logged previously in CallTool.
	return s.simpleToolErrorResult("An internal error occurred while executing the tool.")
}

// --- Resource Content Helpers ---

// createJSONResourceContent marshals data to JSON and wraps it in TextResourceContents.
// Returns an internal error if marshalling fails.
// --- FIX: Use mcptypes prefix ---
func (s *Service) createJSONResourceContent(uri string, data interface{}) ([]interface{}, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		s.logger.Error("Failed to marshal resource content to JSON.", "uri", uri, "error", err)
		// This is an internal server error, should be propagated.
		return nil, fmt.Errorf("internal error: failed to marshal JSON for resource %s: %w", uri, err)
	}
	return []interface{}{
		mcptypes.TextResourceContents{
			ResourceContents: mcptypes.ResourceContents{URI: uri, MimeType: "application/json"},
			Text:             string(jsonData),
		},
	}, nil
}

// notAuthenticatedResourceContent creates the content payload for unauthenticated resource access.
func (s *Service) notAuthenticatedResourceContent(uri string) []interface{} {
	content := map[string]interface{}{
		"error":   "not_authenticated",
		"message": "Not authenticated with Remember The Milk. Use MCP tools to authenticate.",
	}
	contentJSON, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		s.logger.Error("Failed to marshal 'not authenticated' resource content.", "error", err)
		// Fallback to plain text.
		return []interface{}{
			// --- FIX: Use mcptypes prefix ---
			mcptypes.TextResourceContents{
				ResourceContents: mcptypes.ResourceContents{URI: uri, MimeType: "text/plain"},
				Text:             "Error: Not Authenticated.",
			},
		}
	}
	return []interface{}{
		// --- FIX: Use mcptypes prefix ---
		mcptypes.TextResourceContents{
			ResourceContents: mcptypes.ResourceContents{URI: uri, MimeType: "application/json"},
			Text:             string(contentJSON),
		},
	}
}

// --- Schema Definition Helpers ---
// --- FIX: Add missing helper functions ---

// mustMarshalJSON marshals v to JSON and panics on error. Used for static schemas.
// NOTE: This is also defined in internal/mcp/helpers.go. Consider moving to a shared utility package if needed elsewhere.
func mustMarshalJSON(v interface{}) json.RawMessage {
	bytes, err := json.Marshal(v)
	if err != nil {
		// Panic is acceptable here because it indicates a programming error
		// (invalid static schema definition) during initialization.
		panic(fmt.Sprintf("failed to marshal static JSON schema: %v", err))
	}
	return json.RawMessage(bytes)
}

// emptyInputSchema returns a schema for tools that take no input arguments.
func (s *Service) emptyInputSchema() json.RawMessage {
	return mustMarshalJSON(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
}

// getTasksInputSchema defines the input schema for the getTasks tool.
func (s *Service) getTasksInputSchema() json.RawMessage {
	return mustMarshalJSON(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Optional RTM filter expression (e.g., 'list:Inbox status:incomplete dueBefore:tomorrow'). See RTM documentation for filter syntax. If omitted, returns tasks from the default view.",
			},
		},
		// "required": []string{}, // Filter is optional
	})
}

// createTaskInputSchema defines the input schema for the createTask tool.
func (s *Service) createTaskInputSchema() json.RawMessage {
	return mustMarshalJSON(map[string]interface{}{
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
	})
}

// completeTaskInputSchema defines the input schema for the completeTask tool.
func (s *Service) completeTaskInputSchema() json.RawMessage {
	return mustMarshalJSON(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"taskId": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the task to mark as complete (e.g., '12345_67890').",
			},
			"listId": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the list the task belongs to.",
			},
		},
		"required": []string{"taskId", "listId"},
	})
}

// authenticationInputSchema defines the input schema for the authenticate tool.
func (s *Service) authenticationInputSchema() json.RawMessage {
	return mustMarshalJSON(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"frob": map[string]interface{}{
				"type":        "string",
				"description": "Optional. The 'frob' code obtained after visiting the RTM authentication URL in a browser. Provide this to complete the authentication flow.",
			},
		},
		// "required": []string{}, // frob is optional
	})
}

// --- END FIX ---
