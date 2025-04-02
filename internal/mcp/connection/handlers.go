// internal/mcp/connection/handlers.go
package connection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// handleInitialize processes the initialize request and transitions to the connected state.
func (m *ConnectionManager) handleInitialize(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Parse the initialize request
	var initReq InitializeRequest
	if err := json.Unmarshal(*req.Params, &initReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse initialize request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
				"params":        string(*req.Params),
			},
		)
	}

	// Log client information
	clientName := initReq.ClientInfo.Name
	clientVersion := initReq.ClientInfo.Version

	// Fall back to legacy fields if needed
	if clientName == "" {
		clientName = initReq.ServerName
	}
	if clientVersion == "" {
		clientVersion = initReq.ServerVersion
	}

	m.logf(LogLevelInfo, "Initializing connection with client: %s (version: %s)",
		clientName, clientVersion)

	// Validate protocol version
	clientProtoVersion := initReq.ProtocolVersion
	if !isCompatibleProtocolVersion(clientProtoVersion) {
		return nil, cgerr.ErrorWithDetails(
			errors.Newf("incompatible protocol version: %s", clientProtoVersion),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"connection_id":      m.connectionID,
				"client_version":     clientProtoVersion,
				"supported_versions": []string{"2.0", "2024-11-05"},
			},
		)
	}

	// Store client capabilities for later use
	m.clientCapabilities = initReq.Capabilities

	// Transition to initializing state
	if err := m.SetState(StateInitializing); err != nil {
		return nil, err
	}

	// Prepare server info for response
	serverInfo := ServerInfo{
		Name:    m.config.Name,
		Version: m.config.Version,
	}

	// Send initialization response
	response := InitializeResponse{
		ServerInfo:      serverInfo,
		Capabilities:    m.config.Capabilities,
		ProtocolVersion: clientProtoVersion, // Echo back the client's protocol version
	}

	// Transition to connected state
	if err := m.SetState(StateConnected); err != nil {
		return nil, err
	}

	return response, nil
}

// handleListResources processes a list_resources request.
func (m *ConnectionManager) handleListResources(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Ensure we're in the connected state
	if m.GetState() != StateConnected {
		return nil, cgerr.ErrorWithDetails(
			errors.New("cannot list resources in current connection state"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"connection_id":  m.connectionID,
				"current_state":  m.GetState(),
				"required_state": StateConnected,
			},
		)
	}

	// Delegate to the resource manager
	resources := m.resourceManager.GetAllResourceDefinitions()

	m.logf(LogLevelDebug, "Listed %d resources", len(resources))

	return ListResourcesResponse{
		Resources: resources,
	}, nil
}

// handleReadResource processes a read_resource request.
func (m *ConnectionManager) handleReadResource(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Parse the request parameters
	var readReq struct {
		Name string            `json:"name"`
		Args map[string]string `json:"args,omitempty"`
	}

	if err := json.Unmarshal(*req.Params, &readReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse read_resource request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
				"params":        string(*req.Params),
			},
		)
	}

	// Validate resource name
	if readReq.Name == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("missing required resource name"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
			},
		)
	}

	// Delegate to the resource manager
	content, mimeType, err := m.resourceManager.ReadResource(ctx, readReq.Name, readReq.Args)
	if err != nil {
		// Add connection context to the error
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read resource"),
			cgerr.CategoryResource,
			cgerr.GetErrorCode(err),
			map[string]interface{}{
				"connection_id": m.connectionID,
				"resource_name": readReq.Name,
				"resource_args": readReq.Args,
			},
		)
	}

	m.logf(LogLevelDebug, "Read resource %s, mime type: %s, content length: %d",
		readReq.Name, mimeType, len(content))

	return ResourceResponse{
		Content:  content,
		MimeType: mimeType,
	}, nil
}

// handleListTools processes a list_tools request.
func (m *ConnectionManager) handleListTools(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Ensure we're in the connected state
	if m.GetState() != StateConnected {
		return nil, cgerr.ErrorWithDetails(
			errors.New("cannot list tools in current connection state"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"connection_id":  m.connectionID,
				"current_state":  m.GetState(),
				"required_state": StateConnected,
			},
		)
	}

	// Delegate to the tool manager
	tools := m.toolManager.GetAllToolDefinitions()

	m.logf(LogLevelDebug, "Listed %d tools", len(tools))

	return ListToolsResponse{
		Tools: tools,
	}, nil
}

// handleCallTool processes a call_tool request.
func (m *ConnectionManager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Parse the request parameters
	var callReq CallToolRequest

	if err := json.Unmarshal(*req.Params, &callReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse call_tool request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
				"params":        string(*req.Params),
			},
		)
	}

	// Validate tool name
	if callReq.Name == "" {
		return nil, cgerr.ErrorWithDetails(
			errors.New("missing required tool name"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
			},
		)
	}

	// Create a child context with metadata
	childCtx := context.WithValue(ctx, "connection_id", m.connectionID)
	childCtx = context.WithValue(childCtx, "request_id", req.ID)

	// Delegate to the tool manager
	startTime := time.Now()
	result, err := m.toolManager.CallTool(childCtx, callReq.Name, callReq.Arguments)
	duration := time.Since(startTime)

	if err != nil {
		// Add connection context to the error
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to call tool"),
			cgerr.CategoryTool,
			cgerr.GetErrorCode(err),
			map[string]interface{}{
				"connection_id": m.connectionID,
				"tool_name":     callReq.Name,
				"tool_args":     callReq.Arguments,
				"duration_ms":   duration.Milliseconds(),
			},
		)
	}

	m.logf(LogLevelDebug, "Called tool %s, execution time: %s, result length: %d",
		callReq.Name, duration, len(result))

	return ToolResponse{
		Result: result,
	}, nil
}

// handleShutdown processes a shutdown request.
func (m *ConnectionManager) handleShutdown(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	m.logf(LogLevelInfo, "Received shutdown request")

	// Start shutdown in a goroutine to allow response to be sent
	go func() {
		// Wait a short time to allow response to be sent
		time.Sleep(100 * time.Millisecond)
		if err := m.Shutdown(); err != nil {
			m.logf(LogLevelError, "Error during shutdown: %v", err)
		}
	}()

	return map[string]interface{}{
		"status": "shutting_down",
	}, nil
}

// handleError processes an error from a handler and sends an appropriate error response.
func (m *ConnectionManager) handleError(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, err error) {
	// Skip error responses for notifications
	if req.Notif {
		m.logf(LogLevelError, "Error handling notification %s: %v", req.Method, err)
		return
	}

	// Determine if this is a protocol error or system error
	code := jsonrpc2.CodeInternalError
	message := "Internal error"
	data := map[string]interface{}{}

	// Check for specific error types
	errorCode := cgerr.GetErrorCode(err)
	if errorCode != 0 {
		code = int64(errorCode)
		message = cgerr.UserFacingMessage(errorCode)
		data = cgerr.GetErrorProperties(err)
	}

	// Log the error with context
	m.logf(LogLevelError, "Error handling request %s: %+v", req.Method, err)

	// Send error response
	rpcErr := &jsonrpc2.Error{
		Code:    code,
		Message: message,
	}

	if len(data) > 0 {
		rpcErr.Data = data
	}

	if err := conn.ReplyWithError(ctx, req.ID, *rpcErr); err != nil {
		m.logf(LogLevelError, "Failed to send error response: %v", err)
	}
}
