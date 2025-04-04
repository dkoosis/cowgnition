// file: internal/mcp/connection/handlers.go
package connection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// handleInitialize processes the initialize request.
func (m *Manager) handleInitialize(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var initReq definitions.InitializeRequest
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
	m.logf(definitions.LogLevelInfo, "Processing initialize request from client: %s (version: %s) (id: %s)",
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
				"supported_versions": []string{"2.0", "2024-11-05"}, // Example versions
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
		ProtocolVersion: clientProtoVersion, // Echo back compatible version client sent
	}

	m.logf(definitions.LogLevelDebug, "handleInitialize successful (id: %s)", m.connectionID)
	return response, nil
}

// handleListResources processes a list_resources request.
//
//nolint:unused
func (m *Manager) handleListResources(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Get resource definitions - the adapter should now return the correct type
	resources := m.resourceManager.GetAllResourceDefinitions()

	m.logf(definitions.LogLevelDebug, "Listed %d resources (id: %s)", len(resources), m.connectionID)
	return definitions.ListResourcesResponse{
		Resources: resources,
	}, nil
}

// handleReadResource processes a read_resource request.
//
//nolint:unused
func (m *Manager) handleReadResource(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
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

	// Call the resource manager - the adapter should now return string content
	contentStr, mimeType, err := m.resourceManager.ReadResource(ctx, readReq.Name, readReq.Args)
	if err != nil {
		// Attempt to get a specific code, default otherwise
		errCode := cgerr.GetErrorCode(err)
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read resource"),
			cgerr.CategoryResource, // Assuming a category for resource errors
			errCode,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"resource_name": readReq.Name,
				"resource_args": readReq.Args,
			},
		)
	}

	// No conversion needed, contentStr is already a string

	m.logf(definitions.LogLevelDebug, "Read resource %s, mime type: %s, content length: %d (id: %s)",
		readReq.Name, mimeType, len(contentStr), m.connectionID)

	return definitions.ResourceResponse{
		Content:  contentStr,
		MimeType: mimeType,
	}, nil
}

// handleListTools processes a list_tools request.
//
//nolint:unused
func (m *Manager) handleListTools(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Get tool definitions - the adapter should now return the correct type
	tools := m.toolManager.GetAllToolDefinitions()

	m.logf(definitions.LogLevelDebug, "Listed %d tools (id: %s)", len(tools), m.connectionID)
	return definitions.ListToolsResponse{Tools: tools}, nil
}

// handleCallTool processes a call_tool request.
//
//nolint:unused
func (m *Manager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var callReq definitions.CallToolRequest
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

	// Add context values that might be useful for the tool implementation
	childCtx := context.WithValue(ctx, "connection_id", m.connectionID)
	childCtx = context.WithValue(childCtx, "request_id", req.ID)

	startTime := time.Now()
	// Call the tool manager - the adapter should now return string result
	resultStr, err := m.toolManager.CallTool(childCtx, callReq.Name, callReq.Arguments)
	duration := time.Since(startTime)

	if err != nil {
		errCode := cgerr.GetErrorCode(err) // Get specific code if available
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to call tool"),
			cgerr.CategoryTool, // Assuming a category for tool errors
			errCode,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"tool_name":     callReq.Name,
				"tool_args":     callReq.Arguments,
				"duration_ms":   duration.Milliseconds(),
			},
		)
	}

	// No conversion needed, resultStr is already a string

	m.logf(definitions.LogLevelDebug, "Called tool %s, execution time: %s, result length: %d (id: %s)",
		callReq.Name, duration, len(resultStr), m.connectionID)

	return definitions.ToolResponse{Result: resultStr}, nil
}

// handleShutdownRequest handles the RPC message for shutdown.
//
//nolint:unused
func (m *Manager) handleShutdownRequest(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	m.logf(definitions.LogLevelInfo, "Received shutdown request via RPC (id: %s)", m.connectionID)
	// Acknowledges the request immediately. Actual shutdown action is triggered
	// via state machine (e.g., firing TriggerShutdown).
	// The response here confirms receipt, not completion.
	return map[string]interface{}{"status": "shutdown_acknowledged"}, nil
}
