// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time" // Keep time import.

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/fsm"                // Added import for FSM.
	"github.com/dkoosis/cowgnition/internal/logging"            // Keep logging import.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Added import for shared types.

	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Keep for error mapping if needed later.
	"github.com/dkoosis/cowgnition/internal/mcp/router"               // Added import for Router.
	"github.com/dkoosis/cowgnition/internal/mcp/state"                // Added import for FSM Events.
	"github.com/dkoosis/cowgnition/internal/middleware"

	// "github.com/dkoosis/cowgnition/internal/schema" // No longer need schema for the interface type here.
	"github.com/dkoosis/cowgnition/internal/services"
	"github.com/dkoosis/cowgnition/internal/transport"
	lfsm "github.com/looplab/fsm" // <<< IMPORT ADDED FOR ERROR CHECKING >>>
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
	// <<< CHANGED TYPE to mcptypes.ValidatorInterface >>>.
	validator mcptypes.ValidatorInterface

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
// <<< CHANGED PARAMETER TYPE to mcptypes.ValidatorInterface >>>.
func NewServer(
	cfg *config.Config,
	opts ServerOptions,
	validator mcptypes.ValidatorInterface,
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
		validator: validator, // Assign the provided interface.
		fsm:       fsm,       // Store injected FSM.
		router:    router,    // Store injected Router.
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
	s.logger.Debug(">>> ServeSTDIO: Entering function...") // <<< ADDED LOG.
	s.logger.Info("Starting server with stdio transport.")
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin, s.logger) // Stdin used as closer.
	s.logger.Debug(">>> ServeSTDIO: NDJSONTransport created.")                          // <<< ADDED LOG.

	// Set up validation middleware.
	// <<< CORRECTED: Use middleware package for DefaultValidationOptions >>>.
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	// <<< ADJUSTED: ValidateOutgoing often false in production, true in debug maybe >>>.
	// Let's keep it true only if debug is enabled for stricter checking during dev.
	validationOpts.ValidateOutgoing = s.options.Debug

	if s.options.Debug {
		validationOpts.StrictOutgoing = true // Make outgoing strict only in debug.
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: outgoing validation is STRICT.")
	} else {
		validationOpts.StrictOutgoing = false
		s.logger.Info("Non-debug mode: outgoing validation is NON-STRICT (logs warnings).")
	}

	validationOpts.SkipTypes = map[string]bool{
		"exit": true, // Skip schema validation for exit notification.
	}

	s.logger.Debug(">>> ServeSTDIO: Creating validation middleware...") // <<< ADDED LOG.
	// <<< This call should now work because s.validator is mcptypes.ValidatorInterface >>>.
	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator, // s.validator now holds the correct interface type.
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)
	s.logger.Debug(">>> ServeSTDIO: Validation middleware created.") // <<< ADDED LOG.

	// Set up middleware chain.
	s.logger.Debug(">>> ServeSTDIO: Creating middleware chain...") // <<< ADDED LOG.
	chain := middleware.NewChain(s.handleMessageWithFSM)
	chain.Use(validationMiddleware)

	// Add logging middleware if desired.
	if s.options.Debug {
		loggingMiddleware := createLoggingMiddleware(s.logger)
		chain.Use(loggingMiddleware)
	}

	serveHandler := chain.Handler()
	s.logger.Debug(">>> ServeSTDIO: Middleware chain handler finalized.") // <<< ADDED LOG.

	// Run processing loop.
	s.logger.Debug(">>> ServeSTDIO: Calling serverProcessing...") // <<< ADDED LOG.
	err := s.serverProcessing(ctx, serveHandler)
	s.logger.Debug(">>> ServeSTDIO: serverProcessing returned.", "error", err) // <<< ADDED LOG.
	return err
}

// createLoggingMiddleware creates a middleware that logs incoming and outgoing messages.
func createLoggingMiddleware(logger logging.Logger) mcptypes.MiddlewareFunc {
	middlewareLogger := logger.WithField("component", "logging_middleware")

	return func(next mcptypes.MessageHandler) mcptypes.MessageHandler {
		return func(ctx context.Context, message []byte) ([]byte, error) {
			// Log incoming message (truncate if too large).
			maxLogSize := 1024
			msgPreview := string(message)
			if len(msgPreview) > maxLogSize {
				msgPreview = msgPreview[:maxLogSize] + "... [truncated]"
			}
			middlewareLogger.Debug("Incoming message.", "message", msgPreview)

			// Call next handler.
			start := time.Now()
			result, err := next(ctx, message)
			duration := time.Since(start)

			// Log result or error.
			if err != nil {
				middlewareLogger.Debug("Handler error.",
					"error", err,
					"duration_ms", duration.Milliseconds())
			} else if result != nil {
				resultPreview := string(result)
				if len(resultPreview) > maxLogSize {
					resultPreview = resultPreview[:maxLogSize] + "... [truncated]"
				}
				middlewareLogger.Debug("Outgoing message.",
					"message", resultPreview,
					"duration_ms", duration.Milliseconds())
			} else {
				middlewareLogger.Debug("No response from handler (notification).",
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
// CORRECTED: Properly handles fsm.NoTransitionError.
func (s *Server) handleMessageWithFSM(ctx context.Context, msgBytes []byte) ([]byte, error) {
	s.logger.Debug(">>> DEBUG: ENTERING handleMessageWithFSM.", "messagePreview", string(msgBytes[:minInt(len(msgBytes), 60)]))
	// 1. Parse basic structure to get method and ID.
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		s.logger.Error("Internal Error: Failed to unmarshal validated message in core handler.",
			"error", err, "preview", string(msgBytes[:minInt(len(msgBytes), 100)]))
		errRespBytes, _ := s.createErrorResponse(nil, errors.Wrap(err, "internal parse error"), json.RawMessage("null"))
		return errRespBytes, nil
	}

	isNotification := (request.ID == nil || string(request.ID) == "null")
	logFields := []interface{}{"method", request.Method, "id", string(request.ID), "isNotification", isNotification}
	s.logger.Debug("Core handler received message.", logFields...)

	// 2. Determine FSM Event corresponding to the MCP method.
	event := state.EventForMethod(request.Method)
	if event == "" {
		currentState := s.fsm.CurrentState()
		logFields = append(logFields, "state", currentState)
		if currentState == state.StateInitialized {
			if isNotification {
				event = state.EventMCPNotification
			} else {
				event = state.EventMCPRequest
			}
			logFields = append(logFields, "mappedEvent", event)
			s.logger.Debug("Mapping to generic FSM event.", logFields...)
		} else {
			event = state.EventMCPRequest
			logFields = append(logFields, "mappedEvent", event, "note", "Expecting FSM sequence error")
			s.logger.Warn("Received non-lifecycle method in non-initialized state.", logFields...)
		}
	} else {
		logFields = append(logFields, "mappedEvent", event)
	}

	// 3. Attempt FSM Transition - This is now the primary sequence validation.
	transitionErr := s.fsm.Transition(ctx, event, msgBytes)

	// *** CORRECTED ERROR HANDLING LOGIC ***
	if transitionErr != nil {
		var noTransitionErr lfsm.NoTransitionError
		// Check if the error is specifically NoTransitionError
		if errors.As(transitionErr, &noTransitionErr) {
			// This is expected for self-transitions (e.g., ping in initialized state).
			// Log it as debug but DO NOT treat it as a fatal error for response generation.
			s.logger.Debug("FSM NoTransitionError encountered (expected for self-transitions).", append(logFields, "error", transitionErr)...)
			// IMPORTANT: Do *not* return an error response here. Proceed to routing.
		} else {
			// It's a different, *actual* FSM transition error (InvalidEvent, Canceled, etc.)
			s.logger.Warn("FSM transition failed.", append(logFields, "error", transitionErr)...)
			// Generate and return the appropriate JSON-RPC error response.
			errRespBytes, creationErr := s.createErrorResponse(msgBytes, transitionErr, request.ID) // Pass msgBytes here
			if creationErr != nil {
				return nil, creationErr // Propagate critical error creating the response.
			}
			return errRespBytes, nil // Return the formatted JSON-RPC error bytes.
		}
	} else {
		// Log successful transition and new state if no error occurred.
		newState := s.fsm.CurrentState()
		logFields = append(logFields, "newState", newState)
		s.logger.Debug("FSM transition successful.", logFields...)
	}
	// *** END CORRECTED ERROR HANDLING LOGIC ***

	// --- Handle special states after successful transition (or NoTransitionError) ---.
	newState := s.fsm.CurrentState() // Check state *after* transition attempt
	if newState == state.StateShutdown {
		s.logger.Info("FSM reached terminal state (Shutdown), stopping processing for this message.")
		return nil, nil
	}
	// --- End handle special states ---.

	// 4. Route to Handler via Router (Only if transition didn't fail with a *real* error).
	s.logger.Debug("Dispatching to router.", logFields...)
	resultBytes, handlerErr := s.router.Route(ctx, request.Method, request.Params, isNotification)

	// +++ Keep Debug Logging +++
	if request.Method == "ping" && !isNotification {
		s.logger.Debug(">>> DEBUG: Post-router call for 'ping'.",
			"resultBytesFromRouter", string(resultBytes),
			"handlerError", handlerErr)
	}
	// +++ End Debug Logging +++

	// 5. Handle Router/Handler Errors.
	if handlerErr != nil {
		s.logger.Warn("Error returned from router/handler.", append(logFields, "error", fmt.Sprintf("%+v", handlerErr))...)
		errRespBytes, creationErr := s.createErrorResponse(msgBytes, handlerErr, request.ID)
		if creationErr != nil {
			return nil, creationErr
		}
		return errRespBytes, nil
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
		s.logger.Debug("Handler returned nil result bytes for request, using JSON null.", logFields...)
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

	// +++ Keep Debug Logging +++
	if request.Method == "ping" {
		marshalledResponseObjForLog, _ := json.Marshal(responseObj)
		s.logger.Debug(">>> DEBUG: Preparing to marshal 'ping' response.",
			"responseObject", string(marshalledResponseObjForLog),
			"resultFieldContent", string(responseObj.Result))
	}
	// +++ End Debug Logging +++

	respBytes, marshalErr := json.Marshal(responseObj)
	if marshalErr != nil {
		s.logger.Error("Internal error: Failed to marshal successful response.", append(logFields, "error", marshalErr)...)
		errRespBytes, _ := s.createErrorResponse(nil, errors.Wrap(marshalErr, "internal error: failed to marshal success response"), request.ID)
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
