// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// file: internal/rtm/helpers.go.
package rtm

import (
	"encoding/json"
	"fmt"

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import the shared types package.
	// Ensure strings is imported.
)

// --- Tool Result Helpers ---.

// successToolResult creates a standard successful tool result with text content.
func (s *Service) successToolResult(text string) *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	return &mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
		IsError: false,
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: text}}, // Use mcptypes.Content, mcptypes.TextContent.
	}
}

// simpleToolErrorResult creates a standard error tool result with simple text message.
func (s *Service) simpleToolErrorResult(errorMessage string) *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	return &mcptypes.CallToolResult{ // Use mcptypes.CallToolResult.
		IsError: true,
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: errorMessage}}, // Use mcptypes.Content, mcptypes.TextContent.
	}
}

// invalidToolArgumentsError creates a result for invalid tool arguments.
func (s *Service) invalidToolArgumentsError(toolName string, err error) *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	msg := fmt.Sprintf("Invalid arguments for tool '%s': %v.", toolName, err)
	s.logger.Warn(msg, "toolName", toolName, "error", err) // Log the error too.
	return s.simpleToolErrorResult(msg)
}

// rtmAPIErrorResult creates a result for errors returned from the RTM API client.
func (s *Service) rtmAPIErrorResult(action string, err error) *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	msg := fmt.Sprintf("Error %s: %v.", action, err)
	// Error should have been logged in the client, but maybe log here too?.
	// s.logger.Warn("RTM API call failed.", "action", action, "error", err).
	return s.simpleToolErrorResult(msg)
}

// notAuthenticatedError creates a result for when authentication is required but missing.
func (s *Service) notAuthenticatedError() *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	return s.simpleToolErrorResult("Not authenticated with Remember The Milk. Use 'getAuthStatus' or 'authenticate' tool.")
}

// serviceNotInitializedError creates a result when the service hasn't been initialized.
func (s *Service) serviceNotInitializedError() *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	return s.simpleToolErrorResult("RTM service is not initialized.")
}

// unknownToolError creates a result for when an unknown tool is called.
func (s *Service) unknownToolError(toolName string) *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	return s.simpleToolErrorResult(fmt.Sprintf("Unknown RTM tool requested: %s.", toolName))
}

// internalToolError creates a result for unexpected internal errors during tool handling.
func (s *Service) internalToolError() *mcptypes.CallToolResult { // Use mcptypes.CallToolResult.
	// Logged previously in CallTool.
	return s.simpleToolErrorResult("An internal error occurred while executing the tool.")
}

// --- Resource Content Helpers ---.

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
			mcptypes.TextResourceContents{ // Use mcptypes.TextResourceContents.
				ResourceContents: mcptypes.ResourceContents{URI: uri, MimeType: "text/plain"}, // Use mcptypes.ResourceContents.
				Text:             "Error: Not Authenticated.",
			},
		}
	}
	return []interface{}{
		mcptypes.TextResourceContents{ // Use mcptypes.TextResourceContents.
			ResourceContents: mcptypes.ResourceContents{URI: uri, MimeType: "application/json"}, // Use mcptypes.ResourceContents.
			Text:             string(contentJSON),
		},
	}
}

// --- General Helpers ---.

// truncateString truncates a string to a max length for previews.
// Defined ONCE here.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Ensure maxLen is not negative before slicing.
	if maxLen < 0 {
		maxLen = 0
	}
	// Consider runes if dealing with multi-byte characters, but for simple previews byte slicing is often ok.
	return s[:maxLen] + "..."
}
