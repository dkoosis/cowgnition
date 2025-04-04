// file: internal/mcp/connection/handlers.go
package connection

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// handlePing processes a ping request.
// Returns pong response.
func (m *Manager) handlePing(_ context.Context, _ *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	m.logf(definitions.LogLevelDebug, "Received ping request")
	return map[string]interface{}{"pong": true}, nil
}

// handleSubscribe processes a resource subscription request.
func (m *Manager) handleSubscribe(_ context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var subscribeReq struct {
		URI string `json:"uri"`
	}

	if err := jsonrpc.ParseParams(req, &subscribeReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse subscribe request"),
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
			errors.New("missing required parameter: uri"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// In a full implementation, you would store the subscription.
	// and set up notifications when the resource changes.
	m.logf(definitions.LogLevelDebug, "Subscribed to resource %s", subscribeReq.URI)

	return map[string]interface{}{"status": "subscribed"}, nil
}

// Define an unexported type for context keys to avoid collisions.
type contextKey string

const (
	// connectionIDKey is the context key for the connection ID.
	connectionIDKey contextKey = "connection_id"
	// requestIDKey is the context key for the request ID.
	requestIDKey contextKey = "request_id"
)

// handleInitialize processes the initialize request.
func (m *Manager) handleInitialize(_ context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var initReq definitions.InitializeRequest
	if err := jsonrpc.ParseParams(req, &initReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse initialize request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Handle potential legacy field names for client info.
	clientName := initReq.ClientInfo.Name
	clientVersion := initReq.ClientInfo.Version
	if clientName == "" {
		clientName = initReq.ServerName // Using legacy snake_case field.
	}
	if clientVersion == "" {
		clientVersion = initReq.ServerVersion // Using legacy snake_case field.
	}
	m.logf(definitions.LogLevelInfo, "Processing initialize request from client: %s (version: %s)",
		clientName, clientVersion)

	// Check protocol version compatibility (using the function from state.go).
	clientProtoVersion := initReq.ProtocolVersion
	if !isCompatibleProtocolVersion(clientProtoVersion) {
		return nil, cgerr.ErrorWithDetails(
			errors.Newf("incompatible protocol version: %s", clientProtoVersion),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"connection_id":      m.connectionID,
				"client_version":     clientProtoVersion,
				"supported_versions": []string{"2.0", "2024-11-05"}, // Ideally get this from where isCompatibleProtocolVersion is defined.
			},
		)
	}

	m.dataMu.Lock()
	m.clientCapabilities = initReq.Capabilities
	m.dataMu.Unlock()

	serverInfo := definitions.ServerInfo{
		Name:    m.config.Name,
		Version: m.config.Version,
	}

	response := definitions.InitializeResponse{
		ServerInfo:      serverInfo,
		Capabilities:    m.config.Capabilities,
		ProtocolVersion: clientProtoVersion, // Echo back compatible version client sent.
	}

	m.logf(definitions.LogLevelDebug, "handleInitialize successful")
	return response, nil
}

// handleListResources processes a list_resources request.
// Returns resource definitions.
func (m *Manager) handleListResources(_ context.Context, _ *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	resources := m.resourceManager.GetAllResourceDefinitions()
	m.logf(definitions.LogLevelDebug, "Listed %d resources", len(resources))
	return definitions.ListResourcesResponse{
		Resources: resources,
	}, nil
}

// handleReadResource processes a read_resource request.
func (m *Manager) handleReadResource(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var readReq struct {
		Name string            `json:"name"`
		Args map[string]string `json:"args,omitempty"`
	}

	if err := jsonrpc.ParseParams(req, &readReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse read_resource request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	if readReq.Name == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("missing required parameter: name"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Call the resource manager.
	contentStr, mimeType, err := m.resourceManager.ReadResource(ctx, readReq.Name, readReq.Args)
	if err != nil {
		errCode := cgerr.GetErrorCode(err) // Attempt to get specific code.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read resource"),
			cgerr.CategoryResource,
			errCode,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"resource_name": readReq.Name,
				"resource_args": readReq.Args,
			},
		)
	}

	m.logf(definitions.LogLevelDebug, "Read resource %s, mime type: %s, content length: %d",
		readReq.Name, mimeType, len(contentStr))

	return definitions.ResourceResponse{
		Content:  contentStr,
		MimeType: mimeType,
	}, nil
}

// handleListTools processes a list_tools request.
// Returns tool definitions.
func (m *Manager) handleListTools(_ context.Context, _ *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	tools := m.toolManager.GetAllToolDefinitions()
	m.logf(definitions.LogLevelDebug, "Listed %d tools", len(tools))
	return definitions.ListToolsResponse{Tools: tools}, nil
}

// handleCallTool processes a call_tool request.
func (m *Manager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var callReq definitions.CallToolRequest
	if err := jsonrpc.ParseParams(req, &callReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse call_tool request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	if callReq.Name == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("missing required parameter: name"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    jsonrpc.FormatRequestID(req.ID),
			},
		)
	}

	// Add context values that might be useful for the tool implementation.
	childCtx := context.WithValue(ctx, connectionIDKey, m.connectionID)
	childCtx = context.WithValue(childCtx, requestIDKey, jsonrpc.FormatRequestID(req.ID))

	startTime := time.Now()
	// Call the tool manager.
	resultStr, err := m.toolManager.CallTool(childCtx, callReq.Name, callReq.Arguments)
	duration := time.Since(startTime)

	if err != nil {
		errCode := cgerr.GetErrorCode(err) // Attempt to get specific code.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to call tool"),
			cgerr.CategoryTool,
			errCode,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"tool_name":     callReq.Name,
				"tool_args":     callReq.Arguments,
				"duration_ms":   duration.Milliseconds(),
			},
		)
	}

	m.logf(definitions.LogLevelDebug, "Called tool %s, execution time: %s, result length: %d",
		callReq.Name, duration, len(resultStr))

	return definitions.ToolResponse{Result: resultStr}, nil
}

// handleShutdownRequest handles the RPC message for shutdown.
// Returns acknowledgement status.
func (m *Manager) handleShutdownRequest(_ context.Context, _ *jsonrpc2.Request) (interface{}, error) { //nolint:unparam
	m.logf(definitions.LogLevelInfo, "Received shutdown request via RPC")

	// Acknowledges the request immediately. Actual shutdown action is triggered.
	// via state machine (e.g., firing TriggerShutdown).
	// The response here confirms receipt, not completion.
	return map[string]interface{}{"status": "shutdown_acknowledged"}, nil
}
