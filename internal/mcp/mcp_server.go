// file: internal/mcp/mcp_server.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt" // Added for error formatting.
	"io"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors" // Ensure cockroachdb/errors is imported.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// ServerOptions contains configurable options for the MCP server.
type ServerOptions struct {
	// RequestTimeout specifies the maximum duration for processing a request.
	RequestTimeout time.Duration

	// ShutdownTimeout specifies the maximum duration to wait for graceful shutdown.
	ShutdownTimeout time.Duration

	// Debug enables additional debug logging and information.
	Debug bool
}

// MethodHandler is a function type for handling MCP method calls.
type MethodHandler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)

// Server represents an MCP (Model Context Protocol) server instance.
// It handles communication with clients via the protocol.
type Server struct {
	// Configuration for the server.
	config *config.Config

	// Server options.
	options ServerOptions

	// The handler for MCP methods (defined in mcp_handlers.go).
	handler *MCPHandler

	// Method map for routing requests.
	methods map[string]MethodHandler

	// Transport for communication.
	transport transport.Transport

	// Logger for server events.
	logger logging.Logger
}

// NewServer creates a new MCP server with the given configuration and options.
func NewServer(cfg *config.Config, opts ServerOptions, logger logging.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	// Create the MCP method handler (constructor defined in mcp_handlers.go).
	handler := NewMCPHandler(cfg, logger)

	// Create the server instance.
	server := &Server{
		config:  cfg,
		options: opts,
		handler: handler,
		logger:  logger.WithField("component", "mcp_server"),
		methods: make(map[string]MethodHandler),
	}

	// Register method handlers.
	server.registerMethods()

	return server, nil
}

// registerMethods registers all supported MCP methods using lowercase handler names.
func (s *Server) registerMethods() {
	// Core MCP methods.
	s.methods["initialize"] = s.handler.handleInitialize
	s.methods["ping"] = s.handler.handlePing

	// Tools methods.
	s.methods["tools/list"] = s.handler.handleToolsList
	s.methods["tools/call"] = s.handler.handleToolCall

	// Resources methods (assuming handlers exist in mcp_handlers.go).
	s.methods["resources/list"] = s.handler.handleResourcesList
	s.methods["resources/read"] = s.handler.handleResourcesRead

	s.logger.Info("Registered MCP methods.",
		"count", len(s.methods),
		"methods", getMethods(s.methods))
}

// getMethods returns a slice of registered method names for logging.
func getMethods(methods map[string]MethodHandler) []string {
	result := make([]string, 0, len(methods))
	for method := range methods {
		result = append(result, method)
	}
	return result
}

// ServeSTDIO starts the server using standard input/output as the transport.
// This is typically used when the server is launched by a client like Claude Desktop.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")

	// Create a transport using stdin/stdout.
	// TODO: Consider adding MaxMessageSize option from config/options here.
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, nil)

	// TODO: Implement middleware chain including validation middleware here,
	// based on ADR 002, before calling s.serve.
	// Example (conceptual):
	// validator := schema.NewSchemaValidator(...)
	// validator.Initialize(ctx)
	// validationMiddleware := middleware.NewValidationMiddleware(validator, ...)
	// chain := middleware.NewChain(s.handleMessage) // Pass final handler.
	// chain.Use(validationMiddleware) // Add middleware.
	// serveHandler := chain.Handler() // Get the full chain handler.
	// return s.serve(ctx, serveHandler) // Pass chain handler to serve loop.

	// Serve requests until the context is canceled (using direct handleMessage for now).
	return s.serve(ctx)
}

// serve handles the main server loop, processing requests using the configured transport.
// Ideally, this would receive a handler representing the full middleware chain.
func (s *Server) serve(ctx context.Context) error {
	s.logger.Info("Server started, waiting for requests.")

	// This function ideally takes the final handler from the middleware chain.
	// For now, it directly calls handleMessage.
	handlerFunc := s.handleMessage // Replace with `serveHandler` when middleware is added.

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context canceled, shutting down.")
			return ctx.Err()
		default:
			// Read a message from the transport.
			// TODO: Reading should ideally be part of the middleware chain or managed
			//       by the component initiating the handler call.
			msgBytes, err := s.transport.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, transport.ErrTransportClosed) {
					s.logger.Info("Connection closed.")
					return nil // Clean exit on EOF or closed transport.
				}
				// Log transport read errors.
				s.logger.Error("Failed to read message.", "error", fmt.Sprintf("%+v", err))
				// Depending on the error, might need to send a response or continue.
				// For now, we continue, assuming the transport might recover or it was a single bad message.
				continue
			}

			// Handle the message using the designated handler (potentially the full chain).
			respBytes, err := handlerFunc(ctx, msgBytes) // Use the handler function.
			if err != nil {
				// Log application-level errors returned by the handler.
				// createErrorResponse will log details including stack trace.
				s.logger.Warn("Error handling message.", "handlerError", err) // Keep this log concise.

				// Create and send a JSON-RPC error response.
				errResp, marshalErr := s.createErrorResponse(msgBytes, err)
				if marshalErr != nil {
					// If we can't even create the error response, log critical internal error.
					s.logger.Error("CRITICAL: Failed to create error response.", "marshalError", marshalErr, "originalError", fmt.Sprintf("%+v", err))
					continue // Continue processing other requests if possible.
				}

				// Send the error response.
				if writeErr := s.transport.WriteMessage(ctx, errResp); writeErr != nil {
					s.logger.Error("Failed to write error response.", "error", fmt.Sprintf("%+v", writeErr))
				}
				continue // Continue after sending error response.
			}

			// If there's a valid response (not nil), send it.
			// handleMessage returns nil for notifications.
			if respBytes != nil {
				if writeErr := s.transport.WriteMessage(ctx, respBytes); writeErr != nil {
					s.logger.Error("Failed to write success response.", "error", fmt.Sprintf("%+v", writeErr))
				}
			}
		}
	}
}

// handleMessage processes a single JSON-RPC message *after* it has passed
// through any preceding middleware (like validation).
// This function routes the request to the appropriate method handler.
func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	// Parse the JSON-RPC request structure.
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"` // Use RawMessage to handle string, number, or null.
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"` // Keep params raw for the handler.
	}

	// Unmarshal the basic structure. Middleware should ideally handle parse errors.
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		// This indicates a fundamental issue if validation middleware didn't catch it.
		// Return an error that createErrorResponse can map to JSONRPCParseError.
		return nil, transport.NewParseError(msgBytes, err)
	}

	// Basic JSON-RPC 2.0 check. Middleware should ideally handle this.
	if request.JSONRPC != "2.0" {
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Invalid JSON-RPC version, expected 2.0", nil)
	}

	// Find the registered handler for the method.
	handler, ok := s.methods[request.Method]
	if !ok {
		// Return an error that createErrorResponse can map to JSONRPCMethodNotFound.
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Method not found: "+request.Method, nil)
	}

	// Create a context for the request if needed (e.g., add request ID).
	// ctx = context.WithValue(ctx, "requestID", string(request.ID)) // Example.

	// Call the specific method handler.
	resultBytes, err := handler(ctx, request.Params)
	if err != nil {
		// Error occurred within the handler, bubble it up for central mapping.
		// Wrap it to add context about which method failed.
		return nil, errors.Wrapf(err, "error executing method '%s'", request.Method)
	}

	// Check if it was a notification (ID is null or absent).
	// Note: RawMessage treats absent ID as nil.
	if request.ID == nil || string(request.ID) == "null" {
		// It's a notification, no response is sent according to JSON-RPC spec.
		s.logger.Debug("Processed notification.", "method", request.Method)
		return nil, nil
	}

	// It's a request requiring a response, construct the success response.
	responseObj := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"` // Result from handler is already marshaled JSON.
	}{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  resultBytes, // Assign the raw JSON bytes from the handler.
	}

	// Marshal the final response object.
	respBytes, err := json.Marshal(responseObj)
	if err != nil {
		// This is a critical internal error if we can't marshal our own response.
		return nil, mcperrors.NewError(mcperrors.ErrProtocolInvalid, "Internal error: Failed to marshal success response", errors.WithStack(err))
	}

	return respBytes, nil
}

// createErrorResponse creates a JSON-RPC error response based on the provided error.
// It attempts to map internal errors (mcperrors, transport errors) to appropriate
// JSON-RPC error codes and logs detailed error information server-side.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	// Extract the request ID if possible, default to null if parsing fails.
	var requestID json.RawMessage = json.RawMessage("null") // Default to null ID.
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	if jsonErr := json.Unmarshal(msgBytes, &request); jsonErr == nil && request.ID != nil {
		requestID = request.ID
	} // Ignore jsonErr here, ID remains null if parsing failed.

	// Default JSON-RPC error code and message.
	code := transport.JSONRPCInternalError // -32603.
	message := "Internal server error."
	var data interface{} // Optional structured data (keep nil unless safe).

	// Check for specific error types to map codes and messages.
	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError // Check for schema validation errors.

	switch {
	case errors.As(err, &validationErr):
		// Map schema validation errors (typically Invalid Request or Invalid Params).
		if validationErr.Code == schema.ErrInvalidJSONFormat {
			code = transport.JSONRPCParseError // -32700.
			message = "Parse error."
		} else if validationErr.InstancePath != "" && (strings.Contains(validationErr.InstancePath, "/params") || strings.Contains(validationErr.InstancePath, "params")) {
			code = transport.JSONRPCInvalidParams // -32602.
			message = "Invalid params."
			data = validationErr.Context // Include sanitized validation context if available.
		} else {
			code = transport.JSONRPCInvalidRequest // -32600.
			message = "Invalid request."
			data = validationErr.Context // Include sanitized validation context if available.
		}
	case errors.As(err, &mcpErr):
		// Map custom MCP application errors.
		message = mcpErr.Message // Use the message from the custom error.
		// Map specific internal codes to JSON-RPC codes.
		switch mcpErr.Code {
		case mcperrors.ErrProtocolInvalid: // Example mapping.
			// Determine if it's MethodNotFound vs InvalidRequest based on message? Needs refinement.
			if strings.Contains(mcpErr.Message, "Method not found") {
				code = transport.JSONRPCMethodNotFound // -32601.
			} else {
				code = transport.JSONRPCInvalidRequest // -32600.
			}
		case mcperrors.ErrResourceNotFound:
			code = -32001 // Example implementation-defined code.
		case mcperrors.ErrAuthFailure:
			code = -32002 // Example implementation-defined code.
		// Add more mappings for other custom codes.
		default:
			// Keep default internal error or map to a generic implementation code.
			code = transport.JSONRPCInternalError // Or -32000.
		}
		// Optionally include sanitized context from mcpErr.Context in 'data'.
		// data = mcpErr.Context // Be cautious about leaking info.
	case errors.As(err, &transportErr):
		// Map transport errors (already handles common cases like ParseError).
		code, message, data = transport.MapErrorToJSONRPC(transportErr)
	default:
		// Fallback for generic Go errors (use default internal error).
		message = "An unexpected error occurred." // Avoid leaking raw error string to client.
	}

	// Log the detailed error server-side, including stack trace (aligns with ADR 001).
	// TODO: Add request_id, connection_id from context if available via middleware.
	s.logger.Error("Generating JSON-RPC error response.",
		"requestCode", code,
		"requestMessage", message,
		"originalError", fmt.Sprintf("%+v", err), // Log full details including stack trace.
		"responseData", data, // Log any data being sent back.
		"requestID", string(requestID))

	// Construct the JSON-RPC error payload.
	errorPayload := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"` // Use interface{} for flexibility.
	}{
		Code:    code,
		Message: message, // Use the mapped message.
		Data:    data,    // Assign mapped, sanitized data (can be nil).
	}

	// Construct the full JSON-RPC error response object.
	errorResponse := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"` // Use null for notifications or parse errors.
		Error   interface{}     `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      requestID, // Use extracted or null ID.
		Error:   errorPayload,
	}

	// Marshal the final response.
	responseBytes, marshalErr := json.Marshal(errorResponse)
	if marshalErr != nil {
		// This is a critical internal failure if we cannot marshal the error response.
		// Log it, but we probably can't send anything back to the client.
		s.logger.Error("CRITICAL: Failed to marshal final error response.", "marshalError", marshalErr)
		return nil, errors.Wrap(marshalErr, "failed to marshal error response object")
	}

	return responseBytes, nil
}

// ServeHTTP starts the server with an HTTP transport listening on the given address.
// This is typically used for standalone mode or when accessed remotely.
func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
	// TODO: Implement HTTP transport.
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented.")
}

// Shutdown initiates a graceful shutdown of the server.
// It waits for ongoing requests to complete up to the specified timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server.")

	// Close the transport if available.
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			// Log error but don't necessarily fail shutdown.
			s.logger.Error("Failed to close transport during shutdown.", "error", err)
			// Consider if this should return an error or just log.
			// return errors.Wrap(err, "failed to close transport")
		}
	}

	s.logger.Info("Server shutdown sequence completed.")
	return nil
}
