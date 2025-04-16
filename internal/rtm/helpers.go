// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/helpers.go

import (
	"encoding/json"
	"fmt"

	"github.com/dkoosis/cowgnition/internal/mcp"
)

// --- Tool Result Helpers ---

// successToolResult creates a standard successful tool result with text content.
func (s *Service) successToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
	}
}

// simpleToolErrorResult creates a standard error tool result with simple text message.
func (s *Service) simpleToolErrorResult(errorMessage string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: errorMessage}},
	}
}

// invalidToolArgumentsError creates a result for invalid tool arguments.
func (s *Service) invalidToolArgumentsError(toolName string, err error) *mcp.CallToolResult {
	msg := fmt.Sprintf("Invalid arguments for tool '%s': %v.", toolName, err)
	s.logger.Warn(msg, "toolName", toolName, "error", err) // Log the error too
	return s.simpleToolErrorResult(msg)
}

// rtmApiErrorResult creates a result for errors returned from the RTM API client.
func (s *Service) rtmApiErrorResult(action string, err error) *mcp.CallToolResult {
	msg := fmt.Sprintf("Error %s: %v.", action, err)
	// Error should have been logged in the client, but maybe log here too?
	// s.logger.Warn("RTM API call failed.", "action", action, "error", err)
	return s.simpleToolErrorResult(msg)
}

// notAuthenticatedError creates a result for when authentication is required but missing.
func (s *Service) notAuthenticatedError() *mcp.CallToolResult {
	return s.simpleToolErrorResult("Not authenticated with Remember The Milk. Use 'getAuthStatus' or 'authenticate' tool.")
}

// serviceNotInitializedError creates a result when the service hasn't been initialized.
func (s *Service) serviceNotInitializedError() *mcp.CallToolResult {
	return s.simpleToolErrorResult("RTM service is not initialized.")
}

// unknownToolError creates a result for when an unknown tool is called.
func (s *Service) unknownToolError(toolName string) *mcp.CallToolResult {
	return s.simpleToolErrorResult(fmt.Sprintf("Unknown RTM tool requested: %s", toolName))
}

// internalToolError creates a result for unexpected internal errors during tool handling.
func (s *Service) internalToolError() *mcp.CallToolResult {
	// Logged previously in CallTool
	return s.simpleToolErrorResult("An internal error occurred while executing the tool.")
}

// --- Resource Content Helpers ---

// notAuthenticatedResourceContent creates the content payload for unauthenticated resource access.
func (s *Service) notAuthenticatedResourceContent(uri string) []interface{} {
	content := map[string]interface{}{
		"error":   "not_authenticated",
		"message": "Not authenticated with Remember The Milk. Use MCP tools to authenticate.",
	}
	contentJSON, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		s.logger.Error("Failed to marshal 'not authenticated' resource content.", "error", err)
		// Fallback to plain text
		return []interface{}{
			mcp.TextResourceContents{
				ResourceContents: mcp.ResourceContents{URI: uri, MimeType: "text/plain"},
				Text:             "Error: Not Authenticated.",
			},
		}
	}
	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{URI: uri, MimeType: "application/json"},
			Text:             string(contentJSON),
		},
	}
}
