// file: internal/mcp/mcp_server.go
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
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/errors"
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

	// The handler for MCP methods.
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

	// Create the MCP method handler
	handler := NewMCPHandler(cfg, logger)

	// Create the server instance
	server := &Server{
		config:  cfg,
		options: opts,
		handler: handler,
		logger:  logger.WithField("component", "mcp_server"),
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

	s.logger.Info("Registered MCP methods",
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
	s.logger.Info("Starting server with stdio transport")

	// Create a transport using stdin/stdout
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, nil)

	// Serve requests until the context is canceled
	return s.serve(ctx)
}

// serve handles the main server loop, processing requests using the configured transport.
func (s *Server) serve(ctx context.Context) error {
	s.logger.Info("Server started, waiting for requests")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context canceled, shutting down")
			return ctx.Err()
		default:
			// Read a message from the transport
			msgBytes, err := s.transport.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) {
					s.logger.Info("Connection closed")
					return nil
				}
				s.logger.Error("Failed to read message", "error", err)
				continue
			}

			// Handle the message
			respBytes, err := s.handleMessage(ctx, msgBytes)
			if err != nil {
				s.logger.Error("Failed to handle message", "error", err)

				// Create an error response
				errResp, marshalErr := s.createErrorResponse(msgBytes, err)
				if marshalErr != nil {
					s.logger.Error("Failed to create error response", "error", marshalErr)
					continue
				}

				// Send the error response
				if err := s.transport.WriteMessage(ctx, errResp); err != nil {
					s.logger.Error("Failed to write error response", "error", err)
				}
				continue
			}

			// If there's a response, send it
			if respBytes != nil {
				if err := s.transport.WriteMessage(ctx, respBytes); err != nil {
					s.logger.Error("Failed to write response", "error", err)
				}
			}
		}
	}
}

// handleMessage processes a JSON-RPC message.
func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	// Parse the JSON-RPC request
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(msgBytes, &request); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON-RPC request")
	}

	// Verify JSON-RPC version
	if request.JSONRPC != "2.0" {
		return nil, mcperrors.NewError(
			mcperrors.ErrProtocolInvalid,
			"Invalid JSON-RPC version, expected 2.0",
			nil,
		)
	}

	// Find the method handler
	handler, ok := s.methods[request.Method]
	if !ok {
		return nil, mcperrors.NewError(
			mcperrors.ErrProtocolInvalid,
			"Method not found: "+request.Method,
			nil,
		)
	}

	// Call the method handler
	resultBytes, err := handler(ctx, request.Params)
	if err != nil {
		return nil, err
	}

	// Create the JSON-RPC response
	responseObj := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Result  json.RawMessage `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  resultBytes,
	}

	// Marshal the response
	respBytes, err := json.Marshal(responseObj)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal JSON-RPC response")
	}

	return respBytes, nil
}

// createErrorResponse creates a JSON-RPC error response.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	// Extract the request ID if possible
	var requestID json.RawMessage
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	if jsonErr := json.Unmarshal(msgBytes, &request); jsonErr == nil {
		requestID = request.ID
	}

	// Default error code and message
	code := -32603 // Internal error
	message := "Internal error"

	// Check for specific error types
	var mcpErr *mcperrors.BaseError
	if errors.As(err, &mcpErr) {
		code = mcpErr.Code
		message = mcpErr.Message
	} else {
		// Map other error types
		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			// Map transport errors to JSON-RPC codes
			code, message, _ = transport.MapErrorToJSONRPC(err)
		}
	}

	// Create the error response
	errorResponse := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data,omitempty"`
		} `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      requestID,
		Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data,omitempty"`
		}{
			Code:    code,
			Message: message,
			Data:    err.Error(),
		},
	}

	// Marshal the response
	return json.Marshal(errorResponse)
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

	// Close the transport if available
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			return errors.Wrap(err, "failed to close transport")
		}
	}

	return nil
}
