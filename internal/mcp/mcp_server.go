// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os" // Keep os import.
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/middleware" // Need middleware for chain/options.

	// Import schema package to use the interface defined there.
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

// connectionStateKey is the context key for accessing the connection state.
const connectionStateKey contextKey = "connectionState"

// ServerOptions contains configurable options for the MCP server.
type ServerOptions struct {
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	Debug           bool
}

// MethodHandler is a function type for handling MCP method calls.
type MethodHandler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)

// Server represents an MCP (Model Context Protocol) server instance.
type Server struct {
	config    *config.Config
	options   ServerOptions
	handler   *Handler                 // Handles the actual method logic.
	methods   map[string]MethodHandler // Registered method handlers.
	transport transport.Transport
	logger    logging.Logger
	startTime time.Time
	// Corrected: Use the interface defined in the schema package (now ValidatorInterface).
	validator       schema.ValidatorInterface
	connectionState *ConnectionState
}

// NewServer creates a new MCP server instance.
// Corrected: Accepts schema.ValidatorInterface.
func NewServer(cfg *config.Config, opts ServerOptions, validator schema.ValidatorInterface, startTime time.Time, logger logging.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		return nil, errors.New("schema validator is required but was not provided to NewServer")
	}

	connState := NewConnectionState()

	// Pass the validator interface to the handler.
	handler := NewHandler(cfg, validator, startTime, connState, logger)

	server := &Server{
		config:          cfg,
		options:         opts,
		handler:         handler,
		logger:          logger.WithField("component", "mcp_server"),
		methods:         make(map[string]MethodHandler),
		validator:       validator, // Store the interface.
		startTime:       startTime,
		connectionState: connState,
	}

	server.registerMethods() // Register methods provided by the handler.

	return server, nil
}

// registerMethods populates the server's method map.
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
	// ... register other methods ...

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

// ServeSTDIO configures and starts the server using stdio transport.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin, s.logger)

	// Setup validation middleware using internal/middleware package.
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = true

	if s.options.Debug {
		validationOpts.StrictOutgoing = true
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: using strict validation for incoming and outgoing messages.")
	} else {
		validationOpts.StrictOutgoing = false
	}

	// Corrected: Pass the validator (which implements schema.ValidatorInterface).
	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	// Build middleware chain.
	chain := middleware.NewChain(s.handleMessage)
	chain.Use(validationMiddleware)

	serveHandler := chain.Handler()

	return s.serve(ctx, serveHandler)
}

// ServeHTTP starts the server with an HTTP transport (Placeholder).
func (s *Server) ServeHTTP(_ context.Context, _ string) error {
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(_ context.Context) error {
	s.logger.Info("Shutting down MCP server.")
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

// --- mcp_server_processing.go and mcp_server_error_handling.go content should be in their respective files ---.
// --- Ensure handleMessage uses the correct validator interface if needed directly ---.
// --- (Though likely validation happens in middleware before handleMessage) ---.
