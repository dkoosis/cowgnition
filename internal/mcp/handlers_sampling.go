// Package mcp implements the Model Context Protocol server logic, including handlers and types.

package mcp

// file: internal/mcp/handlers_sampling.go

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// handleSamplingCreateMessage would handle a sampling/createMessage request from server to client.
// Official definition: A request from the server to sample an LLM via the client.
// NOTE: This is typically a server-to-client request, but included here as a stub for completeness.
// When implementing a complete MCP client, this would be relevant.
// nolint:unused,unparam
func (h *Handler) handleSamplingCreateMessage(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Warn("Received sampling/createMessage, which is a server-to-client request.")
	// This would typically be implemented by the client, not the server.
	return nil, errors.New("sampling/createMessage is a server-to-client request, not implemented by the server")
}
