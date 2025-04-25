// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/fsm"                // Added import for FSM.
	"github.com/dkoosis/cowgnition/internal/logging"            // Keep logging import.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Added import for shared types.

	// mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // No longer directly used here.
	"github.com/dkoosis/cowgnition/internal/mcp/router" // Added import for Router.
	"github.com/dkoosis/cowgnition/internal/mcp/state"  // Added import for FSM Events.
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/services"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// --- REMOVED unused contextKey type definition ---

// ServerOptions contains configurable options for the MCP server.
type ServerOptions struct {
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	Debug           bool
}

// Server represents an MCP (Model Context Protocol) server instance.
type Server struct {
	config    *config.Config
	options   ServerOptions
	transport transport.Transport
	logger    logging.Logger
	startTime time.Time
	validator schema.ValidatorInterface // Use interface from schema package.

	// --- NEW: FSM and Router Dependencies ---
	fsm    fsm.FSM       // State machine for connection lifecycle.
	router router.Router // Router for dispatching method calls.

	// Service Registry (remains).
	services    map[string]services.Service // Map service name (e.g., "rtm") to its implementation.
	serviceLock sync.RWMutex                // Mutex to protect the services map.
}

// NewServer creates a new MCP server instance.
func NewServer(
	cfg *config.Config,
	opts ServerOptions,
	validator schema.ValidatorInterface,
	fsm fsm.FSM, // Inject FSM.
	router router.Router, // Inject Router.
	startTime time.Time,
	logger logging.Logger,
) (*Server, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		return nil, errors.New("schema validator is required but was not provided to NewServer")
	}
	if fsm == nil {
		return nil, errors.New("FSM instance is required but was not provided to NewServer")
	}
	if router == nil {
		return nil, errors.New("Router instance is required but was not provided to NewServer")
	}

	server := &Server{
		config:    cfg,
		options:   opts,
		logger:    logger.WithField("component", "mcp_server"),
		validator: validator,
		fsm:       fsm,    // Store injected FSM.
		router:    router, // Store injected Router.
		startTime: startTime,
		services:  make(map[string]services.Service), // Initialize the service registry.
	}

	server.logger.Info("MCP Server instance created (FSM & Router injected).")
	return server, nil
}

// --- ADDED: Exported Getters ---

// GetRouter returns the server's internal router instance.
func (s *Server) GetRouter() router.Router {
	return s.router
}

// GetLogger returns the server's configured logger instance.
func (s *Server) GetLogger() logging.Logger {
	return s.logger
}

// GetConfig returns the server's configuration.
func (s *Server) GetConfig() *config.Config {
	return s.config
}

// --- END ADDED: Exported Getters ---

// RegisterService adds a service implementation to the server's registry.
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
		return errors.Newf("service with name '%s' already registered.", name)
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

// ServeSTDIO configures and starts the server using stdio transport.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin, s.logger) // Stdin used as closer.

	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = true // Keep true.

	if s.options.Debug {
		validationOpts.StrictOutgoing = true // Be strict in debug.
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: outgoing validation is STRICT.")
	} else {
		validationOpts.StrictOutgoing = false // Allow known warnings in normal mode.
		s.logger.Info("Non-debug mode: outgoing validation is NON-STRICT (logs warnings).")
	}

	validationOpts.SkipTypes = map[string]bool{
		"notifications/initialized": true,
		"notifications/cancelled":   true,
		"notifications/progress":    true,
		"exit":                      true,
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	chain := middleware.NewChain(s.handleMessage) // Target the new core handler func.
	chain.Use(validationMiddleware)

	serveHandler := chain.Handler()
	return s.serverProcessing(ctx, serveHandler)
}

// ServeHTTP starts the server with an HTTP transport (Placeholder).
func (s *Server) ServeHTTP(_ context.Context, _ string) error {
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(_ context.Context) error {
	s.logger.Info("Shutting down MCP server.")

	s.serviceLock.RLock()
	servicesToShutdown := make([]services.Service, 0, len(s.services))
	for name, service := range s.services {
		s.logger.Debug("Preparing to shutdown service.", "serviceName", name)
		servicesToShutdown = append(servicesToShutdown, service)
	}
	s.serviceLock.RUnlock()

	for _, service := range servicesToShutdown {
		name := service.GetName()
		s.logger.Debug("Shutting down service.", "serviceName", name)
		if err := service.Shutdown(); err != nil {
			s.logger.Error("Error shutting down service.", "serviceName", name, "error", fmt.Sprintf("%+v", err))
		} else {
			s.logger.Debug("Service shut down successfully.", "serviceName", name)
		}
	}

	if s.fsm != nil {
		s.logger.Debug("Resetting FSM.")
		if err := s.fsm.Reset(); err != nil {
			s.logger.Error("Error resetting FSM during shutdown.", "error", err)
		}
	}

	if s.transport != nil {
		s.logger.Debug("Closing transport...")
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

// handleMessage is the core message processing function called after middleware.
func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		s.logger.Error("Internal error: Failed to unmarshal validated message in handleMessage.", "error", err)
		reqID := extractRequestID(s.logger, msgBytes)
		idToUseInResponse := reqID
		if idToUseInResponse == nil || string(idToUseInResponse) == "null" {
			idToUseInResponse = json.RawMessage("0")
		}
		return s.createErrorResponse(msgBytes, errors.Wrap(err, "internal error: failed to parse validated message"), idToUseInResponse)
	}

	event := state.EventForMethod(request.Method)
	if event == "" {
		if s.fsm.CurrentState() == state.StateInitialized {
			if request.ID == nil || string(request.ID) == "null" {
				event = state.EventMCPNotification
			} else {
				event = state.EventMCPRequest
			}
		}
		s.logger.Debug("Mapping method to generic FSM event.", "method", request.Method, "event", event)
	}

	fsmErr := s.fsm.Transition(ctx, event, msgBytes)
	if fsmErr != nil {
		s.logger.Warn("Invalid state transition.", "method", request.Method, "event", event, "state", s.fsm.CurrentState(), "error", fsmErr)
		return nil, errors.Wrapf(fsmErr, "invalid sequence: method '%s' (event '%s') not allowed in state '%s'", request.Method, event, s.fsm.CurrentState())
	}

	isNotification := (request.ID == nil || string(request.ID) == "null")
	s.logger.Debug("Routing to handler via Router.", "method", request.Method, "id", string(request.ID), "isNotification", isNotification)

	resultBytes, handlerErr := s.router.Route(ctx, request.Method, request.Params, isNotification)

	if handlerErr != nil {
		s.logger.Warn("Error returned from routed handler.", "method", request.Method, "error", fmt.Sprintf("%+v", handlerErr))
		return nil, handlerErr
	}

	if isNotification {
		s.logger.Debug("Processed notification via router, no response needed.", "method", request.Method)
		if resultBytes != nil {
			s.logger.Warn("Router handler for notification returned non-nil response bytes, discarding.", "method", request.Method)
		}
		return nil, nil
	}

	if resultBytes == nil {
		s.logger.Debug("Router handler returned nil result bytes for request, using JSON null.", "method", request.Method, "id", string(request.ID))
		resultBytes = json.RawMessage("null")
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
		s.logger.Error("Internal error: Failed to marshal successful response.", "method", request.Method, "id", string(request.ID), "error", marshalErr)
		return nil, errors.Wrap(marshalErr, "internal error: failed to marshal success response")
	}

	s.logger.Debug("Successfully processed request via router, returning response.", "method", request.Method, "id", string(request.ID))
	return respBytes, nil
}

// --- ADDED: AggregateServerCapabilities and LogClientInfo Methods ---

// AggregateServerCapabilities collects capabilities from all registered services.
func (s *Server) AggregateServerCapabilities() mcptypes.ServerCapabilities {
	s.serviceLock.RLock()
	defer s.serviceLock.RUnlock()

	// Start with base capabilities (e.g., logging is always supported by the server itself)
	aggCaps := mcptypes.ServerCapabilities{
		Logging: &mcptypes.LoggingCapability{}, // Assuming logging is always enabled
	}

	hasTools := false
	hasResources := false
	hasPrompts := false

	for _, service := range s.services {
		// Check if service provides tools
		if len(service.GetTools()) > 0 {
			hasTools = true
		}
		// Check if service provides resources
		if len(service.GetResources()) > 0 {
			hasResources = true
		}
		// Check if service provides prompts (using a probe call for now)
		if _, err := service.GetPrompt(context.TODO(), "__probe__", nil); err == nil {
			hasPrompts = true
		}
	}

	if hasTools {
		aggCaps.Tools = &mcptypes.ToolsCapability{ListChanged: false}
	}
	if hasResources {
		aggCaps.Resources = &mcptypes.ResourcesCapability{ListChanged: false, Subscribe: false}
	}
	if hasPrompts {
		aggCaps.Prompts = &mcptypes.PromptsCapability{ListChanged: false}
	}

	s.logger.Debug("Aggregated server capabilities.", "capabilities", aggCaps)
	return aggCaps
}

// LogClientInfo logs the details received from the client during initialization.
func (s *Server) LogClientInfo(clientInfo *mcptypes.Implementation, capabilities *mcptypes.ClientCapabilities) {
	if clientInfo != nil {
		s.logger.Info("Received client information.", "clientName", clientInfo.Name, "clientVersion", clientInfo.Version)
	} else {
		s.logger.Warn("Received initialize request without clientInfo.")
	}
	if capabilities != nil {
		s.logger.Info("Received client capabilities.",
			"supportsRoots", capabilities.Roots != nil,
			"supportsSampling", capabilities.Sampling != nil)
	} else {
		s.logger.Warn("Received initialize request without client capabilities.")
	}
}

// --- END ADDED Methods ---
