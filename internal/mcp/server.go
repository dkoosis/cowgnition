// file: internal/mcp/server.go
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/middleware"
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

// Server represents an MCP (Model Context Protocol) server instance.
// It handles communication with clients via the protocol.
type Server struct {
	// Configuration for the server.
	config *config.Config

	// Server options.
	options ServerOptions

	// Handler for MCP methods
	handler *Handler

	// Transport for communication
	transport transport.Transport

	// Logger for server events
	logger logging.Logger

	// Context for the server lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Method registry
	methods map[string]MethodHandler
}

// MethodHandler is a function that processes a specific MCP method.
type MethodHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// NewServer creates a new MCP server with the given configuration and options.
func NewServer(cfg *config.Config, opts ServerOptions, logger logging.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create the handler
	handler := NewHandler(logger)

	// Create the server instance
	server := &Server{
		config:  cfg,
		options: opts,
		handler: handler,
		logger:  logger.WithField("component", "mcp_server"),
		ctx:     ctx,
		cancel:  cancel,
		methods: make(map[string]MethodHandler),
	}

	// Register method handlers
	server.registerMethods()

	return server, nil
}

// registerMethods registers all supported MCP methods.
func (s *Server) registerMethods() {
	// Core MCP methods
	s.methods["initialize"] = s.handler.HandleInitialize
	s.methods["ping"] = s.handler.HandlePing

	// Tools methods
	s.methods["tools/list"] = s.handler.HandleToolsList
	s.methods["tools/call"] = s.handler.HandleToolCall

	// Resources methods
	s.methods["resources/list"] = s.handler.HandleResourcesList
	s.methods["resources/read"] = s.handler.HandleResourcesRead
}

// ServeSTDIO starts the server using standard input/output as the transport.
// This is typically used when the server is launched by a client like Claude Desktop.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport")

	// Create a transport using stdin/stdout
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, nil)

	// Create a schema validator
	schemaSource := schema.Source{
		// URL for the schema
		URL: "https://raw.githubusercontent.com/anthropics/ModelContextProtocol/main/schema/schema.json",
	}
	validator := schema.NewValidator(schemaSource, s.logger)

	// Initialize the validator
	if err := validator.Initialize(ctx); err != nil {
		return errors.Wrap(err, "failed to initialize schema validator")
	}

	// Create validation middleware
	validationOpts := middleware.DefaultValidationOptions()
	if s.options.Debug {
		validationOpts.MeasurePerformance = true
	}
	validationMiddleware := middleware.NewValidationMiddleware(validator, validationOpts, s.logger)

	// Create the middleware chain
	chain := middleware.NewChain(s.handleMessage)
	chain.Use(validationMiddleware)

	// Start serving requests
	handler := chain.Handler()

	// Process messages until context is canceled or an error occurs
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Read a message from the transport
			message, err := s.transport.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) {
					s.logger.Info("Connection closed")
					return nil
				}
				s.logger.Error("Failed to read message", "error", err)
				// If it's a transport-level error, try to continue
				continue
			}

			// Process the message through the middleware chain
			response, err := handler(ctx, message)
			if err != nil {
				s.logger.Error("Failed to process message", "error", err)
				// Create JSON-RPC error response
				errorResp, respErr := s.createErrorResponse(message, err)
				if respErr != nil {
					s.logger.Error("Failed to create error response", "error", respErr)
					continue
				}

				// Write the error response
				if err := s.transport.WriteMessage(ctx, errorResp); err != nil {
					s.logger.Error("Failed to write error response", "error", err)
				}
				continue
			}

			// If there's a response to send, write it
			if response != nil {
				if err := s.transport.WriteMessage(ctx, response); err != nil {
					s.logger.Error("Failed to write response", "error", err)
				}
			}
		}
	}
}

// createErrorResponse creates a JSON-RPC error response from an error.
func (s *Server) createErrorResponse(message []byte, err error) ([]byte, error) {
	// Try to extract the request ID from the original message
	var req map[string]json.RawMessage
	var id interface{}

	if err := json.Unmarshal(message, &req); err == nil {
		if idRaw, ok := req["id"]; ok {
			_ = json.Unmarshal(idRaw, &id)
		}
	}

	// Map error to JSON-RPC error code and message
	code := -32603 // Internal error by default
	msg := "Internal error"

	// TODO: Implement more detailed error mapping

	// Create the error response
	errResp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": msg,
			"data": map[string]interface{}{
				"details": err.Error(),
			},
		},
	}

	// Marshal to JSON
	return json.Marshal(errResp)
}

// handleMessage processes a JSON-RPC message after it has passed through middleware.
func (s *Server) handleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Parse the message as JSON-RPC
	var jsonRPC map[string]json.RawMessage
	if err := json.Unmarshal(message, &jsonRPC); err != nil {
		return nil, errors.Wrap(err, "failed to parse JSON-RPC message")
	}

	// Check if it's a request (has 'method')
	methodBytes, hasMethod := jsonRPC["method"]
	if !hasMethod {
		// It's a response or notification, which we don't handle yet
		s.logger.Warn("Received non-request message, ignoring")
		return nil, nil
	}

	// Extract the method name
	var method string
	if err := json.Unmarshal(methodBytes, &method); err != nil {
		return nil, errors.Wrap(err, "failed to parse method name")
	}

	// Extract the request ID if present
	var id interface{}
	if idBytes, hasID := jsonRPC["id"]; hasID {
		if err := json.Unmarshal(idBytes, &id); err != nil {
			return nil, errors.Wrap(err, "failed to parse request ID")
		}
	}

	// Extract params if present
	var params json.RawMessage
	if paramsBytes, hasParams := jsonRPC["params"]; hasParams {
		params = paramsBytes
	}

	// Find the method handler
	handler, exists := s.methods[method]
	if !exists {
		return nil, errors.Newf("method not found: %s", method)
	}

	// Call the method handler
	result, err := handler(ctx, params)
	if err != nil {
		return nil, errors.Wrapf(err, "error handling method: %s", method)
	}

	// Create a JSON-RPC success response
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}

	// Convert to JSON
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}

	return responseBytes, nil
}

// ServeHTTP starts the server with an HTTP transport listening on the given address.
// This is typically used for standalone mode or when accessed remotely.
func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
	// TODO: Implement HTTP transport
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
// It waits for ongoing requests to complete up to the specified timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server")

	// Cancel our context to stop any ongoing operations
	s.cancel()

	// Close the transport if available
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			return errors.Wrap(err, "failed to close transport")
		}
	}

	return nil
}
