// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/handlers_notifications.go.
package mcp

import (
	"context"
	"encoding/json"
	// Import fmt for formatting.
	// Import the shared types package.
)

// handleNotificationsCancelled handles the notifications/cancelled notification.
// Official definition: This notification can be sent by either side to indicate that it is
// cancelling a previously-issued request. The request SHOULD still be in-flight, but due to
// communication latency, it is always possible that this notification MAY arrive after
// the request has already finished.
// nolint:unused,unparam // Keeping function signature as-is, suppressing linter.
func (h *Handler) handleNotificationsCancelled(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	var cancelParams struct {
		RequestID interface{} `json:"requestId"`
		Reason    string      `json:"reason,omitempty"`
	}

	if err := json.Unmarshal(params, &cancelParams); err != nil {
		h.logger.Warn("Could not parse notifications/cancelled params.", "error", err)
		// Continue even if we can't parse the params.
	}

	h.logger.Info("Received request cancellation notification.",
		"requestID", cancelParams.RequestID,
		"reason", cancelParams.Reason)

	// TODO: Implement actual cancellation logic for long-running operations.

	// Notifications don't require a response.
	return nil, nil
}

// handleNotificationsProgress handles the notifications/progress notification.
// Official definition: An out-of-band notification used to inform the receiver of a
// progress update for a long-running request.
// nolint:unused,unparam // Keeping function signature as-is, suppressing linter.
func (h *Handler) handleNotificationsProgress(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	var progressParams struct {
		ProgressToken interface{} `json:"progressToken"`
		Progress      float64     `json:"progress"`
		Total         float64     `json:"total,omitempty"`
		Message       string      `json:"message,omitempty"`
	}

	if err := json.Unmarshal(params, &progressParams); err != nil {
		h.logger.Warn("Could not parse notifications/progress params.", "error", err)
	}

	h.logger.Info("Received progress notification.",
		"progressToken", progressParams.ProgressToken,
		"progress", progressParams.Progress,
		"total", progressParams.Total,
		"message", progressParams.Message)

	// This could be used to update UI elements or trigger other actions.

	// Notifications don't require a response.
	return nil, nil
}
