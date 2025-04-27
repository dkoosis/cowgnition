// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/mcp_server.go
package mcp

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

	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Keep for error mapping if needed later.
	"github.com/dkoosis/cowgnition/internal/mcp/router"               // Added import for Router.
	"github.com/dkoosis/cowgnition/internal/mcp/state"                // Added import for FSM Events.
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/services"
	"github.com/dkoosis/cowgnition/internal/transport"
)

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

	// --- MODIFIED: FSM and Router Dependencies ---.
	fsm    fsm.FSM       // State machine for connection lifecycle.
	router router.Router // Router for dispatching method calls.
	// --- connectionState *ConnectionState // REMOVED ---.

	// Service Registry (remains).
	services    map[string]services.Service // Map service name (e.g., "rtm") to its implementation.
	serviceLock sync.RWMutex                // Mutex to protect the services map.
}

// NewServer creates a new MCP server instance.
// MODIFIED: Accepts FSM and Router, removes ConnectionState init.
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
	// --- ADDED: Null checks for injected FSM and Router ---.
	if fsm == nil {
		return nil, errors.New("FSM instance is required but was not provided to NewServer")
	}
	if router == nil {
		return nil, errors.New("Router instance is required but was not provided to NewServer")
	}
	// --- END ADDED ---.

	server := &Server{
		config:    cfg,
		options:   opts,
		logger:    logger.WithField("component", "mcp_server"),
		validator: validator,
		fsm:       fsm,    // Store injected FSM.
		router:    router, // Store injected Router.
		startTime: startTime,
		services:  make(map[string]services.Service), // Initialize the service registry.
		// --- connectionState: NewConnectionState(), // REMOVED ---.
	}

	// Note: Core route registration should happen *after* NewServer, typically in server_runner.go.
	// using server.GetRouter().AddRoute(...).
	server.logger.Info("MCP Server instance created (FSM & Router injected).")
	return server, nil
}

// --- ADDED: Exported Getters ---.

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

// --- END ADDED: Exported Getters ---.

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

	// --- ADDED: Register service routes with the main router ---.
	s.registerServiceRoutes(service)
	// --- END ADDED ---.

	return nil
}

// registerServiceRoutes adds routes for a specific service's tools and resources to the main router.
func (s *Server) registerServiceRoutes(service services.Service) {
	serviceName := service.GetName()
	routerLogger := s.logger.WithField("service", serviceName)

	// Register Tools.
	for _, tool := range service.GetTools() {
		// Capture tool variable for the closure.
		currentTool := tool
		toolName := currentTool.Name // e.g., "rtm_getTasks".
		if toolName == "" {
			routerLogger.Warn("Skipping registration of tool with empty name.")
			continue
		}

		err := s.router.AddRoute(router.Route{
			Method: toolName,
			Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
				// Call the service's CallTool method.
				routerLogger.Debug("Routing tool call to service.", "tool", toolName)
				toolResult, callErr := service.CallTool(ctx, toolName, params)
				if callErr != nil {
					// This error is for failures *handling* the call, not tool execution errors.
					routerLogger.Error("Error handling tool call within service.", "tool", toolName, "error", callErr)
					// Map to internal error? Or let the main error handler handle it?.
					// Let main handler map it for now.
					return nil, callErr
				}
				// Marshal the CallToolResult. Tool errors are inside toolResult.IsError.
				resultBytes, marshalErr := json.Marshal(toolResult)
				if marshalErr != nil {
					routerLogger.Error("Failed to marshal tool result.", "tool", toolName, "error", marshalErr)
					return nil, errors.Wrap(marshalErr, "failed to marshal tool result")
				}
				return resultBytes, nil
			},
			// Tools are typically requests, not notifications.
		})
		if err != nil {
			routerLogger.Error("Failed to register tool route.", "tool", toolName, "error", err)
		} else {
			routerLogger.Debug("Registered tool route.", "tool", toolName)
		}
	}

	// Resources are handled by a core "resources/read" handler, no specific routes needed here.
	routerLogger.Debug("Resource routes handled by core 'resources/read' handler.")

	// Prompts are handled by a core "prompts/get" handler, no specific routes needed here.
	routerLogger.Debug("Prompt routes handled by core 'prompts/get' handler.")
}

// GetService retrieves a registered service by name. (Used internally).
func (s *Server) GetService(name string) (services.Service, bool) {
	s.serviceLock.RLock()
	defer s.serviceLock.RUnlock()
	service, ok := s.services[name]
	return service, ok
}

// GetAllServices returns a slice of all registered services. (Used internally).
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

	// Set up validation middleware
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = true

	if s.options.Debug {
		validationOpts.StrictOutgoing = true
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: outgoing validation is STRICT.")
	} else {
		validationOpts.StrictOutgoing = false
		s.logger.Info("Non-debug mode: outgoing validation is NON-STRICT (logs warnings).")
	}

	validationOpts.SkipTypes = map[string]bool{
		"exit": true, // Skip schema validation for exit notification.
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	// Set up middleware chain
	chain := middleware.NewChain(s.handleMessageWithFSM)
	chain.Use(validationMiddleware)

	// Add logging middleware if desired
	if s.options.Debug {
		loggingMiddleware := createLoggingMiddleware(s.logger)
		chain.Use(loggingMiddleware)
	}

	serveHandler := chain.Handler()

	// Run processing loop
	return s.serverProcessing(ctx, serveHandler)
}

// createLoggingMiddleware creates a middleware that logs incoming and outgoing messages.
func createLoggingMiddleware(logger logging.Logger) mcptypes.MiddlewareFunc {
	middlewareLogger := logger.WithField("component", "logging_middleware")

	return func(next mcptypes.MessageHandler) mcptypes.MessageHandler {
		return func(ctx context.Context, message []byte) ([]byte, error) {
			// Log incoming message (truncate if too large)
			maxLogSize := 1024
			msgPreview := string(message)
			if len(msgPreview) > maxLogSize {
				msgPreview = msgPreview[:maxLogSize] + "... [truncated]"
			}
			middlewareLogger.Debug("Incoming message", "message", msgPreview)

			// Call next handler
			start := time.Now()
			result, err := next(ctx, message)
			duration := time.Since(start)

			// Log result or error
			if err != nil {
				middlewareLogger.Debug("Handler error",
					"error", err,
					"duration_ms", duration.Milliseconds())
			} else if result != nil {
				resultPreview := string(result)
				if len(resultPreview) > maxLogSize {
					resultPreview = resultPreview[:maxLogSize] + "... [truncated]"
				}
				middlewareLogger.Debug("Outgoing message",
					"message", resultPreview,
					"duration_ms", duration.Milliseconds())
			} else {
				middlewareLogger.Debug("No response from handler (notification)",
					"duration_ms", duration.Milliseconds())
			}

			return result, err
		}
	}
}

// ServeHTTP starts the server with an HTTP transport (Placeholder).
func (s *Server) ServeHTTP(_ context.Context, _ string) error {
	s.logger.Error("HTTP transport not implemented.")
	return errors.New("HTTP transport not implemented")
}

// Shutdown initiates a graceful shutdown of the server.
// MODIFIED: Resets FSM instead of old state manager.
func (s *Server) Shutdown(_ context.Context) error {
	s.logger.Info("Shutting down MCP server.")

	// --- Shutdown Services (Unchanged) ---.
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
	// --- End Shutdown Services ---.

	// --- Reset FSM ---.
	if s.fsm != nil {
		s.logger.Debug("Resetting FSM.")
		if err := s.fsm.Reset(); err != nil {
			s.logger.Error("Error resetting FSM during shutdown.", "error", err)
			// Continue shutdown even if FSM reset fails.
		}
	}
	// --- End Reset FSM ---.

	// --- Close Transport (Unchanged) ---.
	if s.transport != nil {
		s.logger.Debug("Closing transport...")
		if err := s.transport.Close(); err != nil {
			s.logger.Error("Failed to close transport during shutdown.", "error", fmt.Sprintf("%+v", err))
			// Return this error? Or just log? Let's log for now.
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}
	// --- End Close Transport ---.

	s.logger.Info("Server shutdown sequence completed.")
	return nil
}

// handleMessageWithFSM is the core message processing function called after middleware.
// It uses the FSM for state validation and the Router for dispatching.
// REFACTORED: Incorporates FSM transition and Router dispatch.
func (s *Server) handleMessageWithFSM(ctx context.Context, msgBytes []byte) ([]byte, error) {
	// 1. Parse basic structure to get method and ID.
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	// Validation middleware should ensure msgBytes is valid JSON.
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		s.logger.Error("Internal Error: Failed to unmarshal validated message in core handler.",
			"error", err, "preview", string(msgBytes[:minInt(len(msgBytes), 100)])) // <<< UPDATE CALL SITE.
		// Can't reliably get ID if unmarshal failed. Create generic internal error response.
		errRespBytes, _ := s.createErrorResponse(nil, errors.Wrap(err, "internal parse error"), json.RawMessage("null"))
		return errRespBytes, nil // Return error response bytes.
	}

	isNotification := (request.ID == nil || string(request.ID) == "null")
	logFields := []interface{}{"method", request.Method, "id", string(request.ID), "isNotification", isNotification}
	s.logger.Debug("Core handler received message.", logFields...)

	// 2. Determine FSM Event corresponding to the MCP method.
	event := state.EventForMethod(request.Method)
	if event == "" { // Not a specific lifecycle method.
		// Check current state BEFORE deciding generic event type.
		currentState := s.fsm.CurrentState()
		logFields = append(logFields, "state", currentState) // Add current state to logs.
		if currentState == state.StateInitialized {
			// Treat as generic request/notification IF in initialized state.
			if isNotification {
				event = state.EventMCPNotification
			} else {
				event = state.EventMCPRequest
			}
			logFields = append(logFields, "mappedEvent", event)
			s.logger.Debug("Mapping to generic FSM event.", logFields...)
		} else {
			// If not initialized, any non-lifecycle method is a sequence error.
			// Triggering with a generic event will let the FSM return the appropriate error.
			event = state.EventMCPRequest // Triggering with this will fail if not in Initialized state.
			logFields = append(logFields, "mappedEvent", event, "note", "Expecting FSM sequence error")
			s.logger.Warn("Received non-lifecycle method in non-initialized state.", logFields...)
		}
	} else {
		logFields = append(logFields, "mappedEvent", event) // Log the specific lifecycle event found.
	}

	// 3. Attempt FSM Transition - This is now the primary sequence validation.
	transitionErr := s.fsm.Transition(ctx, event, msgBytes) // Pass msgBytes as data if needed by actions/guards.
	if transitionErr != nil {
		// Log the FSM error and map it to a JSON-RPC error response.
		s.logger.Warn("FSM transition failed.", append(logFields, "error", transitionErr)...)
		// createErrorResponse should handle mapping FSM errors (NoTransitionError, CanceledError etc.).
		// to appropriate JSON-RPC codes (e.g., InvalidRequest -32600 for sequence errors).
		errRespBytes, creationErr := s.createErrorResponse(request.ID, transitionErr, request.ID)
		if creationErr != nil {
			return nil, creationErr // Propagate critical error creating the response.
		}
		return errRespBytes, nil // Return the formatted JSON-RPC error bytes.
	}
	// Log successful transition and new state.
	newState := s.fsm.CurrentState()
	logFields = append(logFields, "newState", newState)
	s.logger.Debug("FSM transition successful.", logFields...)

	// --- Handle special states after successful transition ---.
	// If the state is now terminal (Shutdown), stop processing.
	if newState == state.StateShutdown {
		// This typically happens on 'exit' notification.
		s.logger.Info("FSM reached terminal state (Shutdown), stopping processing for this message.")
		// The serverProcessing loop should detect this state and terminate the connection.
		return nil, nil // No response for the message that triggered shutdown state.
	}
	// If state is ShuttingDown (due to 'shutdown' request), process the request via router to send null response.
	// --- End handle special states ---.

	// 4. Route to Handler via Router (if FSM transition succeeded and state is not terminal).
	s.logger.Debug("Dispatching to router.", logFields...)
	resultBytes, handlerErr := s.router.Route(ctx, request.Method, request.Params, isNotification)

	// 5. Handle Router/Handler Errors.
	if handlerErr != nil {
		s.logger.Warn("Error returned from router/handler.", append(logFields, "error", fmt.Sprintf("%+v", handlerErr))...)
		// Map handler error to JSON-RPC error response.
		errRespBytes, creationErr := s.createErrorResponse(request.ID, handlerErr, request.ID) // Use original request ID.
		if creationErr != nil {
			return nil, creationErr // Failed to create error response.
		}
		return errRespBytes, nil // Return formatted error bytes.
	}

	// 6. Handle Successful Notifications (No Response).
	if isNotification {
		s.logger.Debug("Processed notification via router, no response needed.", logFields...)
		if resultBytes != nil {
			s.logger.Warn("Router handler for notification returned non-nil response bytes, discarding.", logFields...)
		}
		return nil, nil
	}

	// 7. Format Successful Response for Requests.
	if resultBytes == nil {
		// If handler returns nil bytes for a request, use JSON null as the result.
		s.logger.Debug("Handler returned nil result bytes for request, using JSON null.", logFields...)
		resultBytes = json.RawMessage("null")
	}

	responseObj := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      request.ID, // Use the original request ID.
		Result:  resultBytes,
	}

	respBytes, marshalErr := json.Marshal(responseObj)
	if marshalErr != nil {
		s.logger.Error("Internal error: Failed to marshal successful response.", append(logFields, "error", marshalErr)...)
		// If marshalling fails, create and return an internal error response.
		errRespBytes, _ := s.createErrorResponse(request.ID, errors.Wrap(marshalErr, "internal error: failed to marshal success response"), request.ID)
		return errRespBytes, nil
	}

	s.logger.Debug("Successfully processed request, returning response.", logFields...)
	return respBytes, nil
}

// --- AggregateServerCapabilities and LogClientInfo Methods (Unchanged) ---.

// AggregateServerCapabilities collects capabilities from all registered services.
func (s *Server) AggregateServerCapabilities() mcptypes.ServerCapabilities {
	s.serviceLock.RLock()
	defer s.serviceLock.RUnlock()

	// Start with base capabilities (e.g., logging is always supported by the server itself).
	aggCaps := mcptypes.ServerCapabilities{
		Logging: &mcptypes.LoggingCapability{}, // Assuming logging is always enabled.
	}

	hasTools := false
	hasResources := false
	hasPrompts := false

	for _, service := range s.services {
		// Check if service provides tools.
		if len(service.GetTools()) > 0 {
			hasTools = true
		}
		// Check if service provides resources.
		if len(service.GetResources()) > 0 {
			hasResources = true
		}
		// Check if service provides prompts (using a probe call for now).
		// Use background context for probe, avoid cancelling main request.
		probeCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err := service.GetPrompt(probeCtx, "__probe__", nil)
		cancel() // Release context resources.
		// Check if the error indicates "not implemented" vs other errors.
		var methodNotFoundErr *mcperrors.MethodNotFoundError
		if err == nil || !errors.As(err, &methodNotFoundErr) {
			// Assume prompts are supported if GetPrompt returns nil OR an error *other* than MethodNotFound.
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

// --- End ADDED Methods ---.

// --- Helper function (minInt) ---.
// Renamed to avoid conflict with Go 1.21 built-in 'min'.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Ensure mcp_server_processing.go and mcp_server_error_handling.go are updated ---.
// The serverProcessing loop in mcp_server_processing.go should now call handleMessageWithFSM.
// The createErrorResponse function in mcp_server_error_handling.go needs to handle.
// potential FSM errors (like sequence errors) passed to it.
