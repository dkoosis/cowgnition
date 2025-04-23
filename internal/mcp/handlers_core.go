// file: internal/mcp/handlers_core.go
// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// This file contains handlers for core MCP methods like initialize, shutdown, ping.
package mcp

import (
	"context"
	"encoding/json"

	// Required for logging format
	// Keep time import
	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config" // Keep config import

	// Keep logging import
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import mcp_types for shared types
	// REMOVED: "github.com/dkoosis/cowgnition/version". Use config directly.
)

// handlePing handles the "ping" request.
// Returns an empty object as result, signifying success.
// This handler function signature might change/be removed when routing is fully refactored.
func (h *Handler) handlePing(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Debug("Handling ping request.")
	return json.RawMessage(`{}`), nil
}

// handleInitialize handles the "initialize" request.
// It performs capability negotiation and sets the connection state.
// This handler function signature might change/be removed when routing is fully refactored.
func (h *Handler) handleInitialize(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling initialize request.")

	// Use mcptypes.InitializeRequest
	var req mcptypes.InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal initialize request parameters")
	}

	// --- FIX: Check if ConnectionState exists before using ---
	if h.connectionState == nil {
		h.logger.Error("Internal error: connectionState is nil in handleInitialize.")
		// Decide how to handle this - maybe return an internal error?
		// For now, log and continue cautiously.
	} else {
		// Safely call methods only if connectionState is not nil
		h.connectionState.SetClientInfo(req.ClientInfo)
		h.connectionState.SetClientCapabilities(req.Capabilities)
		h.connectionState.SetInitialized() // Mark connection as initialized.
	}
	// --- END FIX ---

	h.logger.Info("Client capabilities received.",
		"clientInfo", req.ClientInfo,
		"capabilities", req.Capabilities)

	// Log received protocol version for debugging.
	if req.ProtocolVersion != "" {
		h.logger.Info("Client requested protocol version.", "version", req.ProtocolVersion)
	} else {
		h.logger.Warn("Client did not specify a protocol version in initialize request.")
	}

	// Interim Fix: Force protocol version to schema version known to work with Claude Desktop.
	// See: docs/TODO.md - Protocol Version Handling.
	serverProtocolVersion := "2024-11-05"
	h.logger.Warn("Forcing server protocol version.",
		"serverVersion", serverProtocolVersion,
		"reason", "Compatibility fix for clients like Claude Desktop lacking standard version field")

	// Define server capabilities (should ideally be dynamic based on registered services).
	// Use mcptypes.ServerCapabilities
	caps := mcptypes.ServerCapabilities{
		// Use mcptypes.ToolsCapability
		Tools: &mcptypes.ToolsCapability{ListChanged: false},
		// Use mcptypes.ResourcesCapability
		Resources: &mcptypes.ResourcesCapability{ListChanged: false, Subscribe: false},
		// Use mcptypes.PromptsCapability
		Prompts: &mcptypes.PromptsCapability{ListChanged: false},
		// Add other capabilities like 'sampling' if supported.
	}

	// --- FIX: Use GetAppVersion helper function ---
	appVersion := GetAppVersion(h.config)
	// --- END FIX ---

	// Use mcptypes.Implementation
	serverInfo := mcptypes.Implementation{Name: h.config.Server.Name, Version: appVersion}

	// Use mcptypes.InitializeResult
	res := mcptypes.InitializeResult{
		ProtocolVersion: serverProtocolVersion, // Use the forced version.
		ServerInfo:      &serverInfo,
		Capabilities:    caps,
	}

	resBytes, err := json.Marshal(res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal initialize result")
	}

	h.logger.Info("Initialize successful, returning server capabilities.")
	return resBytes, nil
}

// handleShutdown handles the "shutdown" request.
// It prepares the server for exit but doesn't terminate the process itself.
// This handler function signature might change/be removed when routing is fully refactored.
func (h *Handler) handleShutdown(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling shutdown request.")
	// --- FIX: Check if ConnectionState exists before using ---
	if h.connectionState != nil {
		h.connectionState.SetShutdown()
	} else {
		h.logger.Warn("Cannot set shutdown state: connectionState is nil.")
	}
	// --- END FIX ---
	// Actual server shutdown (closing transport, etc.) is handled by the Server.Shutdown method.
	// This handler just acknowledges the request according to the protocol.
	return json.RawMessage(`null`), nil // Shutdown response result is null.
}

// handleExit handles the "exit" notification.
// It logs the event and potentially signals the main application to terminate.
// Note: This is a notification handler, doesn't return response bytes.
// This handler function signature might change/be removed when routing is fully refactored.
// --- FIX: Return type changed to error for consistency ---
func (h *Handler) handleExit(ctx context.Context, _ json.RawMessage) error {
	h.logger.Info("Handling exit notification.")
	// Signal the main server loop to exit gracefully.
	// This might involve canceling the main context or using a channel.
	// The exact mechanism depends on how the server's run loop is structured.
	// For now, just log.
	h.logger.Warn("Exit notification received. Server should terminate process if running standalone.")

	// Find the cancel function passed via context if available.
	type cancelCtxKey struct{}
	cancelFunc, ok := ctx.Value(cancelCtxKey{}).(context.CancelFunc)
	if ok && cancelFunc != nil {
		h.logger.Info("Calling context cancel function due to exit notification.")
		cancelFunc()
	} else {
		// Fallback or alternative mechanism might be needed if context cancellation isn't used.
		h.logger.Warn("No cancel function found in context for exit notification.")
		// Potentially force exit if this is the intended behavior for stdio transport.
		// os.Exit(0). // Be cautious with os.Exit.
	}

	return nil // Notifications don't have responses.
}

// handleCancelRequest handles the "$/cancelRequest" notification.
// Logs the cancellation attempt; actual cancellation logic might be complex.
// This handler function signature might change/be removed when routing is fully refactored.
// --- FIX: Return type changed to error for consistency ---
func (h *Handler) handleCancelRequest(_ context.Context, params json.RawMessage) error {
	// --- FIX: Correct variable declaration ---
	var reqParams struct {
		ID json.RawMessage `json:"id"`
	}
	// --- END FIX ---
	if err := json.Unmarshal(params, &reqParams); err != nil {
		h.logger.Warn("Failed to unmarshal $/cancelRequest params.", "error", err)
		return nil // Don't propagate parse errors for notifications.
	}
	h.logger.Info("Received cancellation request notification.", "requestId", string(reqParams.ID))
	// TODO: Implement actual request cancellation logic if feasible/required.
	// This often involves associating cancellable contexts with ongoing requests.
	return nil
}

// --- Helper ---

// GetAppVersion retrieves the application version (placeholder).
// DEPRECATED: Version should come from config or build flags.
func GetAppVersion(cfg *config.Config) string {
	// In a real application, this would read from embedded build info or a config file.
	// --- FIX: Assuming config might be nil, add check ---
	if cfg != nil && cfg.Server.Name != "" { // Check Name as Version field doesn't exist in current config struct
		// If a specific version field is added to cfg.Server later, use that.
		// For now, maybe return the server name as a proxy? Or a default.
		// Let's stick to a default for now.
		// return cfg.Server.Name // Or return cfg.Server.Version if added
	}
	// --- END FIX ---
	// TODO: Read version from build flags (e.g., ldflags)
	return "0.1.0-dev" // Default fallback.
}
