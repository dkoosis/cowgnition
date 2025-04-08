// file: internal/mcp/mcp_handlers.go
package mcp

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors" // Using cockroachdb/errors for wrapping.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Corrected import path.
	// Import other necessary packages like your RTM client if handlers need it.
)

// Handler holds dependencies for MCP method handlers.
type Handler struct {
	logger logging.Logger
	config *config.Config
	// Add other dependencies like RTM client instance here.
}

// NewHandler creates a new Handler.
func NewHandler(cfg *config.Config, logger logging.Logger) *Handler {
	return &Handler{
		logger: logger.WithField("component", "mcp_handler"),
		config: cfg,
	}
}

// handleInitialize handles the initialize request.
func (h *Handler) handleInitialize(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Decode params.
	var req InitializeRequest // Assumes type defined in types.go.
	if err := json.Unmarshal(params, &req); err != nil {
		// Return a wrapped error. createErrorResponse will map it.
		return nil, errors.Wrap(err, "invalid params for initialize")
	}

	h.logger.Info("Handling initialize request.", "clientVersion", req.ProtocolVersion, "clientName", req.ClientInfo.Name)

	// Define server capabilities - enable tools.
	caps := ServerCapabilities{ // Assumes type defined in types.go.
		Tools: &ToolsCapability{ListChanged: false}, // Indicate tool support.
		// Resources: &ResourcesCapability{ListChanged: false, Subscribe: false}, // Example.
		// Logging: map[string]interface{}{}, // Example.
	}

	// Prepare result. Use placeholder version for now.
	// TODO: Pass actual version via config or handler struct.
	appVersion := "0.1.0-dev"
	serverInfo := Implementation{Name: h.config.Server.Name, Version: appVersion} // Assumes type defined in types.go.
	res := InitializeResult{                                                      // Assumes type defined in types.go.
		ServerInfo:      serverInfo,
		ProtocolVersion: "2024-11-05", // The MCP version this server implements.
		Capabilities:    caps,
		// Instructions:    "Optional instructions for the LLM.",
	}

	// Marshal and return result.
	resultBytes, err := json.Marshal(res)
	if err != nil {
		h.logger.Error("Failed to marshal InitializeResult.", "error", err)
		// Return a wrapped error. createErrorResponse will map it.
		return nil, errors.Wrap(err, "failed to marshal InitializeResult")
	}
	return resultBytes, nil
}

// handlePing handles the ping request.
func (h *Handler) handlePing(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Ping usually returns an empty JSON object result upon success.
	h.logger.Debug("Handling ping request.")
	// Return empty JSON object result.
	resultBytes, err := json.Marshal(map[string]interface{}{})
	if err != nil { // Should ideally not happen for an empty map.
		h.logger.Error("Failed to marshal empty ping result.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ping response")
	}
	return resultBytes, nil
}

// handleToolsList handles the tools/list request.
func (h *Handler) handleToolsList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
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
		// Return a wrapped error.
		return nil, errors.Wrap(err, "internal server error: failed to create tool schema")
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
		// Return a wrapped error.
		return nil, errors.Wrap(err, "failed to marshal ListToolsResult")
	}

	h.logger.Info("Handled tools/list request.", "toolsCount", len(result.Tools))
	return resultBytes, nil
}

// handleToolCall handles the tools/call request.
func (h *Handler) handleToolCall(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Decode the request parameters to find out which tool is being called.
	var req CallToolRequest // Assumes 'CallToolRequest' type defined in types.go.
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for tools/call") // Wrapped error.
	}

	h.logger.Info("Handling tool/call request.", "toolName", req.Name)

	// Route the call to the specific tool implementation.
	switch req.Name {
	case "echo":
		// Call the specific function to handle the echo tool.
		return h.executeEchoTool(ctx, req.Arguments) // Pass ctx even if unused below.
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
			return nil, errors.Wrap(marshalErr, "internal error: Failed to marshal error response") // Wrapped error.
		}
		return resultBytes, nil // Return success at JSON-RPC level (contains error details).
	}
}

// handleResourcesList handles the resources/list request.
func (h *Handler) handleResourcesList(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// TODO: Implement actual resource listing, potentially based on RTM lists/tags.
	h.logger.Info("Handling resources/list request (currently placeholder).")
	result := ListResourcesResult{ // Assumes type defined in types.go.
		Resources: []Resource{}, // Return empty list for now.
	}

	// Marshal the result.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("Failed to marshal ListResourcesResult.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal ListResourcesResult") // Wrapped error.
	}
	return resultBytes, nil
}

// handleResourcesRead handles the resources/read request.
func (h *Handler) handleResourcesRead(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Decode request to get URI.
	var req ReadResourceRequest // Assumes type defined in types.go.
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.Wrap(err, "invalid params for resources/read") // Wrapped error.
	}

	h.logger.Info("Handling resources/read request (currently placeholder).", "uri", req.URI)
	// TODO: Implement actual resource reading based on URI.

	// For now, return resource not found error using specific mcperrors type.
	return nil, mcperrors.NewResourceError("Resource not found: "+req.URI, nil, map[string]interface{}{"uri": req.URI})
}

// --- Tool Execution Logic ---

// executeEchoTool is the specific implementation for the "echo" tool.
// Renamed ctx to _ because it's unused.
func (h *Handler) executeEchoTool(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
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
			// Return wrapped internal error.
			return nil, errors.Wrap(marshalErr, "internal error: Failed to marshal error response")
		}
		return resultBytes, nil // Return the marshaled CallToolResult containing the error.
	}

	// Use logger from handler 'h'. Use context '_' if logging func requires it but it's unused.
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
		// Return wrapped internal error.
		return nil, errors.Wrap(err, "internal error: Failed to marshal success response")
	}
	return resultBytes, nil
}

// Add other tool execution functions here (e.g., executeRTMGetTasks).
