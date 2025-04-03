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

// handleInitialize processes the initialize request.
func (m *ConnectionManager) handleInitialize(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var initReq InitializeRequest
	if err := json.Unmarshal(*req.Params, &initReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse initialize request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
			},
		)
	}

	clientName := initReq.ClientInfo.Name
	clientVersion := initReq.ClientInfo.Version
	if clientName == "" {
		clientName = initReq.ServerName // Using legacy snake_case field
	}
	if clientVersion == "" {
		clientVersion = initReq.ServerVersion // Using legacy snake_case field
	}
	m.logf(LogLevelInfo, "Processing initialize request from client: %s (version: %s) (id: %s)",
		clientName, clientVersion, m.connectionID)

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

	m.dataMu.Lock()
	m.clientCapabilities = initReq.Capabilities
	m.dataMu.Unlock()

	serverInfo := ServerInfo{
		Name:    m.config.Name,
		Version: m.config.Version,
	}

	response := InitializeResponse{
		ServerInfo:      serverInfo,
		Capabilities:    m.config.Capabilities,
		ProtocolVersion: clientProtoVersion,
	}

	m.logf(LogLevelDebug, "handleInitialize successful (id: %s)", m.connectionID)
	return response, nil
}

// handleListResources processes a list_resources request.
func (m *ConnectionManager) handleListResources(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	resources := m.resourceManager.GetAllResourceDefinitions()
	m.logf(LogLevelDebug, "Listed %d resources (id: %s)", len(resources), m.connectionID)
	return ListResourcesResponse{
		Resources: resources,
	}, nil
}

// handleReadResource processes a read_resource request.
func (m *ConnectionManager) handleReadResource(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
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
				"request_id":    req.ID,
			},
		)
	}

	content, mimeType, err := m.resourceManager.ReadResource(ctx, readReq.Name, readReq.Args)
	if err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read resource"),
			cgerr.CategoryResource, cgerr.GetErrorCode(err),
			map[string]interface{}{
				"connection_id": m.connectionID,
				"resource_name": readReq.Name,
				"resource_args": readReq.Args,
			},
		)
	}

	m.logf(LogLevelDebug, "Read resource %s, mime type: %s, content length: %d (id: %s)",
		readReq.Name, mimeType, len(content), m.connectionID)

	return ResourceResponse{
		Content:  content,
		MimeType: mimeType,
	}, nil
}

// handleListTools processes a list_tools request.
func (m *ConnectionManager) handleListTools(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	tools := m.toolManager.GetAllToolDefinitions()
	m.logf(LogLevelDebug, "Listed %d tools (id: %s)", len(tools), m.connectionID)
	return ListToolsResponse{Tools: tools}, nil
}

// handleCallTool processes a call_tool request.
func (m *ConnectionManager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var callReq CallToolRequest
	if err := json.Unmarshal(*req.Params, &callReq); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to parse call_tool request"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    req.ID,
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
				"request_id":    req.ID,
			},
		)
	}

	childCtx := context.WithValue(ctx, "connection_id", m.connectionID)
	childCtx = context.WithValue(childCtx, "request_id", req.ID)

	startTime := time.Now()
	result, err := m.toolManager.CallTool(childCtx, callReq.Name, callReq.Arguments)
	duration := time.Since(startTime)

	if err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to call tool"),
			cgerr.CategoryTool, cgerr.GetErrorCode(err),
			map[string]interface{}{
				"connection_id": m.connectionID,
				"tool_name":     callReq.Name,
				"tool_args":     callReq.Arguments,
				"duration_ms":   duration.Milliseconds(),
			},
		)
	}

	m.logf(LogLevelDebug, "Called tool %s, execution time: %s, result length: %d (id: %s)",
		callReq.Name, duration, len(result), m.connectionID)

	return ToolResponse{Result: result}, nil
}

// handleShutdownRequest handles the RPC message for shutdown.
func (m *ConnectionManager) handleShutdownRequest(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	m.logf(LogLevelInfo, "Received shutdown request via RPC (id: %s)", m.connectionID)
	// Acknowledges the request. Actual shutdown action is triggered via FSM OnEntry.
	return map[string]interface{}{
		"status": "shutdown_initiated",
	}, nil
}
