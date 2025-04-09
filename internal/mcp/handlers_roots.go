// file: internal/mcp/handlers_roots.go

package mcp

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// handleRootsListChanged handles the notifications/roots/list_changed notification.
// Official definition: A notification from the client to the server, informing it that
// the list of roots has changed. This notification should be sent whenever the client adds,
// removes, or modifies any root. The server should then request an updated list of roots
// using the ListRootsRequest.
// nolint:unused,unparam
func (h *Handler) handleRootsListChanged(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Received roots/list_changed notification.")

	// This would typically trigger a roots/list request from the server to get updated roots.
	// For now, we just log the notification.

	// Notifications don't require a response.
	return nil, nil
}

// handleRootsList would handle a roots/list request from server to client.
// Official definition: Sent from the server to request a list of root URIs from the client.
// NOTE: This is typically a server-to-client request, but included here as a stub for completeness.
// nolint:unused,unparam
func (h *Handler) handleRootsList(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Warn("Received roots/list, which is a server-to-client request.")
	// This would typically be implemented by the client, not the server.
	return nil, errors.New("roots/list is a server-to-client request, not implemented by the server")
}
