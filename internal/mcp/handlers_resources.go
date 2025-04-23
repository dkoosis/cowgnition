// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/handlers_resources.go.

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
	// Import the shared types package.
	// Keep schema import for ValidatorInterface.
)

// --- handleResourcesList REMOVED (unused) ---

// --- handleResourcesRead REMOVED (unused) ---

// handleResourcesSubscribe handles the resources/subscribe request.
// Official definition: Sent from the client to request resources/updated notifications
// from the server whenever a particular resource changes.
// nolint:unused,unparam.
func (h *Handler) handleResourcesSubscribe(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for resources/subscribe")
	}

	h.logger.Info("Handling resources/subscribe request.", "uri", req.URI)

	// In a real implementation, would store this subscription for later notifications.
	// TODO: Implement subscription storage mechanism.

	// Return empty result for success.
	result := map[string]interface{}{}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal resources/subscribe result")
	}

	return resultBytes, nil
}

// handleResourcesUnsubscribe handles the resources/unsubscribe request.
// Official definition: Sent from the client to request cancellation of resources/updated
// notifications from the server. This should follow a previous resources/subscribe request.
// nolint:unused,unparam.
func (h *Handler) handleResourcesUnsubscribe(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for resources/unsubscribe")
	}

	h.logger.Info("Handling resources/unsubscribe request.", "uri", req.URI)

	// In a real implementation, would remove this subscription.
	// TODO: Implement subscription removal.

	// Return empty result for success.
	result := map[string]interface{}{}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal resources/unsubscribe result")
	}

	return resultBytes, nil
}

// handleResourcesUpdated handles the notifications/resources/updated notification.
// Official definition: A notification from the server to the client, informing it that
// a resource has changed and may need to be read again. This should only be sent if
// the client previously sent a resources/subscribe request.
// Corrected: Changed return type to void.
//
//nolint:unused // Reserved for future server-sent notification implementation.
func (h *Handler) handleResourcesUpdated(_ context.Context, params json.RawMessage) {
	var updateParams struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &updateParams); err != nil {
		h.logger.Warn("Invalid parameters for resources/updated notification.", "error", err)
		return // Nothing to return.
	}

	h.logger.Info("Resource updated notification received.", "uri", updateParams.URI)
	// No response needed for notifications.
}

// handleResourceListChanged handles the notifications/resources/list_changed notification.
// Official definition: An optional notification from the server to the client, informing
// it that the list of resources it can read from has changed. This may be issued by
// servers without any previous subscription from the client.
// Corrected: Changed return type to void.
//
//nolint:unused // Reserved for future server-sent notification implementation.
func (h *Handler) handleResourceListChanged(_ context.Context, _ json.RawMessage) {
	h.logger.Info("Sending resource list changed notification to client.")
	// NOTE: This would typically be sent from the server to the client, not handled by the server.
	// Included here for completeness of the protocol implementation.
}
