// file: internal/mcp/handlers_prompts.go

package mcp

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// handlePromptsList handles the prompts/list request.
// Official definition: Sent from the client to request a list of prompts and prompt templates the server has.
// The server responds with information about prompts that the client can access.
func (h *Handler) handlePromptsList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling prompts/list request.")

	// Parse pagination cursor if provided.
	var listParams struct {
		Cursor string `json:"cursor,omitempty"`
	}
	if err := json.Unmarshal(params, &listParams); err != nil {
		// If we can't parse params, just ignore cursor and return first page.
		h.logger.Debug("Could not parse prompts/list params.", "error", err)
	}

	// For now, return an empty list of prompts.
	// In the future, this would be populated with actual prompt templates
	// from a configuration file or database.
	prompts := []Prompt{}

	// Handle pagination (if any prompts are added in the future).
	var nextCursor string
	if listParams.Cursor != "" {
		// For now, we don't have pagination, so return empty nextCursor.
		nextCursor = ""
	}

	result := ListPromptsResult{
		Prompts:    prompts,
		NextCursor: nextCursor,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListPromptsResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ListPromptsResult")
	}

	h.logger.Info("Handled prompts/list request.", "promptsCount", len(result.Prompts))
	return resultBytes, nil
}

// handlePromptsGet handles the prompts/get request.
// Official definition: Used by the client to get a prompt provided by the server.
// It retrieves a specific prompt by name, optionally with arguments for templating.
func (h *Handler) handlePromptsGet(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		h.logger.Warn("Invalid parameters for prompts/get request.", "error", err)
		return nil, errors.Wrap(err, "invalid params for prompts/get")
	}

	h.logger.Info("Handling prompts/get request.", "name", req.Name, "argumentCount", len(req.Arguments))

	// Currently, we don't support any prompts, so return a not found error.
	// This error will be mapped to an appropriate JSON-RPC error by the server.
	return nil, errors.Newf("prompt not found: %s", req.Name)
}

// handlePromptsListChanged handles the notifications/prompts/list_changed notification.
// Official definition: An optional notification from the server to the client, informing it
// that the list of prompts it offers has changed. This may be issued by servers without
// any previous subscription from the client.
func (h *Handler) handlePromptsListChanged(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Sending prompts list changed notification to client.")

	// This would typically be sent from the server to the client, not handled by the server.
	// Included here for completeness of the MCP protocol implementation.
	// No response is needed for notifications.
	return nil, nil
}
