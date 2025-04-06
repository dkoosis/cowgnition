// file: internal/mcp/connection/handlers.go
// Package connection contains handlers for processing specific MCP methods within a connection's lifecycle.
// Terminate all comments with a period.
package connection

import (
	"context" // Keep for logging if needed.
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"

	// Use the corrected definitions package.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// handlePing processes a ping request.
// Returns pong response.
func (m *Manager) handlePing(_ context.Context, _ *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	m.logf(definitions.LogLevelDebug, "Received ping request.") // Added period.
	// Simple map is fine as interface{} return.
	return map[string]interface{}{"pong": true}, nil
}

// handleSubscribe processes a resource subscription request.
// NOTE: This likely needs updating if the response spec changes, currently returns simple status.
func (m *Manager) handleSubscribe(_ context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Define params struct locally for parsing.
	var subscribeReq struct {
		URI string `json:"uri"`
	}

	if err := jsonrpc.ParseParams(req.Params, &subscribeReq); err != nil { // Pass req.Params directly.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "handleSubscribe: failed to parse subscribe request params."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	if subscribeReq.URI == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("handleSubscribe: missing required parameter: uri."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// TODO: Implement actual subscription logic here.
	m.logf(definitions.LogLevelDebug, "Subscribed to resource %s (placeholder implementation).", subscribeReq.URI) // Added context.

	// Simple map is fine as interface{} return for now.
	return map[string]interface{}{"status": "subscribed"}, nil
}

// --- Context Keys ---

// Define an unexported type for context keys to avoid collisions.
type contextKey string

const (
	// connectionIDKey is the context key for the connection ID.
	connectionIDKey contextKey = "connection_id"
	// requestIDKey is the context key for the request ID.
	requestIDKey contextKey = "request_id"
)

// --- Request Handlers ---

// handleInitialize processes the initialize request.
func (m *Manager) handleInitialize(_ context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Corrected: Parse into InitializeRequestParams struct.
	var params definitions.InitializeRequestParams
	if err := jsonrpc.ParseParams(req.Params, &subscribeReq); err != nil { // Pass req.Params.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "handleInitialize: failed to parse initialize request params."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Handle client info (no legacy fields needed if parsing InitializeRequestParams directly).
	clientName := params.ClientInfo.Name
	clientVersion := params.ClientInfo.Version
	m.logf(definitions.LogLevelInfo, "Processing initialize request from client: %s (version: %s).",
		clientName, clientVersion) // Added period.

	// Check protocol version compatibility.
	clientProtoVersion := params.ProtocolVersion
	// Assuming isCompatibleProtocolVersion is defined elsewhere in the package.
	if !isCompatibleProtocolVersion(clientProtoVersion) {
		return nil, cgerr.ErrorWithDetails(
			errors.Newf("handleInitialize: incompatible protocol version: %s.", clientProtoVersion), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"connection_id":      m.connectionID,
				"client_version":     clientProtoVersion,
				"supported_versions": getSupportedProtocolVersions(), // Use helper func.
			},
		)
	}

	// Store client capabilities (already correct type definitions.ClientCapabilities).
	m.dataMu.Lock()
	m.clientCapabilities = params.Capabilities // Store the parsed capabilities struct.
	m.dataMu.Unlock()

	// Construct the response using the corrected definitions.InitializeResult struct.
	// The ServerCapabilities in m.config MUST have been corrected elsewhere (e.g., adapter).
	response := definitions.InitializeResult{
		// Corrected: Use definitions.Implementation for ServerInfo.
		ServerInfo: definitions.Implementation{
			Name:    m.config.Name,
			Version: m.config.Version,
		},
		// Corrected: Assign ServerCapabilities struct directly.
		// This assumes m.config.Capabilities is now the correct struct type or compatible map.
		Capabilities:    m.config.Capabilities.(definitions.ServerCapabilities), // Type assertion needed if config still holds map[string]interface{}.
		ProtocolVersion: clientProtoVersion,                                     // Echo back compatible version client sent.
		// Instructions: &instructionsString, // Add if defined.
	}

	m.logf(definitions.LogLevelDebug, "handleInitialize successful.") // Added period.
	// Return the result struct directly. JSON marshalling happens later.
	return response, nil
}

// handleListResources processes a resources/list request.
func (m *Manager) handleListResources(_ context.Context, req *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	// TODO: Handle pagination parameters (cursor) if present in req.Params.

	// Call the resource manager using the updated contract interface.
	resources := m.resourceManager.GetAllResourceDefinitions()                // Now returns []definitions.Resource.
	m.logf(definitions.LogLevelDebug, "Listed %d resources.", len(resources)) // Added period.

	// Corrected: Return the spec-compliant ListResourcesResult struct.
	response := definitions.ListResourcesResult{
		Resources: resources,
		// NextCursor: &nextCursorValue, // Add if pagination is implemented.
	}
	return response, nil
}

// Define params struct for resources/read request.
type readResourceRequestParams struct {
	URI string `json:"uri"`
}

// handleReadResource processes a resources/read request.
func (m *Manager) handleReadResource(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Corrected: Parse into local struct expecting URI.
	var params readResourceRequestParams
	if err := jsonrpc.ParseParams(req.Params, &subscribeReq); err != nil { // Pass req.Params.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "handleReadResource: failed to parse request params."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Corrected: Validate URI instead of Name.
	if params.URI == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("handleReadResource: missing required parameter: uri."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Corrected: Call the resource manager using the updated contract signature.
	// It now returns (definitions.ReadResourceResult, error).
	result, err := m.resourceManager.ReadResource(ctx, params.URI)
	if err != nil {
		// Error is already wrapped by the ResourceManager, add connection context.
		errCode := cgerr.GetErrorCode(err)
		return nil, cgerr.ErrorWithDetails(
			errors.Wrapf(err, "handleReadResource: failed to read resource URI '%s'.", params.URI), // Added func context.
			cgerr.CategoryResource,
			errCode,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"resource_uri":  params.URI, // Use URI in context.
			},
		)
	}

	// Log success, result contains details.
	m.logf(definitions.LogLevelDebug, "Read resource %s successfully.", params.URI) // Added period.

	// Corrected: Return the definitions.ReadResourceResult directly.
	return result, nil
}

// handleListTools processes a tools/list request.
func (m *Manager) handleListTools(_ context.Context, req *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	// TODO: Handle pagination parameters (cursor) if present in req.Params.

	// Call the tool manager using the updated contract interface.
	tools := m.toolManager.GetAllToolDefinitions()                    // Now returns []definitions.ToolDefinition.
	m.logf(definitions.LogLevelDebug, "Listed %d tools.", len(tools)) // Added period.

	// Corrected: Return the spec-compliant ListToolsResult struct.
	response := definitions.ListToolsResult{
		Tools: tools,
		// NextCursor: &nextCursorValue, // Add if pagination is implemented.
	}
	return response, nil
}

// handleCallTool processes a tools/call request.
func (m *Manager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Corrected: Parse into definitions.CallToolRequestParams struct.
	var params definitions.CallToolRequestParams
	if err := jsonrpc.ParseParams(req.Params, &subscribeReq); err != nil { // Pass req.Params.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "handleCallTool: failed to parse request params."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Corrected: Validate Name from params struct.
	if params.Name == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("handleCallTool: missing required parameter: name."), // Added func context.
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Add context values.
	childCtx := context.WithValue(ctx, connectionIDKey, m.connectionID)
	if reqID := jsonrpc.FormatRequestID(req.ID); reqID != "" { // Avoid adding empty request ID.
		childCtx = context.WithValue(childCtx, requestIDKey, reqID)
	}

	startTime := time.Now()
	// Corrected: Call the tool manager using the updated contract signature.
	// It now returns (definitions.CallToolResult, error).
	result, err := m.toolManager.CallTool(childCtx, params.Name, params.Arguments)
	duration := time.Since(startTime)

	// Handle Go errors returned from the manager (protocol errors, internal errors, etc.).
	if err != nil {
		errCode := cgerr.GetErrorCode(err)
		// Error already wrapped by ToolManager, add connection context details.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrapf(err, "handleCallTool: failed to call tool '%s'.", params.Name), // Added func context.
			cgerr.CategoryTool, // Keep category set by lower layer if appropriate.
			errCode,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
				"tool_name":     params.Name,
				"tool_args":     params.Arguments,
				"duration_ms":   duration.Milliseconds(),
			},
		)
	}

	// If err is nil, the call was successful at the protocol level.
	// The 'result' (definitions.CallToolResult) contains the actual tool output
	// and potentially an 'IsError' flag indicating a tool-specific failure.
	// We simply return the result struct; the client interprets IsError.
	m.logf(definitions.LogLevelDebug, "Called tool %s, execution time: %s.", params.Name, duration) // Added period.

	// Corrected: Return the definitions.CallToolResult directly.
	return result, nil
}

// handleShutdownRequest handles the RPC message for shutdown.
// Returns acknowledgement status. No changes needed here structurally.
func (m *Manager) handleShutdownRequest(_ context.Context, _ *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	m.logf(definitions.LogLevelInfo, "Received shutdown request via RPC.") // Added period.
	// Simple map response is okay here.
	return map[string]interface{}{"status": "shutdown_acknowledged"}, nil
}

// Helper function to get supported versions.
func getSupportedProtocolVersions() []string {
	// Should ideally be defined centrally.
	return []string{definitions.LATEST_PROTOCOL_VERSION, "2.0"} // Example.
}
