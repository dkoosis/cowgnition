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
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema" // Needed for validation error check.
	"github.com/dkoosis/cowgnition/internal/transport"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

// connectionStateKey is the context key for accessing the connection state.
const connectionStateKey contextKey = "connectionState"

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

	startTime time.Time

	// validator is the schema validator instance.
	validator middleware.SchemaValidatorInterface

	// connectionState tracks the protocol state of the MCP connection.
	connectionState *ConnectionState
}

// NewServer creates a new MCP server with the given configuration and options.
func NewServer(cfg *config.Config, opts ServerOptions, validator middleware.SchemaValidatorInterface, startTime time.Time, logger logging.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		return nil, errors.New("schema validator is required but was not provided to NewServer")
	}

	connState := NewConnectionState()

	// Create handler. Handler needs concrete validator if it uses specific methods not on interface.
	var concreteValidator *schema.SchemaValidator
	var ok bool
	if concreteValidator, ok = validator.(*schema.SchemaValidator); !ok {
		// If the provided validator isn't the concrete type the Handler expects, return error
		// Or, refactor Handler to only depend on the interface.
		return nil, errors.New("NewServer requires a concrete *schema.SchemaValidator instance for the Handler")
	}
	handler := NewHandler(cfg, concreteValidator, startTime, connState, logger)

	server := &Server{
		config:          cfg,
		options:         opts,
		handler:         handler,
		logger:          logger.WithField("component", "mcp_server"),
		methods:         make(map[string]MethodHandler),
		validator:       validator,
		startTime:       startTime,
		connectionState: connState,
	}

	server.registerMethods()

	return server, nil
}

// registerMethods registers all supported MCP methods using lowercase handler names.
func (s *Server) registerMethods() {
	s.methods["initialize"] = s.handler.handleInitialize
	s.methods["ping"] = s.handler.handlePing
	s.methods["notifications/initialized"] = s.handler.handleNotificationsInitialized
	s.methods["tools/list"] = s.handler.handleToolsList
	s.methods["tools/call"] = s.handler.handleToolCall
	s.methods["resources/list"] = s.handler.handleResourcesList
	s.methods["resources/read"] = s.handler.handleResourcesRead
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
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin)

	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = true

	if s.options.Debug {
		validationOpts.StrictOutgoing = true
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: using strict validation for incoming and outgoing messages")
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	chain := middleware.NewChain(s.handleMessage)
	chain.Use(validationMiddleware)

	serveHandler := chain.Handler()
	return s.serve(ctx, serveHandler)
}

// serve handles the main server loop.
func (s *Server) serve(ctx context.Context, handlerFunc transport.MessageHandler) error {
	s.logger.Info("Server processing loop started.")
	if handlerFunc == nil {
		return errors.New("serve called with nil handler function")
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context canceled, stopping server loop.")
			return ctx.Err()
		default:
			if err := s.processNextMessage(ctx, handlerFunc); err != nil {
				if s.isTerminalError(err) {
					return err
				}
				s.logger.Error("Error processing message", "error", fmt.Sprintf("%+v", err))
			}
		}
	}
}

// processNextMessage handles reading and processing a single message.
func (s *Server) processNextMessage(ctx context.Context, handlerFunc transport.MessageHandler) error {
	msgBytes, readErr := s.transport.ReadMessage(ctx)
	if readErr != nil {
		return s.handleTransportReadError(readErr)
	}

	ctxWithState := context.WithValue(ctx, connectionStateKey, s.connectionState)
	method, id := s.extractMessageInfo(msgBytes)

	respBytes, handleErr := handlerFunc(ctxWithState, msgBytes)
	if handleErr != nil {
		return s.handleProcessingError(ctx, msgBytes, method, id, handleErr)
	}

	if method == "initialize" && respBytes != nil {
		var respObj struct {
			Error *json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(respBytes, &respObj); err == nil && respObj.Error == nil {
			s.logger.Info("Initialize request successful, marking connection as initialized")
			s.connectionState.SetInitialized()
		} else if err != nil {
			s.logger.Warn("Failed to parse response during initialize state check", "error", err)
		} else {
			s.logger.Warn("Initialize request resulted in an error response, not marking connection as initialized.")
		}
	}

	if respBytes != nil {
		if err := s.writeResponse(ctx, respBytes, method, id); err != nil {
			return err
		}
	}

	return nil
}

// extractMessageInfo gets method name and ID from a message for logging.
func (s *Server) extractMessageInfo(msgBytes []byte) (method, id string) {
	// Removed the line: method, id = "", "unknown"

	var parsedReq struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
	}
	// Tolerate errors here, just for logging context
	_ = json.Unmarshal(msgBytes, &parsedReq)
	method = parsedReq.Method // Assign the parsed method
	if parsedReq.ID != nil {
		id = string(parsedReq.ID) // Assign the parsed ID if present
	} else {
		id = "unknown" // Assign default if ID is nil
	}
	return method, id
}

// handleTransportReadError processes errors from transport.ReadMessage.
func (s *Server) handleTransportReadError(readErr error) error {
	var transportErr *transport.Error
	isEOF := errors.Is(readErr, io.EOF)
	isClosedErr := errors.As(readErr, &transportErr) && transportErr.Code == transport.ErrTransportClosed
	isContextDone := errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded)

	if isEOF || isClosedErr || isContextDone {
		s.logger.Info("Connection closed or context done, exiting serve loop.", "reason", readErr)
		return readErr
	}

	s.logger.Error("Failed to read message, continuing loop.", "error", fmt.Sprintf("%+v", readErr))
	return nil
}

// handleProcessingError handles errors during message processing.
func (s *Server) handleProcessingError(ctx context.Context, msgBytes []byte, method, id string, handleErr error) error {
	s.logger.Warn("Error processing message via handler.",
		"method", method,
		"requestID", id,
		"error", fmt.Sprintf("%+v", handleErr))

	errRespBytes, creationErr := s.createErrorResponse(msgBytes, handleErr)
	if creationErr != nil {
		s.logger.Error("CRITICAL: Failed to create error response.",
			"creationError", fmt.Sprintf("%+v", creationErr),
			"originalError", fmt.Sprintf("%+v", handleErr))
		return errors.Wrap(creationErr, "failed to marshal error response")
	}

	return s.writeResponse(ctx, errRespBytes, method, id)
}

// writeResponse sends a response through the transport.
func (s *Server) writeResponse(ctx context.Context, respBytes []byte, method, id string) error {
	if writeErr := s.transport.WriteMessage(ctx, respBytes); writeErr != nil {
		s.logger.Error("Failed to write response.",
			"method", method,
			"requestID", id,
			"error", fmt.Sprintf("%+v", writeErr))
		return writeErr
	}
	return nil
}

// isTerminalError checks if an error should terminate the server loop.
func (s *Server) isTerminalError(err error) bool {
	var transportErr *transport.Error
	if errors.As(err, &transportErr) &&
		(transportErr.Code == transport.ErrTransportClosed || transportErr.Code == transport.ErrWriteTimeout) {
		return true
	}
	return errors.Is(err, io.EOF) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// handleMessage processes a single *validated* JSON-RPC message.
func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(msgBytes, &request); err != nil {
		return nil, errors.Wrap(err, "internal error: failed to parse validated message in handleMessage")
	}

	// Validate method against connection state first.
	if err := s.connectionState.ValidateMethodSequence(request.Method); err != nil {
		// Just wrap the error; mapping logic will check the string content later
		return nil, errors.Wrapf(err, "method sequence validation failed")
	}

	handler, ok := s.methods[request.Method]
	if !ok {
		// Return a standard Go error indicating method not found
		// The mapping logic will detect this string later
		return nil, errors.Newf("Method not found: %s", request.Method)
	}

	resultBytes, handlerErr := handler(ctx, request.Params)
	if handlerErr != nil {
		return nil, errors.Wrapf(handlerErr, "error executing method '%s'", request.Method)
	}

	if request.ID == nil || string(request.ID) == "null" {
		s.logger.Debug("Processed notification.", "method", request.Method)
		return nil, nil
	}

	responseObj := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  resultBytes,
	}

	respBytes, marshalErr := json.Marshal(responseObj)
	if marshalErr != nil {
		return nil, errors.Wrap(marshalErr, "internal error: failed to marshal success response")
	}

	return respBytes, nil
}

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	requestID := extractRequestID(msgBytes)
	code, message, data := s.mapErrorToJSONRPCComponents(err)
	s.logErrorDetails(code, message, requestID, data, err)

	errorPayload := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	}{
		Code:    code,
		Message: message,
		Data:    data,
	}

	errorResponse := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   interface{}     `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      requestID,
		Error:   errorPayload,
	}

	responseBytes, marshalErr := json.Marshal(errorResponse)
	if marshalErr != nil {
		s.logger.Error("CRITICAL: Failed to marshal final error response.", "marshalError", fmt.Sprintf("%+v", marshalErr))
		return nil, errors.Wrap(marshalErr, "failed to marshal error response object")
	}

	return responseBytes, nil
}

// extractRequestID attempts to get the ID from raw message bytes.
func extractRequestID(msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(msgBytes, &request) // Ignore error, default to null
	if request.ID != nil {
		return request.ID
	}
	return json.RawMessage("null")
}

// file: internal/mcp/mcp_server.go

// mapErrorToJSONRPCComponents maps Go errors to JSON-RPC code, message, and optional data.
func (s *Server) mapErrorToJSONRPCComponents(err error) (code int, message string, data interface{}) {
	data = nil // Initialize data

	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError

	// Use errors.Cause to get the root error before checking its string representation
	// This makes the check robust even if the error gets wrapped.
	rootErr := errors.Cause(err)
	errStr := rootErr.Error() // Get the string of the root cause

	// Check for specific error strings first for method not found/sequence errors
	if strings.Contains(errStr, "Method not found:") {
		code = transport.JSONRPCMethodNotFound // -32601
		message = "Method not found."
		// Try to extract method name for data field
		methodName := strings.TrimPrefix(errStr, "Method not found: ")
		if methodName != errStr { // Check if prefix was actually removed
			data = map[string]interface{}{
				"method": methodName,
				"detail": "The requested method is not supported by this MCP server.",
			}
		}
	} else if strings.Contains(errStr, "protocol sequence error:") {
		code = transport.JSONRPCMethodNotFound // -32601 to match expected test value
		message = "Connection initialization required."

		// Extract the detailed error information
		// Include the full sequence error message in the data
		data = map[string]interface{}{
			"detail": errStr,
			"state":  s.connectionState.CurrentState(),
		}

		// Check for specific protocol sequence errors to add targeted help
		if strings.Contains(errStr, "must first call 'initialize'") {
			data = map[string]interface{}{
				"detail":    errStr,
				"state":     s.connectionState.CurrentState(),
				"help":      "The MCP protocol requires initialize to be called first to establish connection capabilities.",
				"reference": "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization",
			}
		} else if strings.Contains(errStr, "can only be called once") {
			data = map[string]interface{}{
				"detail":    errStr,
				"state":     s.connectionState.CurrentState(),
				"help":      "The initialize method can only be called once per connection.",
				"reference": "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization",
			}
		}
	} else if strings.Contains(errStr, "connection not initialized") {
		// This is a fallback for other connection state errors that may not use the new format
		code = transport.JSONRPCMethodNotFound // -32601
		message = "Connection initialization required."
		data = map[string]interface{}{
			"detail": errStr,
			"state":  s.connectionState.CurrentState(),
			"help":   "The MCP protocol requires initialize to be called first to establish connection capabilities.",
		}

		// Then check specific error types
	} else if errors.As(err, &validationErr) {
		code, message, data = mapValidationError(validationErr)
	} else if errors.As(err, &mcpErr) {
		code, message = mapMCPError(mcpErr) // Passes MCP context if needed
		if mcpErr.Context != nil {
			data = mcpErr.Context
		}
	} else if errors.As(err, &transportErr) {
		code, message, data = transport.MapErrorToJSONRPC(transportErr)
	} else {
		// Default for generic Go errors
		code, message = mapGenericGoError(err)
	}

	return code, message, data
}

// mapValidationError maps schema.ValidationError to JSON-RPC components.
func mapValidationError(validationErr *schema.ValidationError) (code int, message string, data interface{}) {
	if validationErr.Code == schema.ErrInvalidJSONFormat {
		code = transport.JSONRPCParseError
		message = "Parse error."
	} else if validationErr.InstancePath != "" && (strings.Contains(validationErr.InstancePath, "/params") || strings.Contains(validationErr.InstancePath, "params")) {
		code = transport.JSONRPCInvalidParams
		message = "Invalid params."
	} else {
		code = transport.JSONRPCInvalidRequest
		message = "Invalid request."
	}
	data = map[string]interface{}{
		"validationPath":  validationErr.InstancePath,
		"validationError": validationErr.Message,
	}
	return code, message, data
}

// mapMCPError maps mcperrors.BaseError to JSON-RPC code and message.
func mapMCPError(mcpErr *mcperrors.BaseError) (code int, message string) {
	message = mcpErr.Message
	code = mcpErr.Code

	// Map specific internal codes to standard or implementation-defined JSON-RPC codes.
	switch mcpErr.Code {
	// ErrMethodNotFound handled by string check earlier
	case mcperrors.ErrProtocolInvalid:
		code = transport.JSONRPCInvalidRequest
	case mcperrors.ErrResourceNotFound:
		code = -32001
	case mcperrors.ErrAuthFailure:
		code = -32002
	case mcperrors.ErrRTMAPIFailure:
		code = -32010
	default:
		if code < -32099 || code > -32000 {
			code = -32000 // Generic implementation-defined server error.
		}
	}
	return code, message
}

// mapGenericGoError maps generic Go errors.
func mapGenericGoError(err error) (code int, message string) {
	code = transport.JSONRPCInternalError
	message = "An unexpected internal server error occurred."
	return code, message
}

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, requestID json.RawMessage, data interface{}, err error) {
	logArgs := []interface{}{
		"jsonrpcErrorCode", code,
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err),
		"requestID", string(requestID),
	}
	if data != nil {
		logArgs = append(logArgs, "responseData", data)
	}

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
		logArgs = append(logArgs, "validationInstancePath", validationErr.InstancePath)
		logArgs = append(logArgs, "validationSchemaPath", validationErr.SchemaPath)
		if len(validationErr.Context) > 0 {
			logArgs = append(logArgs, "validationErrorContext", validationErr.Context)
		}
	}

	s.logger.Error("Generating JSON-RPC error response.", logArgs...)
}

// ServeHTTP starts the server with an HTTP transport listening on the given address.
func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server.")
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			s.logger.Error("Failed to close transport during shutdown.", "error", fmt.Sprintf("%+v", err))
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}
	s.logger.Info("Server shutdown sequence completed.")
	return nil
}
