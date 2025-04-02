package connection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	// Assuming your custom error package exists:
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	// Assuming your mcp package defining ResourceDefinition/ToolDefinition exists:

	"github.com/sourcegraph/jsonrpc2"
)

// NOTE: Request/Response struct definitions are now taken directly from
// your provided types.go (InitializeRequest, InitializeResponse, ServerInfo,
// ListResourcesResponse, ResourceResponse, ListToolsResponse, CallToolRequest, ToolResponse).
// Ensure that types.go is in the same package or imported appropriately.

// --- Handler Implementations (Refactored using types from types.go) ---

// handleInitialize processes the initialize request.
// Uses InitializeRequest, InitializeResponse, ServerInfo from types.go.
func (m *ConnectionManager) handleInitialize(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var initReq InitializeRequest // Using type from types.go
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
		clientName = initReq.ServerName // Using legacy snake_case field from types.go
	}
	if clientVersion == "" {
		clientVersion = initReq.ServerVersion // Using legacy snake_case field from types.go
	}
	m.logf(LogLevelInfo, "Processing initialize request from client: %s (version: %s) (id: %s)",
		clientName, clientVersion, m.connectionID)

	clientProtoVersion := initReq.ProtocolVersion
	if !isCompatibleProtocolVersion(clientProtoVersion) { // Uses function from state.go
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

	serverInfo := ServerInfo{ // Using type from types.go
		Name:    m.config.Name,
		Version: m.config.Version,
	}

	response := InitializeResponse{ // Using type from types.go
		ServerInfo:      serverInfo, // Note: json tag is server_info
		Capabilities:    m.config.Capabilities,
		ProtocolVersion: clientProtoVersion,
		// Instructions:    "Set instructions if needed", // Field exists in schema, not in types.go struct
	}

	m.logf(LogLevelDebug, "handleInitialize successful (id: %s)", m.connectionID)
	return response, nil
}

// handleListResources processes a list_resources request.
// Uses ListResourcesResponse and mcp.ResourceDefinition from types.go/mcp package.
func (m *ConnectionManager) handleListResources(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Assumes resourceManager interface matches types.go definition
	resources := m.resourceManager.GetAllResourceDefinitions() // Returns []mcp.ResourceDefinition

	m.logf(LogLevelDebug, "Listed %d resources (id: %s)", len(resources), m.connectionID)

	return ListResourcesResponse{ // Uses type from types.go
		Resources: resources,
	}, nil
}

// handleReadResource processes a read_resource request.
// Uses ResourceResponse from types.go (Content is string).
func (m *ConnectionManager) handleReadResource(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var readReq struct {
		// Assuming method uses 'name' internally based on original code. Schema uses 'uri'.
		// Adapt if your manager expects 'uri'.
		Name string            `json:"name"`
		Args map[string]string `json:"args,omitempty"`
	}

	if err := json.Unmarshal(*req.Params, &readReq); err != nil {
		return nil, cgerr.ErrorWithDetails( /* ... */ )
	}

	if readReq.Name == "" {
		return nil, cgerr.ErrorWithDetails( /* ... */ )
	}

	// Assumes resourceManager interface matches types.go definition
	content, mimeType, err := m.resourceManager.ReadResource(ctx, readReq.Name, readReq.Args) // Returns (string, string, error)
	if err != nil {
		// Use cgerr.ErrorWithDetails as before
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read resource"),
			cgerr.CategoryResource, cgerr.GetErrorCode(err),
			map[string]interface{}{
				"connection_id": m.connectionID,
				"resource_name": readReq.Name, "resource_args": readReq.Args,
			},
		)
	}

	m.logf(LogLevelDebug, "Read resource %s, mime type: %s, content length: %d (id: %s)",
		readReq.Name, mimeType, len(content), m.connectionID)

	// Uses ResourceResponse from types.go (Content is string, json tags snake_case)
	return ResourceResponse{
		Content:  content,  // content is string
		MimeType: mimeType, // json tag is mime_type
	}, nil
}

// handleListTools processes a list_tools request.
// Uses ListToolsResponse and mcp.ToolDefinition from types.go/mcp package.
func (m *ConnectionManager) handleListTools(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	// Assumes toolManager interface matches types.go definition
	tools := m.toolManager.GetAllToolDefinitions() // Returns []mcp.ToolDefinition
	m.logf(LogLevelDebug, "Listed %d tools (id: %s)", len(tools), m.connectionID)
	return ListToolsResponse{Tools: tools}, nil // Uses type from types.go
}

// handleCallTool processes a call_tool request.
// Uses CallToolRequest and ToolResponse from types.go (Result is string).
func (m *ConnectionManager) handleCallTool(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	var callReq CallToolRequest // Using type from types.go
	if err := json.Unmarshal(*req.Params, &callReq); err != nil {
		return nil, cgerr.ErrorWithDetails( /* ... */ )
	}

	if callReq.Name == "" {
		return nil, cgerr.ErrorWithDetails( /* ... */ )
	}

	childCtx := context.WithValue(ctx, "connection_id", m.connectionID)
	childCtx = context.WithValue(childCtx, "request_id", req.ID)

	startTime := time.Now()
	// Assumes toolManager interface matches types.go definition
	result, err := m.toolManager.CallTool(childCtx, callReq.Name, callReq.Arguments) // Returns (string, error)
	duration := time.Since(startTime)

	if err != nil {
		// Use cgerr.ErrorWithDetails as before
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to call tool"),
			cgerr.CategoryTool, cgerr.GetErrorCode(err),
			map[string]interface{}{
				"connection_id": m.connectionID,
				"tool_name":     callReq.Name, "tool_args": callReq.Arguments, "duration_ms": duration.Milliseconds(),
			},
		)
	}

	m.logf(LogLevelDebug, "Called tool %s, execution time: %s, result length: %d (id: %s)",
		callReq.Name, duration, len(result), m.connectionID)

	// Uses ToolResponse from types.go (Result is string, json tag is snake_case)
	return ToolResponse{Result: result}, nil // result is string
}

// handleShutdownRequest handles the RPC message for shutdown.
func (m *ConnectionManager) handleShutdownRequest(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	m.logf(LogLevelInfo, "Received shutdown request via RPC (id: %s)", m.connectionID)
	// Acknowledges the request. Actual shutdown action is triggered via FSM OnEntry.
	return map[string]interface{}{
		"status": "shutdown_initiated",
	}, nil
}
