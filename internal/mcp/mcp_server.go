// file: internal/mcp/mcp_server.go
// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

import (
	"context"
	"fmt"
	"os"
	"sync" // Added for service registry mutex.
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging" // Use mcp_types for interfaces/types.
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/services" // Import the services interface package.
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
// DEPRECATED: Handlers are now part of services.Service interface implementations.
// type MethodHandler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error).

// Server represents an MCP (Model Context Protocol) server instance.
type Server struct {
	config    *config.Config
	options   ServerOptions
	transport transport.Transport
	logger    logging.Logger
	startTime time.Time
	validator schema.ValidatorInterface // Use interface from schema package.

	// NEW: Service Registry.
	services    map[string]services.Service // Map service name (e.g., "rtm") to its implementation.
	serviceLock sync.RWMutex                // Mutex to protect the services map.

	connectionState *ConnectionState
	// REMOVED: handler *Handler - Logic will be delegated to services.
	// REMOVED: methods map[string]MethodHandler - Routing logic moved.
}

// NewServer creates a new MCP server instance.
// It now initializes an empty service registry.
func NewServer(cfg *config.Config, opts ServerOptions, validator schema.ValidatorInterface,
	startTime time.Time, logger logging.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		return nil, errors.New("schema validator is required but was not provided to NewServer")
	}

	connState := NewConnectionState()

	server := &Server{
		config:          cfg,
		options:         opts,
		logger:          logger.WithField("component", "mcp_server"),
		validator:       validator,
		startTime:       startTime,
		connectionState: connState,
		services:        make(map[string]services.Service), // Initialize the service registry.
	}

	// REMOVED: Old handler creation and method registration.
	// server.handler = NewHandler(cfg, validator, startTime, connState, logger).
	// server.registerMethods().

	server.logger.Info("MCP Server instance created (services need registration).")
	return server, nil
}

// RegisterService adds a service implementation to the server's registry.
// This should be called during server setup after creating service instances.
func (s *Server) RegisterService(service services.Service) error {
	if service == nil {
		return errors.New("cannot register a nil service")
	}
	name := service.GetName()
	if name == "" {
		return errors.New("cannot register service with an empty name")
	}

	s.serviceLock.Lock()
	defer s.serviceLock.Unlock()

	if _, exists := s.services[name]; exists {
		return errors.Newf("service with name '%s' already registered", name)
	}
	s.services[name] = service
	s.logger.Info("Registered service.", "serviceName", name)
	return nil
}

// GetService retrieves a registered service by name. (Used internally for routing).
func (s *Server) GetService(name string) (services.Service, bool) {
	s.serviceLock.RLock()
	defer s.serviceLock.RUnlock()
	service, ok := s.services[name]
	return service, ok
}

// GetAllServices returns a slice of all registered services. (Used internally for list methods).
func (s *Server) GetAllServices() []services.Service {
	s.serviceLock.RLock()
	defer s.serviceLock.RUnlock()
	allServices := make([]services.Service, 0, len(s.services))
	for _, service := range s.services {
		allServices = append(allServices, service)
	}
	return allServices
}

// REMOVED: registerMethods - Routing logic moved to handleMessage / processing loop.
// REMOVED: getMethods - No longer needed.

// ServeSTDIO configures and starts the server using stdio transport.
// It now builds a middleware chain ending in the refactored s.handleMessage.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin, s.logger) // Stdin used as closer.

	// Setup validation middleware.
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = true // Keep true, but handle known issues.

	// Interim Fix Note: StrictOutgoing=false needed for now due to initialize response warning.
	if s.options.Debug {
		validationOpts.StrictOutgoing = true // Be strict in debug.
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: outgoing validation is STRICT.")
	} else {
		validationOpts.StrictOutgoing = false // Allow known warnings in normal mode.
		s.logger.Info("Non-debug mode: outgoing validation is NON-STRICT (logs warnings).")
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	// Build middleware chain. The final handler is s.routeMessage.
	// s.routeMessage will contain the logic to dispatch to the correct service.
	chain := middleware.NewChain(s.routeMessage) // NEW: Target the router func.
	chain.Use(validationMiddleware)

	serveHandler := chain.Handler()

	// Start the processing loop.
	return s.serverProcessing(ctx, serveHandler)
}

// ServeHTTP starts the server with an HTTP transport (Placeholder).
func (s *Server) ServeHTTP(_ context.Context, _ string) error {
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
// It now also calls Shutdown on all registered services.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down MCP server.")

	// Shutdown registered services.
	s.serviceLock.RLock()
	servicesToShutdown := make([]services.Service, 0, len(s.services))
	for name, service := range s.services {
		s.logger.Debug("Preparing to shutdown service.", "serviceName", name)
		servicesToShutdown = append(servicesToShutdown, service)
	}
	s.serviceLock.RUnlock() // Release lock before calling Shutdown on services.

	for _, service := range servicesToShutdown {
		name := service.GetName()
		s.logger.Debug("Shutting down service.", "serviceName", name)
		if err := service.Shutdown(); err != nil {
			// Log error but continue shutdown process.
			s.logger.Error("Error shutting down service.", "serviceName", name, "error", fmt.Sprintf("%+v", err))
		} else {
			s.logger.Debug("Service shut down successfully.", "serviceName", name)
		}
	}

	// Close transport.
	if s.transport != nil {
		// Use shutdown context with timeout for closing transport.
		closeCtx, cancel := context.WithTimeout(ctx, s.options.ShutdownTimeout)
		defer cancel()
		if err := s.transport.Close(); err != nil {
			// Check if the error is due to the context deadline being exceeded.
			if errors.Is(err, context.DeadlineExceeded) {
				s.logger.Warn("Transport close timed out during shutdown.", "timeout", s.options.ShutdownTimeout)
			} else {
				s.logger.Error("Failed to close transport during shutdown.", "error", fmt.Sprintf("%+v", err))
			}
			// Don't return error here, allow shutdown to continue if possible.
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}

	s.logger.Info("Server shutdown sequence completed.")
	return nil
}

// --- Core Processing Logic (Moved to mcp_server_processing.go) ---
// func (s *Server) serve(ctx context.Context, handlerFunc mcptypes.MessageHandler) error
// func (s *Server) serverProcessing(ctx context.Context, handlerFunc mcptypes.MessageHandler) error
// func (s *Server) processNextMessage(ctx context.Context, handlerFunc mcptypes.MessageHandler) error

// --- NEW Routing Logic (Replaces old handleMessage) ---
// This function will be the final handler in the middleware chain.
// It inspects the request method and delegates to the appropriate service.
// (Implementation will be provided in the next step - mcp_server_processing.go).
func (s *Server) routeMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	// TODO: Implementation in mcp_server_processing.go.
	// 1. Parse msgBytes to get method name and params.
	// 2. Handle core methods like "ping", "initialize" directly.
	// 3. For service methods ("tools/call", "resources/read", etc.):
	//    - Identify target service based on name/URI.
	//    - Look up service in s.services registry.
	//    - Call appropriate service method (service.CallTool, service.ReadResource).
	// 4. For list methods ("tools/list", "resources/list"):
	//    - Iterate through s.services.
	//    - Call GetTools/GetResources on each.
	//    - Aggregate and marshal results.
	// 5. Construct JSON-RPC success/error response.
	s.logger.Error("routeMessage logic not yet implemented in mcp_server.go.", "messagePreview", string(msgBytes))
	// Placeholder: Return internal error until implemented.
	return s.createErrorResponse(msgBytes, errors.New("message routing not implemented"))
}

// --- Error Handling (Moved to mcp_server_error_handling.go) ---
// func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error)
// func extractRequestID(logger logging.Logger, msgBytes []byte) json.RawMessage
// func (s *Server) mapErrorToJSONRPCComponents(logger logging.Logger, err error) (code int, message string, data interface{})
// func (s *Server) logErrorDetails(code int, message string, requestID json.RawMessage, data interface{}, err error)
