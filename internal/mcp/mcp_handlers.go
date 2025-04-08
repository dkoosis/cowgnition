// file: internal/mcp/mcp_handlers.go
package mcp

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
)

// MCPHandler holds dependencies for MCP method handlers.
type MCPHandler struct {
	logger logging.Logger
	config *config.Config
	// Add other dependencies like RTM client instance here.
}

// NewMCPHandler creates a new MCPHandler.
func NewMCPHandler(cfg *config.Config, logger logging.Logger) *MCPHandler {
	return &MCPHandler{
		logger: logger.WithField("component", "mcp_handler"),
		config: cfg,
	}
}

// handleInitialize handles the initialize request.
// Renamed from HandleInitialize.
func (h *MCPHandler) handleInitialize(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Decode params - example using a placeholder struct.
	var req InitializeRequest // Assumes type defined in types.go.
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Invalid params for initialize", errors.WithStack(err))
	}

	h.logger.Info("Handling initialize request.", "clientVersion", req.ProtocolVersion, "clientName", req.ClientInfo.Name)

	// Define server capabilities - enable tools and potentially others.
	caps := ServerCapabilities{ // Assumes type defined in types.go.
		Tools: &ToolsCapability{ListChanged: false}, // Indicate tool support.
		// Example: Enable resources if implemented.
		// Resources: &ResourcesCapability{ListChanged: false, Subscribe: false},
		// Example: Enable logging capability.
		// Logging: map[string]interface{}{},
	}

	// Prepare result.
	serverInfo := Implementation{Name: h.config.Server.Name, Version: Version} // Assumes Version is defined globally or passed in.
	res := InitializeResult{                                                   // Assumes type defined in types.go.
		ServerInfo:      serverInfo,
		ProtocolVersion: "2024-11-05", // The MCP version this server implements.
		Capabilities:    caps,
		// Instructions:    "Optional instructions for the LLM.",
	}

	// Marshal and return result.
	resultBytes, err := json.Marshal(res)
	if err != nil {
		h.logger.Error("Failed to marshal InitializeResult.", "error", err)
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Failed to marshal InitializeResult", errors.WithStack(err))
	}
	return resultBytes, nil
}

// handlePing handles the ping request.
// Renamed from HandlePing.
func (h *MCPHandler) handlePing(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Ping usually returns an empty JSON object or null result upon success.
	h.logger.Debug("Handling ping request.") // Use Debug for frequent messages.
	// Return empty JSON object result.
	return json.Marshal(map[string]interface{}{})
}

// handleToolsList handles the tools/list request.
// Renamed from HandleToolsList and implemented to return the echo tool.
func (h *MCPHandler) handleToolsList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// 1. Define the input schema for the "echo" tool.
	echoSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The message to echo back.",
			},
		},
		"required": []string{"message"},
	}
	echoSchemaBytes, err := json.Marshal(echoSchema)
	if err != nil {
		// Log the error internally.
		h.logger.Error("Failed to marshal echo tool schema.", "error", err)
		// Return an internal error response.
		return nil, mcperrors.NewError(
			mcperrors.ErrProtocolInvalid, // Or a specific internal code.
			"Internal server error: failed to create tool schema",
			errors.WithStack(err),
		)
	}

	// 2. Create the "echo" tool definition.
	echoTool := Tool{ // Assumes 'Tool' type is defined in types.go.
		Name:        "echo",
		Description: "A simple tool that echoes back its input message.",
		InputSchema: json.RawMessage(echoSchemaBytes),
		// Annotations can be added here if needed.
	}

	// 3. Create the result containing the tool list.
	result := ListToolsResult{ // Assumes 'ListToolsResult' type is defined in types.go.
		Tools: []Tool{echoTool}, // Include the echo tool.
		// NextCursor can be added here for pagination if needed.
	}

	// Marshal the result.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListToolsResult.", "error", err)
		// Use the custom error type.
		return nil, mcperrors.NewError(
			mcperrors.ErrProtocolInvalid, // Or a more specific internal error code.
			"Failed to marshal ListToolsResult",
			errors.WithStack(err),
		)
	}

	h.logger.Info("Handled tools/list request.", "toolsCount", len(result.Tools))
	return resultBytes, nil
}

// handleToolCall handles the tools/call request.
// Renamed from HandleToolCall.
func (h *MCPHandler) handleToolCall(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Decode the request parameters to find out which tool is being called.
	var req CallToolRequest // Assumes 'CallToolRequest' type defined in types.go.
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Invalid params for tools/call", errors.WithStack(err))
	}

	h.logger.Info("Handling tool/call request.", "toolName", req.Name)

	// Route the call to the specific tool implementation.
	switch req.Name {
	case "echo":
		// Call the specific function to handle the echo tool.
		return h.executeEchoTool(ctx, req.Arguments)
	// Add cases for other tools here, e.g., RTM tools.
	// case "rtmGetTasks":
	//    return h.executeRTMGetTasks(ctx, req.Arguments)
	default:
		// Tool not found - return error within CallToolResult per ADR 001.
		h.logger.Warn("Tool not found during tool/call.", "toolName", req.Name)
		errResult := CallToolResult{ // Assumes 'CallToolResult' type defined in types.go.
			IsError: true,
			Content: []Content{ // Assumes 'Content' interface and 'TextContent' type defined in types.go.
				TextContent{Type: "text", Text: "Error: Tool not found: " + req.Name},
			},
		}
		// Marshal the CallToolResult containing the error, return nil error for JSON-RPC layer.
		resultBytes, marshalErr := json.Marshal(errResult)
		if marshalErr != nil {
			// This is an internal server error during error reporting.
			h.logger.Error("Failed to marshal tool not found error result.", "error", marshalErr)
			return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Internal error: Failed to marshal error response", errors.WithStack(marshalErr))
		}
		return resultBytes, nil
	}
}

// handleResourcesList handles the resources/list request.
// Renamed from HandleResourcesList.
func (h *MCPHandler) handleResourcesList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// TODO: Implement actual resource listing, potentially based on RTM lists/tags.
	h.logger.Info("Handling resources/list request (currently placeholder).")
	result := ListResourcesResult{ // Assumes type defined in types.go.
		Resources: []Resource{}, // Return empty list for now.
	}

	// Marshal the result.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListResourcesResult.", "error", err)
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Failed to marshal ListResourcesResult", errors.WithStack(err))
	}
	return resultBytes, nil
}

// handleResourcesRead handles the resources/read request.
// Renamed from HandleResourcesRead.
func (h *MCPHandler) handleResourcesRead(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Decode request to get URI.
	var req ReadResourceRequest // Assumes type defined in types.go.
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Invalid params for resources/read", errors.WithStack(err))
	}

	h.logger.Info("Handling resources/read request (currently placeholder).", "uri", req.URI)
	// TODO: Implement actual resource reading based on URI.

	// For now, return resource not found error.
	return nil, mcperrors.NewError(mcperrors.ErrResourceNotFound, "Resource not found: "+req.URI, nil) // This maps to JSON-RPC error.
}

// --- Tool Execution Logic ---

// executeEchoTool is the specific implementation for the "echo" tool.
func (h *MCPHandler) executeEchoTool(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	// Decode the specific arguments for the echo tool.
	var echoArgs struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &echoArgs); err != nil {
		// Invalid arguments for the tool - return error within CallToolResult.
		h.logger.Warn("Invalid arguments received for echo tool.", "error", err, "args", string(args))
		errResult := CallToolResult{
			IsError: true,
			Content: []Content{
				TextContent{Type: "text", Text: "Error calling echo tool: Invalid arguments: " + err.Error()},
			},
		}
		// Marshal the error result and return nil error for JSON-RPC layer.
		resultBytes, marshalErr := json.Marshal(errResult)
		if marshalErr != nil {
			h.logger.Error("Failed to marshal echo tool argument error result.", "error", marshalErr)
			return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Internal error: Failed to marshal error response", errors.WithStack(marshalErr))
		}
		return resultBytes, nil // Return the marshaled CallToolResult containing the error.
	}

	h.logger.Info("Executing echo tool.", "message", echoArgs.Message)

	// Prepare the successful result, echoing the message.
	result := CallToolResult{
		IsError: false,
		Content: []Content{
			TextContent{Type: "text", Text: "Echo: " + echoArgs.Message},
		},
	}

	// Marshal the successful result.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal echo tool success result.", "error", err)
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Internal error: Failed to marshal success response", errors.WithStack(err))
	}
	return resultBytes, nil
}

// Add other tool execution functions here (e.g., executeRTMGetTasks).

// --- Need types from types.go ---
// Ensure these types are defined correctly in internal/mcp/types.go

// InitializeRequest represents the request sent by a client to initialize the connection.
type InitializeRequest struct {
	ClientInfo      Implementation     `json:"clientInfo"`
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// ClientCapabilities defines the capabilities that a client may support.
type ClientCapabilities struct {
	Sampling     *struct{}                  `json:"sampling,omitempty"`
	Roots        *struct{}                  `json:"roots,omitempty"`
	Experimental map[string]json.RawMessage `json:"experimental,omitempty"`
}

// InitializeResult represents the server's response to an initialize request.
type InitializeResult struct {
	ServerInfo      Implementation     `json:"serverInfo"`
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ListToolsResult represents the result of a tools/list request.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolRequest represents a request to call a tool.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// CallToolResult represents the result of a tool call.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// ListResourcesResult represents the result of a resources/list request.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceRequest represents a request to read a resource.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// Assuming Content, TextContent, Tool, Resource are defined correctly in types.go.
