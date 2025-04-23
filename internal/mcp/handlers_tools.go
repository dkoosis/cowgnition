// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/handlers_tools.go
// This file contains handlers for tool-related MCP methods.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Use mcptypes alias.
)

// --- Core Tool Handlers (Potentially used by routeMessage in the future) ---.

// handleToolsList handles the 'tools/list' request.
// This handler is currently UNUSED because routeMessage calls s.handleListTools which aggregates directly from services.
// Keep for potential future routing changes.
func (h *Handler) handleToolsList(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling tools/list request (currently unused path).")

	// If this were used, it might list tools defined statically or configured differently,
	// instead of relying solely on registered services.
	// For now, return an empty list to avoid errors if somehow called.

	// TODO: Define static/core tools if any (e.g., a help tool?).
	coreTools := []mcptypes.Tool{} // Empty for now.

	result := mcptypes.ListToolsResult{
		Tools: coreTools,
		// NextCursor: "", // Pagination not implemented here.
	}

	resBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal empty tools list.", "error", err)
		// Return internal error consistent with MCP spec.
		return nil, errors.Wrap(err, "internal error marshalling tool list")
	}

	return resBytes, nil
}

// handleToolCall handles the 'tools/call' request.
// This handler is currently UNUSED because routeMessage calls s.handleServiceDelegation which routes to services.
// Keep for potential future routing changes or handling non-service tools.
func (h *Handler) handleToolCall(_ context.Context, params json.RawMessage) (json.RawMessage, error) { // Renamed ctx to _.
	h.logger.Info("Handling tools/call request (currently unused path).")

	var req mcptypes.CallToolRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Use mcperrors constants. Ensure mcperrors package defines NewInvalidParamsError.
		// return nil, mcperrors.NewInvalidParamsError("invalid params structure for tools/call", err, nil).
		// Temporary: Return generic protocol error until mcperrors integrated.
		return nil, fmt.Errorf("invalid params structure for tools/call: %w", err) // Use fmt.Errorf.
	}

	h.logger.Warn("tools/call handler invoked, but no non-service tools are currently defined.", "toolName", req.Name)

	// If this handler were active, it would look for tools NOT handled by services.
	// switch req.Name {
	// case "core_help":
	//     // ... handle help tool ...
	// default:
	//     // Return method not found if not handled by services either.
	// }

	// For now, return an error indicating the tool wasn't found via this path.
	// Use mcperrors constants. Ensure mcperrors package defines NewMethodNotFoundError.
	// return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Tool not found via core handler: %s", req.Name), nil, map[string]interface{}{"toolName": req.Name}).
	// Temporary: Return generic protocol error.
	// Corrected capitalization for ST1005.
	return nil, fmt.Errorf("tool not found via core handler: %s", req.Name)
}

// --- Placeholder Tool Executors (REMOVED) ---.
// func (h *Handler) executeRTMGetTasksPlaceholder(...) { ... }
// func (h *Handler) executeRTMCreateTaskPlaceholder(...) { ... }
// func (h *Handler) executeRTMCompleteTaskPlaceholder(...) { ... }
