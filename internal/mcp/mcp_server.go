// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/middleware"
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
	config          *config.Config
	options         ServerOptions
	handler         *Handler                 // Handles the actual method logic
	methods         map[string]MethodHandler // Registered method handlers
	transport       transport.Transport
	logger          logging.Logger
	startTime       time.Time
	validator       middleware.SchemaValidatorInterface
	connectionState *ConnectionState
}

// NewServer creates a new MCP server instance.
func NewServer(cfg *config.Config, opts ServerOptions, validator middleware.SchemaValidatorInterface, startTime time.Time, logger logging.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		return nil, errors.New("schema validator is required but was not provided to NewServer")
	}

	connState := NewConnectionState()

	// Create the core method handler instance.
	// Ensure the validator type is compatible if Handler requires concrete type.
	var concreteValidator *schema.SchemaValidator
	var ok bool
	if concreteValidator, ok = validator.(*schema.SchemaValidator); !ok {
		return nil, errors.New("NewServer requires a concrete *schema.SchemaValidator instance for the Handler")
	}
	handler := NewHandler(cfg, concreteValidator, startTime, connState, logger) // Assuming NewHandler is in handler.go

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

	// Register methods provided by the handler.
	server.registerMethods()

	return server, nil
}

// registerMethods populates the server's method map.
func (s *Server) registerMethods() {
	// Assuming handlers are defined in handlers_*.go files within the Handler struct.
	// Example registration:
	s.methods["initialize"] = s.handler.handleInitialize
	s.methods["ping"] = s.handler.handlePing
	s.methods["notifications/initialized"] = s.handler.handleNotificationsInitialized
	s.methods["tools/list"] = s.handler.handleToolsList
	s.methods["tools/call"] = s.handler.handleToolCall
	s.methods["resources/list"] = s.handler.handleResourcesList
	s.methods["resources/read"] = s.handler.handleResourcesRead
	s.methods["prompts/list"] = s.handler.handlePromptsList
	s.methods["prompts/get"] = s.handler.handlePromptsGet
	// ... register other methods from s.handler ...

	s.logger.Info("Registered MCP methods.",
		"count", len(s.methods),
		"methods", getMethods(s.methods)) // getMethods remains here or move to helpers
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
	// Setup transport
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin, s.logger) // Use os.Stdin as closer for stdio

	// Setup validation middleware
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true       // Enforce strict validation
	validationOpts.ValidateOutgoing = true // Validate server responses

	if s.options.Debug {
		validationOpts.StrictOutgoing = true     // Fail on invalid outgoing in debug
		validationOpts.MeasurePerformance = true // Measure validation time
		s.logger.Info("Debug mode enabled: using strict validation for incoming and outgoing messages.")
	} else {
		validationOpts.StrictOutgoing = false // Log outgoing errors but don't fail
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	// Build middleware chain
	chain := middleware.NewChain(s.handleMessage) // s.handleMessage is the final dispatcher
	chain.Use(validationMiddleware)
	// Add other middleware here if needed, e.g., logging, auth checks

	serveHandler := chain.Handler()

	// Start the processing loop (serve is now in mcp_server_processing.go)
	return s.serve(ctx, serveHandler)
}

// ServeHTTP starts the server with an HTTP transport (Placeholder).
func (s *Server) ServeHTTP(_ context.Context, _ string) error {
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
	// Implementation would involve setting up an HTTP server,
	// creating a transport per connection (e.g., SSE), and running s.serve.
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(_ context.Context) error { // Context might be used for timeout
	s.logger.Info("Shutting down MCP server.")
	// Close the transport if it exists
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			// Log error but don't necessarily fail shutdown
			s.logger.Error("Failed to close transport during shutdown.", "error", fmt.Sprintf("%+v", err))
			// Return the error if closing transport is critical
			// return errors.Wrap(err, "transport close failed during shutdown")
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}

	// REMOVED empty if block for s.handler != nil

	s.logger.Info("Server shutdown sequence completed.")
	return nil // Indicate successful shutdown attempt
}
