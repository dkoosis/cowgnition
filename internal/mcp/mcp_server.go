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
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
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
	handler         *Handler                 // Handles the actual method logic.
	methods         map[string]MethodHandler // Registered method handlers.
	transport       transport.Transport
	logger          logging.Logger
	startTime       time.Time
	validator       schema.ValidatorInterface // Use interface from schema package.
	connectionState *ConnectionState
}

// NewServer creates a new MCP server instance.
func NewServer(cfg *config.Config, opts ServerOptions, validator schema.ValidatorInterface,
	startTime time.Time, logger logging.Logger) (*Server, error) {
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
	// Add other methods as needed.

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
	validationOpts.StrictMode = true       // Incoming messages must be valid.
	validationOpts.ValidateOutgoing = true // Enable validation for outgoing messages.

	// --- Interim Fix Consideration ---
	// We keep StrictOutgoing=false in normal operation because the server's outgoing
	// 'initialize' response currently triggers a known validation warning (due to
	// schema/struct mismatch during validation step, even if final JSON is ok).
	// Setting this to false logs the warning but allows the connection to proceed.
	// Ideally, the underlying validation logic or schema would be fixed.
	// Ref: https://github.com/modelcontextprotocol/modelcontextprotocol/issues/394
	// ---------------------------------
	if s.options.Debug {
		// In debug mode, we might want stricter outgoing validation to surface issues,
		// even if it breaks the connection due to the known 'initialize' warning.
		validationOpts.StrictOutgoing = true // Make outgoing errors fatal in debug builds.
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: outgoing validation is STRICT.")
	} else {
		validationOpts.StrictOutgoing = false // Make outgoing errors non-fatal in normal builds.
		s.logger.Info("Non-debug mode: outgoing validation is NON-STRICT (logs warnings).")
	}
	// --- End Interim Fix Consideration ---

	// Pass the validator (which implements schema.ValidatorInterface).
	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	// Build middleware chain using mcptypes interfaces.
	chain := middleware.NewChain(s.handleMessage) // s.handleMessage is the final destination.
	chain.Use(validationMiddleware)               // Apply validation middleware first.

	// Handler() returns the composed mcptypes.MessageHandler.
	serveHandler := chain.Handler()

	// Pass the composed handler to the serve loop.
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
			// Don't return error here, allow shutdown to continue if possible
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}
	// TODO: Add shutdown hooks for services (like RTM) if needed.
	s.logger.Info("Server shutdown sequence completed.")
	return nil
}

// serve handles the main server loop, reading messages and dispatching them to the handler.
// Now accepts mcptypes.MessageHandler.
func (s *Server) serve(ctx context.Context, handlerFunc mcptypes.MessageHandler) error {
	// This function's implementation is in mcp_server_processing.go.
	return s.serverProcessing(ctx, handlerFunc)
}

// serverProcessing is the actual implementation, located in mcp_server_processing.go.
// func (s *Server) serverProcessing(ctx context.Context, handlerFunc mcptypes.MessageHandler) error { ... }

// handleMessage is the final handler in the middleware chain, located in mcp_server_processing.go.
// func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) { ... }
