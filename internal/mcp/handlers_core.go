// file: internal/mcp/handlers_core.go

package mcp

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors" // Using cockroachdb/errors for wrapping.
)

// handleInitialize handles the initialize request.
// Official definition: This request is sent from the client to the server when it first connects,
// asking it to begin initialization. The server responds with information about its capabilities,
// supported protocol version, and other metadata.
func (h *Handler) handleInitialize(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for initialize")
	}
	h.logger.Info("Handling initialize request.", "clientVersion", req.ProtocolVersion, "clientName", req.ClientInfo.Name)

	// Enable capabilities based on what we support.
	caps := ServerCapabilities{
		// Tools capability.
		Tools: &ToolsCapability{ListChanged: false},

		// Resources capability.
		Resources: &ResourcesCapability{ListChanged: false, Subscribe: false},

		// Prompts capability.
		Prompts: &PromptsCapability{ListChanged: false},

		// Logging capability - enables the client to receive structured logs.
		Logging: map[string]interface{}{},

		// Experimental capabilities can be added here.
		// Experimental: map[string]json.RawMessage{},
	}

	appVersion := "0.1.0-dev" // TODO: Get from build flags.
	serverInfo := Implementation{Name: h.config.Server.Name, Version: appVersion}
	res := InitializeResult{
		ServerInfo:      serverInfo,
		ProtocolVersion: "2024-11-05", // TODO: Consider making this dynamic or a constant.
		Capabilities:    caps,
		Instructions:    "You can use CowGnition to manage your Remember The Milk tasks. Use RTM tools to create, view, and complete tasks.",
	}

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
func (h *Handler) handlePing(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Debug("Handling ping request.")
	// Empty object is a valid response for ping.
	resultBytes, err := json.Marshal(map[string]interface{}{})
	if err != nil {
		h.logger.Error("Failed to marshal empty ping result.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ping response")
	}
	return resultBytes, nil
}
