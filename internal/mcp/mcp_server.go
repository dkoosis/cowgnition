// file: internal/mcp/mcp_server.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors" // Ensure cockroachdb/errors is imported.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Corrected import path.
	"github.com/dkoosis/cowgnition/internal/middleware"               // Import middleware package.
	"github.com/dkoosis/cowgnition/internal/schema"                   // Needed for validation error check.
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
	handler *Handler

	// Method map for routing requests.
	methods map[string]MethodHandler

	// Transport for communication.
	transport transport.Transport

	// Logger for server events.
	logger logging.Logger

	// validator is the schema validator instance.
	validator *schema.SchemaValidator
}

// NewServer creates a new MCP server with the given configuration and options.
// It now requires an initialized schema validator.
func NewServer(cfg *config.Config, opts ServerOptions, validator *schema.SchemaValidator, logger logging.Logger) (*Server, error) {
	// Ensure logger is initialized.
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	// Ensure validator is provided, as it's now essential.
	if validator == nil {
		return nil, errors.New("schema validator is required but was not provided to NewServer")
	}

	// Create the MCP method handler.
	handler := NewHandler(cfg, logger)

	// Create the server instance.
	server := &Server{
		config:    cfg,
		options:   opts,
		handler:   handler,
		logger:    logger.WithField("component", "mcp_server"),
		methods:   make(map[string]MethodHandler),
		validator: validator, // Store the validator.
	}

	// Register method handlers.
	server.registerMethods()

	return server, nil
}

// file: internal/mcp/mcp_server.go (partial update to registerMethods function)

// registerMethods registers all supported MCP methods using lowercase handler names.
func (s *Server) registerMethods() {
	// Core MCP methods.
	s.methods["initialize"] = s.handler.handleInitialize
	s.methods["ping"] = s.handler.handlePing

	// Notifications methods (important for protocol handshake).
	s.methods["notifications/initialized"] = s.handler.handleNotificationsInitialized

	// Tools methods.
	s.methods["tools/list"] = s.handler.handleToolsList
	s.methods["tools/call"] = s.handler.handleToolCall

	// Resources methods.
	s.methods["resources/list"] = s.handler.handleResourcesList
	s.methods["resources/read"] = s.handler.handleResourcesRead

	// Prompts methods (required for complete MCP dialog).
	s.methods["prompts/list"] = s.handler.handlePromptsList
	s.methods["prompts/get"] = s.handler.handlePromptsGet

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
// This version integrates the validation middleware.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")
	// Use Stdin as closer for stdio transport.
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin)

	// --- Create Middleware Chain ---
	// Configure validation middleware options (can be customized).
	validationOpts := middleware.DefaultValidationOptions()
	// Ensure strict mode is enabled for protocol compliance.
	validationOpts.StrictMode = true
	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"), // Give middleware its own logger context.
	)

	// Create the chain with handleMessage as the final handler.
	chain := middleware.NewChain(s.handleMessage)
	// Add validation middleware first in the chain.
	chain.Use(validationMiddleware)
	// Add other middleware here if needed (e.g., logging, auth).
	serveHandler := chain.Handler() // Get the composed handler.
	// --- End Middleware Chain ---

	// Pass the chained handler to the main serve loop.
	return s.serve(ctx, serveHandler)
}

// serve handles the main server loop, processing requests using the configured transport.
// It now accepts the handler function representing the full middleware chain.
func (s *Server) serve(ctx context.Context, handlerFunc transport.MessageHandler) error {
	s.logger.Info("Server processing loop started.")

	// Ensure the handler is not nil.
	if handlerFunc == nil {
		return errors.New("serve called with nil handler function")
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context canceled, stopping server loop.")
			return ctx.Err() // Return context error.
		default:
			// Read a message from the transport.
			msgBytes, readErr := s.transport.ReadMessage(ctx)
			if readErr != nil {
				// Check for expected closure errors.
				var transportErr *transport.Error
				isEOF := errors.Is(readErr, io.EOF)
				isClosedErr := errors.As(readErr, &transportErr) && transportErr.Code == transport.ErrTransportClosed
				isContextDone := errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded)

				if isEOF || isClosedErr || isContextDone {
					s.logger.Info("Connection closed or context done, exiting serve loop.", "reason", readErr)
					return nil // Clean exit for expected closures or context cancellation.
				}

				// Log other transport read errors. Use %+v for stack trace if available.
				s.logger.Error("Failed to read message, continuing loop.", "error", fmt.Sprintf("%+v", readErr))
				// Continue processing other messages if possible after a read error.
				continue
			}

			// Process the message using the provided handlerFunc (middleware chain).
			respBytes, handleErr := handlerFunc(ctx, msgBytes)

			if handleErr != nil {
				// Errors from the handler chain (including validation or method execution).
				s.logger.Warn("Error processing message via handler.", "error", fmt.Sprintf("%+v", handleErr))

				// Create and send JSON-RPC error response based on the handler error.
				errRespBytes, creationErr := s.createErrorResponse(msgBytes, handleErr)
				if creationErr != nil {
					s.logger.Error("CRITICAL: Failed to create error response.", "creationError", fmt.Sprintf("%+v", creationErr), "originalError", fmt.Sprintf("%+v", handleErr))
					// Exit on critical marshal error as we can't inform the client.
					return errors.Wrap(creationErr, "failed to marshal error response")
				}

				// Attempt to write the error response.
				if writeErr := s.transport.WriteMessage(ctx, errRespBytes); writeErr != nil {
					s.logger.Error("Failed to write error response.", "error", fmt.Sprintf("%+v", writeErr))
					// Depending on the error, may want to continue or return writeErr.
					// Failure to write might indicate a broken connection.
				}
				continue // Continue processing next message after handling error.
			}

			// If handlerFunc returns non-nil response bytes (i.e., not a notification), send them.
			if respBytes != nil {
				if writeErr := s.transport.WriteMessage(ctx, respBytes); writeErr != nil {
					s.logger.Error("Failed to write success response.", "error", fmt.Sprintf("%+v", writeErr))
					// Failure to write might indicate a broken connection.
				}
			}
			// If respBytes is nil (e.g., for notifications), do nothing and continue.
		}
	}
}

// handleMessage processes a single *validated* JSON-RPC message.
// This acts as the final handler in the middleware chain.
func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	// Parse the JSON-RPC request structure (already validated by middleware).
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"` // Use RawMessage for flexibility.
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"` // Keep params raw for the specific handler.
	}

	// Basic unmarshal; should succeed as middleware validated JSON syntax.
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		// This indicates an internal error post-validation.
		return nil, errors.Wrap(err, "internal error: failed to parse validated message in handleMessage")
	}

	// Find the registered handler for the method.
	handler, ok := s.methods[request.Method]
	if !ok {
		// *** CORRECTED ERROR CREATION ***
		// Directly create a BaseError using the ErrProtocolInvalid code.
		err := &mcperrors.BaseError{
			Code:    mcperrors.ErrProtocolInvalid,
			Message: "Method not found: " + request.Method,
			Cause:   nil, // No underlying cause here.
			Context: map[string]interface{}{"method": request.Method},
		}
		// Return the specific MCP error.
		return nil, err
	}

	// Add request context if desired (e.g., request ID).
	// ctx = context.WithValue(ctx, "requestID", string(request.ID)).

	// Call the specific method handler.
	resultBytes, handlerErr := handler(ctx, request.Params)
	if handlerErr != nil {
		// Error occurred within the specific method handler. Bubble it up.
		// Wrap to add context about which method failed.
		return nil, errors.Wrapf(handlerErr, "error executing method '%s'", request.Method)
	}

	// Handle Notifications (no response needed).
	// Check if ID is absent or explicitly null.
	if request.ID == nil || string(request.ID) == "null" {
		s.logger.Debug("Processed notification.", "method", request.Method)
		return nil, nil // Return nil bytes, nil error for notifications.
	}

	// Construct Success Response for requests.
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
	respBytes, marshalErr := json.Marshal(responseObj)
	if marshalErr != nil {
		// This is a critical internal error.
		return nil, errors.Wrap(marshalErr, "internal error: failed to marshal success response")
	}

	return respBytes, nil
}

// createErrorResponse creates the byte representation of a JSON-RPC error response.
// It maps the incoming Go error to the appropriate JSON-RPC error structure.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	// Extract request ID from the original message, defaulting to null if unavailable.
	requestID := extractRequestID(msgBytes)

	// Determine the JSON-RPC error code, message, and data based on the error type.
	code, message, data := s.mapErrorToJSONRPCComponents(err)

	// Log the detailed error server-side, including stack trace if available.
	s.logErrorDetails(code, message, requestID, data, err)

	// Construct the JSON-RPC error payload.
	errorPayload := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"` // Include data only if it's non-nil.
	}{
		Code:    code,
		Message: message, // Use the mapped message.
		Data:    data,    // Use the mapped data.
	}

	// Construct the full JSON-RPC error response object.
	errorResponse := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   interface{}     `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      requestID, // Use extracted or null ID.
		Error:   errorPayload,
	}

	// Marshal the final response.
	responseBytes, marshalErr := json.Marshal(errorResponse)
	if marshalErr != nil {
		// This is a critical internal failure.
		s.logger.Error("CRITICAL: Failed to marshal final error response.", "marshalError", fmt.Sprintf("%+v", marshalErr))
		// Wrap the marshaling error.
		return nil, errors.Wrap(marshalErr, "failed to marshal error response object")
	}

	return responseBytes, nil
}

// extractRequestID attempts to get the ID from raw message bytes.
// Returns json.RawMessage("null") if parsing fails or ID is absent/null.
func extractRequestID(msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	// Use the original message bytes to attempt ID extraction.
	// Ignore error, default to null if parsing fails.
	_ = json.Unmarshal(msgBytes, &request)
	if request.ID != nil {
		return request.ID
	}
	// Default to null ID if ID is absent or explicitly null in the original message.
	return json.RawMessage("null")
}

// mapErrorToJSONRPCComponents maps Go errors to JSON-RPC code, message, and optional data.
// This implements the core error mapping logic as per ADR 001.
func (s *Server) mapErrorToJSONRPCComponents(err error) (code int, message string, data interface{}) {
	// Initialize data to nil. It will only be populated if specific mapping logic adds it.
	data = nil

	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError

	// Check error types in order of specificity using errors.As.
	switch {
	case errors.As(err, &validationErr):
		code, message, data = mapValidationError(validationErr)
	case errors.As(err, &mcpErr):
		code, message = mapMCPError(mcpErr)
		// data remains nil for MCP errors unless mapMCPError is modified.
	case errors.As(err, &transportErr):
		code, message, data = transport.MapErrorToJSONRPC(transportErr)
	default:
		// Handle generic Go errors.
		code, message = mapGenericGoError(err)
	}

	return code, message, data
}

// mapValidationError maps schema.ValidationError to JSON-RPC components.
func mapValidationError(validationErr *schema.ValidationError) (code int, message string, data interface{}) {
	if validationErr.Code == schema.ErrInvalidJSONFormat {
		code = transport.JSONRPCParseError // -32700.
		message = "Parse error."
	} else if validationErr.InstancePath != "" && (strings.Contains(validationErr.InstancePath, "/params") || strings.Contains(validationErr.InstancePath, "params")) {
		// If the validation error path points to parameters, use Invalid Params.
		code = transport.JSONRPCInvalidParams // -32602.
		message = "Invalid params."
	} else {
		// Otherwise, it's likely a structural issue, use Invalid Request.
		code = transport.JSONRPCInvalidRequest // -32600.
		message = "Invalid request."
	}
	// Include structured validation details in the data field.
	data = map[string]interface{}{
		"validationPath":  validationErr.InstancePath, // Path within the JSON data.
		"validationError": validationErr.Message,      // Specific error message from validator.
		// Consider adding schemaPath if useful: "schemaPath": validationErr.SchemaPath,.
	}
	return code, message, data
}

// mapMCPError maps mcperrors.BaseError to JSON-RPC code and message.
// It does not return data currently, adhering to previous fix.
func mapMCPError(mcpErr *mcperrors.BaseError) (code int, message string) {
	message = mcpErr.Message // Use the message from the custom error.
	code = mcpErr.Code       // Use the code from the custom error.

	// Map specific internal codes to standard JSON-RPC codes where applicable.
	switch mcpErr.Code {
	case mcperrors.ErrProtocolInvalid:
		if strings.Contains(mcpErr.Message, "Method not found") {
			code = transport.JSONRPCMethodNotFound // -32601.
		} else {
			// Other protocol issues map to Invalid Request.
			code = transport.JSONRPCInvalidRequest // -32600.
		}
	// Map application-specific codes to the implementation-defined range.
	case mcperrors.ErrResourceNotFound:
		code = -32001 // Example: Resource not found.
	case mcperrors.ErrAuthFailure:
		code = -32002 // Example: Authentication required/failed.
	case mcperrors.ErrRTMAPIFailure:
		code = -32010 // Example: RTM API interaction failed.
	// Add more mappings as needed.
	default:
		// If it's an MCP error but not specifically mapped, use a generic server error code
		// unless it's already in the valid custom range.
		if code < -32099 || code > -32000 {
			code = -32000 // Generic implementation-defined server error.
		}
	}
	// data = mcpErr.Context // Keep commented: optionally include *sanitized* context later.
	return code, message
}

// mapGenericGoError maps generic Go errors (non-MCP, non-transport, non-validation)
// to JSON-RPC code and message.
func mapGenericGoError(err error) (code int, message string) {
	// Default to Internal Error.
	code = transport.JSONRPCInternalError // -32603.
	errMsg := err.Error()                 // Get the Go error message.

	// Try to map common underlying error strings to standard codes.
	// This is less reliable than type assertions but provides some fallback mapping.
	// Note: Order matters here if messages overlap.
	if strings.Contains(errMsg, "method not found") { // Check specific strings.
		code = transport.JSONRPCMethodNotFound
		message = "Method not found."
	} else if strings.Contains(errMsg, "invalid character") || strings.Contains(errMsg, "unexpected EOF") || strings.Contains(errMsg, "invalid JSON syntax") {
		code = transport.JSONRPCParseError
		message = "Parse error."
	} else if strings.Contains(errMsg, "invalid params") { // Check specific strings.
		code = transport.JSONRPCInvalidParams
		message = "Invalid params."
	} else if strings.Contains(errMsg, "invalid JSON-RPC version") || strings.Contains(errMsg, "invalid request") { // Check specific strings.
		code = transport.JSONRPCInvalidRequest
		message = "Invalid request."
	} else {
		// For truly unexpected internal errors, provide a generic message.
		// Do not leak the raw Go error string (errMsg) to the client.
		message = "An unexpected internal server error occurred."
	}
	return code, message
}

// logErrorDetails logs detailed error information server-side.
// Follows ADR 001 guidelines, using structured logging and including stack traces.
func (s *Server) logErrorDetails(code int, message string, requestID json.RawMessage, data interface{}, err error) {
	// Base arguments for structured logging.
	// Use %+v for 'originalError' to include stack trace from cockroachdb/errors.
	logArgs := []interface{}{
		"jsonrpcErrorCode", code, // Use distinct key for JSON-RPC code.
		"jsonrpcErrorMessage", message, // Use distinct key for JSON-RPC message.
		"originalError", fmt.Sprintf("%+v", err), // Log full details including stack trace.
		"responseData", data, // Log any data being sent back.
		"requestID", string(requestID), // Log request ID if available.
		// TODO: Add connection_id from context if available via middleware.
	}

	// Extract and add details from specific structured error types.
	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError

	if errors.As(err, &mcpErr) {
		logArgs = append(logArgs, "internalErrorCode", mcpErr.Code)
		if len(mcpErr.Context) > 0 {
			logArgs = append(logArgs, "internalErrorContext", mcpErr.Context)
		}
	} else if errors.As(err, &transportErr) {
		logArgs = append(logArgs, "transportErrorCode", transportErr.Code)
		if len(transportErr.Context) > 0 {
			logArgs = append(logArgs, "transportErrorContext", transportErr.Context)
		}
	} else if errors.As(err, &validationErr) {
		logArgs = append(logArgs, "validationErrorCode", validationErr.Code)
		if len(validationErr.Context) > 0 {
			logArgs = append(logArgs, "validationErrorContext", validationErr.Context)
		}
		logArgs = append(logArgs, "validationInstancePath", validationErr.InstancePath)
		logArgs = append(logArgs, "validationSchemaPath", validationErr.SchemaPath)
	}

	// Log as an error level event.
	s.logger.Error("Generating JSON-RPC error response.", logArgs...)
}

// ServeHTTP starts the server with an HTTP transport listening on the given address.
// This is currently a placeholder.
func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
	// TODO: Implement HTTP transport, including middleware chain integration.
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server.")

	// Close the transport if available.
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			// Log error but don't necessarily fail shutdown.
			s.logger.Error("Failed to close transport during shutdown.", "error", fmt.Sprintf("%+v", err))
			// Depending on transport implementation, this might be critical or recoverable.
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}

	// Add any other resource cleanup here (e.g., database connections).

	s.logger.Info("Server shutdown sequence completed.")
	return nil
}
