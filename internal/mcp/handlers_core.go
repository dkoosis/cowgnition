// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/handlers_core.go.
package mcp

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"                             // Using cockroachdb/errors for wrapping.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import the shared types package.
)

// handleInitialize handles the initialize request.
// Official definition: This request is sent from the client to the server when it first connects,
// asking it to begin initialization. The server responds with information about its capabilities,
// supported protocol version, and other metadata.
//
// NOTE (Interim Fix): This handler currently FORCES the protocol version to "2024-11-05"
// in the response. This is an interim solution to ensure compatibility with clients
// requesting that specific version, because the official schema.json file currently lacks
// a standard version identifier (like $id) for automatic detection.
// See: https://github.com/modelcontextprotocol/modelcontextprotocol/issues/394 .
// The automatic detection logic remains in internal/schema/validator.go but is bypassed here
// for setting the response version. Ideally, the schema file should be updated, and this
// handler should revert to using h.validator.GetSchemaVersion().
func (h *Handler) handleInitialize(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req mcptypes.InitializeRequest // Use mcptypes.InitializeRequest.
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for initialize")
	}

	// --- START INTERIM FIX ---.
	// Force the protocol version to the one expected by the client (e.g., Claude Desktop),
	// regardless of what the schema file might contain or what the validator detects.
	const forcedProtocolVersion = "2024-11-05"

	// Log client request and the version this server will respond with.
	h.logger.Info("Handling initialize request.",
		"clientRequestedVersion", req.ProtocolVersion,
		"serverForcingVersion", forcedProtocolVersion, // Log the version we are forcing.
		"clientName", req.ClientInfo.Name,
	)

	// Log a warning if the client requested something different than what we are forcing.
	// This helps debug connections with other clients in the future.
	if req.ProtocolVersion != forcedProtocolVersion {
		h.logger.Warn("MCP Protocol Version Mismatch Detected!.",
			"clientRequested", req.ProtocolVersion,
			"serverRespondingWith", forcedProtocolVersion, // Log the forced version.
			"action", "Proceeding with forced server version (client MAY disconnect if it doesn't match its request, per MCP spec).")
	}
	// --- END INTERIM FIX ---.

	// Enable capabilities based on what this server supports.
	// Ensure these are appropriate for the forced 2024-11-05 spec version if needed.
	caps := mcptypes.ServerCapabilities{ // Use mcptypes.ServerCapabilities.
		Tools:     &mcptypes.ToolsCapability{ListChanged: false},                       // Use mcptypes.ToolsCapability.
		Resources: &mcptypes.ResourcesCapability{ListChanged: false, Subscribe: false}, // Use mcptypes.ResourcesCapability.
		Prompts:   &mcptypes.PromptsCapability{ListChanged: false},                     // Use mcptypes.PromptsCapability.
		Logging:   map[string]interface{}{},
	}

	appVersion := "0.1.0-dev"                                                              // TODO: Get from build flags.
	serverInfo := mcptypes.Implementation{Name: h.config.Server.Name, Version: appVersion} // Use mcptypes.Implementation.

	// --- Use the forced version in the response ---.
	res := mcptypes.InitializeResult{ // Use mcptypes.InitializeResult.
		ServerInfo:      serverInfo,
		ProtocolVersion: forcedProtocolVersion, // Use the forced constant here.
		Capabilities:    caps,
		Instructions:    "You can use CowGnition to manage your Remember The Milk tasks. Use RTM tools to create, view, and complete tasks.",
	}
	// -------------------------------------------.

	resultBytes, err := json.Marshal(res)
	if err != nil {
		h.logger.Error("Failed to marshal InitializeResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal InitializeResult")
	}
	return resultBytes, nil
}

// handlePing handles the ping request.
// Official definition: A ping, issued by either the server or the client, to check that
// the other party is still alive. The receiver must promptly respond, or else may be disconnected.
func (h *Handler) handlePing(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Debug("Handling ping request.")
	// Empty object is a valid response for ping.
	resultBytes, err := json.Marshal(map[string]interface{}{})
	if err != nil {
		h.logger.Error("Failed to marshal empty ping result.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ping response")
	}
	return resultBytes, nil
}
