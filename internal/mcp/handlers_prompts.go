// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/handlers_prompts.go

// --- All functions previously here were removed due to being unused. ---

// handlePromptsListChanged handles the notifications/prompts/list_changed notification.
// Official definition: An optional notification from the server to the client, informing it
// that the list of prompts it offers has changed. This may be issued by servers without
// any previous subscription from the client.
// nolint:unused,unparam // Keeping function signature as-is, suppressing linter.
/* // Keep function signature commented out for reference if needed later.
func (h *Handler) handlePromptsListChanged(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Sending prompts list changed notification to client.")

	// This would typically be sent from the server to the client, not handled by the server.
	// Included here for completeness of the MCP protocol implementation.
	// No response is needed for notifications.
	return nil, nil
}
*/
