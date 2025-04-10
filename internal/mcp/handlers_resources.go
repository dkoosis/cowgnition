// file: internal/mcp/handlers_resources.go

package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
)

// Import schema

// handleResourcesList handles the resources/list request.
// Official definition: Sent from the client to request a list of resources the server has.
// The server should respond with information about resources that the client can access.
// handleResourcesList handles the resources/list request.
// Official definition: Sent from the client to request a list of resources the server has.
// The server should respond with information about resources that the client can access.
func (h *Handler) handleResourcesList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling resources/list request.")

	// Parse pagination cursor if provided.
	var listParams struct {
		Cursor string `json:"cursor,omitempty"`
	}
	if err := json.Unmarshal(params, &listParams); err != nil {
		// If we can't parse params, just ignore cursor and return first page.
		h.logger.Debug("Could not parse resources/list params.", "error", err)
	}

	// Create resources that represent RTM data AND server health.
	resources := []Resource{
		{
			Name:        "RTM Authentication Status",
			URI:         "auth://rtm",
			Description: "Provides the current authentication status with Remember The Milk (RTM).",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Lists",
			URI:         "rtm://lists",
			Description: "Lists available in your Remember The Milk account.",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Tags",
			URI:         "rtm://tags",
			Description: "Tags used in your Remember The Milk account.",
			MimeType:    "application/json",
		},
		// *** ADDED HEALTH RESOURCE ***
		{
			Name:        "Server Health Metrics",
			URI:         "cowgnition://health",
			Description: "Provides internal health and performance metrics for the CowGnition server.",
			MimeType:    "application/json",
		},
		// *** END ADDED HEALTH RESOURCE ***
	}

	// Handle pagination.
	var nextCursor string
	if listParams.Cursor != "" {
		nextCursor = ""
		// TODO: Implement actual cursor logic if needed.
	}

	result := ListResourcesResult{
		Resources:  resources,
		NextCursor: nextCursor,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListResourcesResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ListResourcesResult")
	}
	return resultBytes, nil
}

// handleResourcesRead handles the resources/read request.
// Official definition: Sent from the client to the server, to read a specific resource URI.
// The server responds with the contents of the resource.
func (h *Handler) handleResourcesRead(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req ReadResourceRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for resources/read")
	}
	h.logger.Info("Handling resources/read request.", "uri", req.URI)

	var contents []interface{}

	switch req.URI {
	case "auth://rtm":
		authStatus := map[string]interface{}{
			"isAuthenticated": true, // Placeholder
			"username":        "example_user",
			"accountType":     "Pro",
		}
		// *** FIX: Use standard marshal and handle error ***
		authBytes, marshalErr := json.MarshalIndent(authStatus, "", "  ")
		if marshalErr != nil {
			h.logger.Error("Failed to marshal auth status.", "error", marshalErr)
			return nil, errors.Wrap(marshalErr, "failed to marshal auth status")
		}
		authString := string(authBytes) // Convert bytes to string

		contents = append(contents, TextResourceContents{
			ResourceContents: ResourceContents{URI: req.URI, MimeType: "application/json"},
			Text:             authString, // *** FIX: Use the marshalled string ***
		})

	case "rtm://lists":
		lists := []map[string]interface{}{
			{"id": "1", "name": "Inbox", "taskCount": 5},
			{"id": "2", "name": "Work", "taskCount": 12},
			{"id": "3", "name": "Personal", "taskCount": 8},
		}
		// *** FIX: Use standard marshal and handle error ***
		listsBytes, marshalErr := json.MarshalIndent(lists, "", "  ")
		if marshalErr != nil {
			h.logger.Error("Failed to marshal RTM lists.", "error", marshalErr)
			return nil, errors.Wrap(marshalErr, "failed to marshal RTM lists")
		}
		listsString := string(listsBytes) // Convert bytes to string

		contents = append(contents, TextResourceContents{
			ResourceContents: ResourceContents{URI: req.URI, MimeType: "application/json"},
			Text:             listsString, // *** FIX: Use the marshalled string ***
		})

	case "rtm://tags":
		tags := []map[string]interface{}{
			{"name": "urgent", "taskCount": 3},
			{"name": "shopping", "taskCount": 2},
			{"name": "work", "taskCount": 7},
		}
		// *** FIX: Use standard marshal and handle error ***
		tagsBytes, marshalErr := json.MarshalIndent(tags, "", "  ")
		if marshalErr != nil {
			h.logger.Error("Failed to marshal RTM tags.", "error", marshalErr)
			return nil, errors.Wrap(marshalErr, "failed to marshal RTM tags")
		}
		tagsString := string(tagsBytes) // Convert bytes to string

		contents = append(contents, TextResourceContents{
			ResourceContents: ResourceContents{URI: req.URI, MimeType: "application/json"},
			Text:             tagsString, // *** FIX: Use the marshalled string ***
		})

	case "cowgnition://health":
		h.logger.Debug("Reading health metrics.")
		uptime := time.Since(h.startTime).Round(time.Second).String()
		schemaInitialized := h.validator.IsInitialized()
		schemaLoadDuration := h.validator.GetLoadDuration().String()
		schemaCompileDuration := h.validator.GetCompileDuration().String()
		currentState := h.connectionState.CurrentState()

		metrics := map[string]interface{}{
			"serverUptime":          uptime,
			"schemaInitialized":     schemaInitialized,
			"schemaLoadDuration":    schemaLoadDuration,
			"schemaCompileDuration": schemaCompileDuration,
			"connectionState":       currentState,
		}

		metricsBytes, marshalErr := json.MarshalIndent(metrics, "", "  ")
		if marshalErr != nil {
			h.logger.Error("Failed to marshal health metrics.", "error", marshalErr)
			return nil, errors.Wrap(marshalErr, "failed to marshal health metrics")
		}
		metricsString := string(metricsBytes) // Define metricsString

		contents = append(contents, TextResourceContents{
			ResourceContents: ResourceContents{
				URI:      req.URI,
				MimeType: "application/json",
			},
			Text: metricsString, // *** FIX: Use metricsString here ***
		})

	default:
		// Resource not found.
		h.logger.Warn("Resource URI not found.", "uri", req.URI)
		return nil, mcperrors.NewResourceError("Resource not found: "+req.URI, nil, map[string]interface{}{"uri": req.URI})
	}

	result := ReadResourceResult{
		Contents: contents,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ReadResourceResult.", "uri", req.URI, "error", err)
		return nil, errors.Wrap(err, "failed to marshal ReadResourceResult")
	}

	return resultBytes, nil
}

// handleResourcesSubscribe handles the resources/subscribe request.
// Official definition: Sent from the client to request resources/updated notifications
// from the server whenever a particular resource changes.
// nolint:unused,unparam
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
// nolint:unused,unparam
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
// nolint:unused,unparam
func (h *Handler) handleResourcesUpdated(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	var updateParams struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &updateParams); err != nil {
		h.logger.Warn("Invalid parameters for resources/updated notification.", "error", err)
		// Still return nil as this is a notification.
		return nil, nil
	}

	h.logger.Info("Resource updated notification received.", "uri", updateParams.URI)
	// No response needed for notifications.
	return nil, nil
}

// handleResourceListChanged handles the notifications/resources/list_changed notification.
// Official definition: An optional notification from the server to the client, informing
// it that the list of resources it can read from has changed. This may be issued by
// servers without any previous subscription from the client.
// nolint:unused,unparam
func (h *Handler) handleResourceListChanged(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Sending resource list changed notification to client.")
	// NOTE: This would typically be sent from the server to the client, not handled by the server.
	// Included here for completeness of the protocol implementation.
	return nil, nil
}
