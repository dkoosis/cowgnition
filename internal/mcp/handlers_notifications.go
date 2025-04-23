// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/handlers_notifications.go.
package mcp

import (
	"context"
	"encoding/json"
	"fmt" // Import fmt for formatting.

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import the shared types package.
)

// handleNotificationsInitialized handles the notifications/initialized notification.
// Official definition: This notification is sent from the client to the server after
// initialization has finished. It signals that the client has successfully processed
// the server's initialization response and is ready for further communication.
// nolint:unused,unparam.
func (h *Handler) handleNotificationsInitialized(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Received 'notifications/initialized' from client.")

	// Extract client capabilities if available.
	var notifParams struct {
		ClientCapabilities *mcptypes.ClientCapabilities `json:"clientCapabilities,omitempty"` // Use mcptypes.ClientCapabilities.
	}
	if err := json.Unmarshal(params, &notifParams); err != nil {
		// It's okay if we can't unmarshal, the params might be empty.
		h.logger.Debug("Could not parse notifications/initialized params (might be empty).", "error", err)
	}

	if notifParams.ClientCapabilities != nil {
		h.logger.Debug("Client capabilities confirmed during initialized.",
			"capabilities", fmt.Sprintf("%+v", notifParams.ClientCapabilities))
	}

	// This is a notification (no response needed).
	return nil, nil
}

// handleNotificationsCancelled handles the notifications/cancelled notification.
// Official definition: This notification can be sent by either side to indicate that it is
// cancelling a previously-issued request. The request SHOULD still be in-flight, but due to
// communication latency, it is always possible that this notification MAY arrive after
// the request has already finished.
// nolint:unused,unparam.
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
// nolint:unused,unparam.
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
