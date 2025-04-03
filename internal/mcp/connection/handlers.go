// file: internal/mcp/connection/handlers.go
package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// handleInitialize processes the initialize request.
func (m *ConnectionManager) handleInitialize(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
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
				"supported_versions": []string{"2.0", "2024-11-05"},
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
		ProtocolVersion: clientProtoVersion,
	}

	m.logf(definitions.LogLevelDebug, "handleInitialize successful (id: %s)", m.connectionID)
	return response, nil
}

// handleListResources processes a list_resources request.
func (m *ConnectionManager) handleListResources(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Get resource definitions and convert types as needed
	resourceInterfaces := m.resourceManager.GetAllResourceDefinitions()
	resources := make([]definitions.ResourceDefinition, len(resourceInterfaces))

	// Convert each interface{} to ResourceDefinition
	for i, r := range resourceInterfaces {
		if rd, ok := r.(definitions.ResourceDefinition); ok {
			resources[i] = rd
		} else {
			// Log warning and create empty definition if type conversion fails
			m.logf(definitions.LogLevelWarn, "Failed to convert resource definition at index %d (id: %s)", i, m.connectionID)
			resources[i] = definitions.ResourceDefinition{}
		}
	}

	m.logf(definitions.LogLevelDebug, "Listed %d resources (id: %s)", len(resources), m.connectionID)
	return definitions.ListResourcesResponse{
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

	// Convert []byte to string if necessary
	contentStr := ""
	if contentBytes, ok := content.([]byte); ok {
		contentStr = string(contentBytes)
	} else if contentStr, ok = content.(string); !ok {
		// Try to convert to string otherwise
		contentStr = fmt.Sprintf("%v", content)
	}

	m.logf(definitions.LogLevelDebug, "Read resource %s, mime type: %s, content length: %d (id: %s)",
		readReq.Name, mimeType, len(contentStr), m.connectionID)

	return definitions.ResourceResponse{
		Content:  contentStr,
		MimeType: mimeType,
	}, nil
}

// handleListTools processes a list_tools request.
func (m *ConnectionManager) handleListTools(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Get tool definitions and convert types as needed
	toolInterfaces := m.toolManager.GetAllToolDefinitions()
	tools := make([]definitions.ToolDefinition, len(toolInterfaces))

	// Convert each interface{} to ToolDefinition
	for i, t := range toolInterfaces {
		if td, ok := t.(definitions.ToolDefinition); ok {
			tools[i] = td
		} else {
			// Log warning and create empty definition if type conversion fails
			m.logf(definitions.LogLevelWarn, "Failed to convert tool definition at index %d (id: %s)", i, m.connectionID)
			tools[i] = definitions.ToolDefinition{}
		}
	}

	m.logf(definitions.LogLevelDebug, "Listed %d tools (id: %s)", len(tools), m.connectionID)
	return definitions.ListToolsResponse{Tools: tools}, nil
}

// handleCallTool processes a call_tool request.
func (m *ConnectionManager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
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

	// Convert result to string if necessary
	resultStr := ""
	if resultBytes, ok := result.([]byte); ok {
		resultStr = string(resultBytes)
	} else if resultStr, ok = result.(string); !ok {
		// Try to convert to string otherwise
		resultStr = fmt.Sprintf("%v", result)
	}

	m.logf(definitions.LogLevelDebug, "Called tool %s, execution time: %s, result length: %d (id: %s)",
		callReq.Name, duration, len(resultStr), m.connectionID)

	return definitions.ToolResponse{Result: resultStr}, nil
}

// handleShutdownRequest handles the RPC message for shutdown.
func (m *ConnectionManager) handleShutdownRequest(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	m.logf(definitions.LogLevelInfo, "Received shutdown request via RPC (id: %s)", m.connectionID)
	// Acknowledges the request. Actual shutdown action is triggered via FSM OnEntry.
	return map[string]interface{}{
		"status": "shutdown_initiated",
	}, nil
}
