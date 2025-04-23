// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/mcp_server.go
package mcp

import (
	"context"
	"encoding/json" // Add json import.
	"fmt"
	"net/url" // Add url import for ReadResource URI parsing.
	"os"
	"strings" // Add strings import.
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import mcp_errors.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"       // Use mcp_types for interfaces/types.
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

// ServeSTDIO configures and starts the server using stdio transport.
// It now builds a middleware chain ending in the refactored s.routeMessage.
func (s *Server) ServeSTDIO(ctx context.Context) error {
	s.logger.Info("Starting server with stdio transport.")
	s.transport = transport.NewNDJSONTransport(os.Stdin, os.Stdout, os.Stdin, s.logger) // Stdin used as closer.

	// Setup validation middleware.
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = true // Keep true.

	// Interim Fix Note: StrictOutgoing=false needed for now due to initialize response warning.
	if s.options.Debug {
		validationOpts.StrictOutgoing = true // Be strict in debug.
		validationOpts.MeasurePerformance = true
		s.logger.Info("Debug mode enabled: outgoing validation is STRICT.")
	} else {
		validationOpts.StrictOutgoing = false // Allow known warnings in normal mode.
		s.logger.Info("Non-debug mode: outgoing validation is NON-STRICT (logs warnings).")
	}

	// Add SkipTypes for core notifications that don't need schema validation.
	// We can add more here as needed.
	validationOpts.SkipTypes = map[string]bool{
		"notifications/initialized": true,
		"notifications/cancelled":   true,
		"notifications/progress":    true,
		"exit":                      true, // Exit is a notification.
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		s.validator,
		validationOpts,
		s.logger.WithField("subcomponent", "validation_mw"),
	)

	// Build middleware chain. The final handler is s.routeMessage.
	chain := middleware.NewChain(s.routeMessage) // Target the router func.
	chain.Use(validationMiddleware)              // Add validation middleware first.

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
func (s *Server) Shutdown(_ context.Context) error {
	s.logger.Info("Shutting down MCP server.")

	// Shutdown registered services.
	s.serviceLock.RLock()
	servicesToShutdown := make([]services.Service, 0, len(s.services))
	for name, service := range s.services {
		s.logger.Debug("Preparing to shutdown service.", "serviceName", name)
		servicesToShutdown = append(servicesToShutdown, service)
	}
	s.serviceLock.RUnlock() //nolint:staticcheck // SA2001: Intentional unlock before calling potentially blocking service Shutdown methods.

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
		// The overall Shutdown function is controlled by the input ctx.
		// The closeCtx was unused because transport.Close likely doesn't take ctx.
		// Remove the unused variable declaration.
		// closeCtx, cancel := context.WithTimeout(ctx, s.options.ShutdownTimeout) // REMOVED.
		// defer cancel() // REMOVED.

		s.logger.Debug("Closing transport...")
		if err := s.transport.Close(); err != nil {
			// Log error but continue shutdown process.
			s.logger.Error("Failed to close transport during shutdown.", "error", fmt.Sprintf("%+v", err))
		} else {
			s.logger.Debug("Transport closed successfully.")
		}
	} else {
		s.logger.Warn("Shutdown called but transport was nil.")
	}

	s.logger.Info("Server shutdown sequence completed.")
	return nil
} // Removed trailing newline.

// routeMessage is the final handler in the middleware chain.
// It parses the message, validates sequence, and routes to appropriate handlers or services.
func (s *Server) routeMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"` // Use RawMessage to check presence/null.
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"` // Use RawMessage.
	}

	// We assume the message passed schema validation in middleware.
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		// Should not happen if schema validation ran, but handle defensively.
		s.logger.Error("Internal error: Failed to unmarshal validated message in routeMessage.", "error", err)
		// Use createErrorResponse for consistent formatting. Pass original bytes.
		return s.createErrorResponse(msgBytes, errors.Wrap(err, "internal error: failed to parse validated message"))
	}

	// Add connection state validation (safety net).
	if s.connectionState == nil {
		s.logger.Error("Internal error: connectionState is nil in routeMessage.")
		return s.createErrorResponse(msgBytes, errors.New("internal server error: connection state missing"))
	}
	if err := s.connectionState.ValidateMethodSequence(request.Method); err != nil {
		s.logger.Warn("Method sequence validation failed.", "method", request.Method, "state", s.connectionState.CurrentState(), "error", err)
		// Map sequence error to JSON-RPC error. Pass original bytes.
		return s.createErrorResponse(msgBytes, err)
	}

	isNotification := (request.ID == nil || string(request.ID) == "null")
	s.logger.Debug("Routing message.", "method", request.Method, "id", string(request.ID), "isNotification", isNotification)

	// --- Route Based on Method ---.
	var resultBytes json.RawMessage
	var handlerErr error

	// 1. Core MCP Methods (Handled directly by Server/Handler).
	// TODO (Refactor-CoreHandlers): Refactor core MCP method handling (ping, initialize, etc.) directly into Server struct, removing dependency on temporary NewHandler(). See TODO.md.
	coreHandler := NewHandler(s.config, s.validator, s.startTime, s.connectionState, s.logger)

	switch request.Method {
	case "ping":
		resultBytes, handlerErr = coreHandler.handlePing(ctx, request.Params)
	case "initialize":
		// Initialize needs special handling to update state.
		resultBytes, handlerErr = coreHandler.handleInitialize(ctx, request.Params)
		if handlerErr == nil {
			// Update server's main connection state upon successful initialize response generation.
			s.connectionState.SetInitialized()
			s.logger.Info("Connection state set to initialized after successful initialize handling.")
		}
	case "shutdown":
		resultBytes, handlerErr = coreHandler.handleShutdown(ctx, request.Params)
		// Shutdown method itself doesn't close transport, just acknowledges.
		// Actual shutdown happens via Server.Shutdown().
	case "exit":
		// Exit is a notification, call handler but expect no result bytes.
		handlerErr = coreHandler.handleExit(ctx, request.Params) // Use error return.
		resultBytes = nil                                        // Explicitly nil for notifications.
	case "$/cancelRequest":
		// Cancel is a notification.
		handlerErr = coreHandler.handleCancelRequest(ctx, request.Params) // Use error return.
		resultBytes = nil                                                 // Explicitly nil for notifications.

	// 2. Tool-related Methods.
	case "tools/list":
		resultBytes, handlerErr = s.handleListTools(ctx) // New aggregate handler.
	case "tools/call":
		resultBytes, handlerErr = s.handleServiceDelegation(ctx, request.Method, request.Params) // Delegate.

	// 3. Resource-related Methods.
	case "resources/list":
		resultBytes, handlerErr = s.handleListResources(ctx) // New aggregate handler.
	case "resources/read":
		resultBytes, handlerErr = s.handleServiceDelegation(ctx, request.Method, request.Params) // Delegate.
	// TODO: Add resources/subscribe, resources/unsubscribe if implementing subscriptions.

	// 4. Prompt-related Methods.
	case "prompts/list":
		resultBytes, handlerErr = s.handleListPrompts(ctx) // New aggregate handler.
	case "prompts/get":
		resultBytes, handlerErr = s.handleServiceDelegation(ctx, request.Method, request.Params) // Delegate.

	// 5. Other methods (logging, completion, etc.).
	// TODO: Add handlers for other MCP methods as needed.

	default:
		// Method not handled by core or known service prefixes.
		s.logger.Warn("Method not found during routing.", "method", request.Method)
		// Use mcperrors constants. Ensure mcperrors package defines NewProtocolError.
		// Assuming a structure like NewProtocolError(code, message, cause error, context map[string]interface{}).
		handlerErr = mcperrors.NewProtocolError(mcperrors.ErrMethodNotFound, fmt.Sprintf("Method not found: %s", request.Method), nil, nil)
	}

	// --- Handle Handler Errors ---.
	if handlerErr != nil {
		// Let the main processing loop handle creating/sending the error response.
		s.logger.Warn("Error returned from routed handler.", "method", request.Method, "error", fmt.Sprintf("%+v", handlerErr))
		return nil, handlerErr // Propagate error to processNextMessage/handleProcessingError.
	}

	// --- Handle Notifications ---.
	if isNotification {
		s.logger.Debug("Processed notification, no response needed.", "method", request.Method)
		return nil, nil // No response bytes, no error.
	}

	// --- Construct and Marshal Success Response ---.
	if resultBytes == nil {
		// If handler succeeded but returned nil bytes for a request, use JSON null.
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
		s.logger.Error("Internal error: Failed to marshal successful response.", "method", request.Method, "error", marshalErr)
		// Return marshalling error to be handled by main loop.
		return nil, errors.Wrap(marshalErr, "internal error: failed to marshal success response")
	}

	s.logger.Debug("Successfully processed request, returning response.", "method", request.Method, "id", string(request.ID))
	return respBytes, nil
}

// --- NEW Aggregate Handlers ---.

// handleListTools aggregates tools from all registered services.
func (s *Server) handleListTools(_ context.Context) (json.RawMessage, error) {
	allTools := []mcptypes.Tool{}
	s.serviceLock.RLock()
	for _, service := range s.services {
		allTools = append(allTools, service.GetTools()...)
	}
	s.serviceLock.RUnlock()

	// TODO: Implement pagination if needed.
	result := mcptypes.ListToolsResult{
		Tools: allTools,
		// NextCursor: "",
	}
	// Use standard json.Marshal for consistency.
	return json.Marshal(result)
}

// handleListResources aggregates resources from all registered services.
func (s *Server) handleListResources(_ context.Context) (json.RawMessage, error) {
	allResources := []mcptypes.Resource{}
	s.serviceLock.RLock()
	for _, service := range s.services {
		allResources = append(allResources, service.GetResources()...)
	}
	s.serviceLock.RUnlock()

	// TODO: Implement pagination if needed.
	result := mcptypes.ListResourcesResult{
		Resources: allResources,
		// NextCursor: "",
	}
	// Use standard json.Marshal for consistency.
	return json.Marshal(result)
}

// File: internal/mcp/mcp_server.go.

// handleListPrompts aggregates prompts from all registered services.
func (s *Server) handleListPrompts(_ context.Context) (json.RawMessage, error) { // Rename ctx to _.
	allPrompts := []mcptypes.Prompt{} // Assuming Prompt type exists in mcptypes.
	s.serviceLock.RLock()             // Lock before accessing services.
	// TODO: Add GetPrompts() method to services.Service interface if implementing prompts.
	// for _, service := range s.services {.
	//  allPrompts = append(allPrompts, service.GetPrompts()...).
	// }.
	s.serviceLock.RUnlock() // Unlock after accessing services. // <--- Moved RUnlock here.
	s.logger.Warn("Prompts/list called, but GetPrompts not implemented on services yet.")

	// TODO: Implement pagination if needed.
	result := mcptypes.ListPromptsResult{
		Prompts: allPrompts, // Will be empty for now.
		// NextCursor: "",
	}
	// Use standard json.Marshal for consistency.
	return json.Marshal(result)
}

// --- NEW Service Delegation Handler ---.

// handleServiceDelegation routes requests like tools/call and resources/read to the correct service.
func (s *Server) handleServiceDelegation(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	// Example: "tools/call" -> need tool name from params.
	// Example: "resources/read" -> need URI from params.

	var serviceName string
	var specificArgs interface{} // To hold parsed args/URI.

	switch method {
	case "tools/call":
		var req mcptypes.CallToolRequest
		if err := json.Unmarshal(params, &req); err != nil {
			// Use mcperrors constants. Ensure mcperrors package defines NewInvalidParamsError.
			// Assuming a structure like NewInvalidParamsError(message, cause, context).
			return nil, mcperrors.NewInvalidParamsError("invalid params structure for tools/call", err, nil)
		}
		// Extract service name prefix (e.g., "rtm" from "rtm_getTasks").
		parts := strings.SplitN(req.Name, "_", 2)
		if len(parts) < 2 || parts[0] == "" {
			// Use mcperrors constants. Ensure mcperrors package defines NewMethodNotFoundError.
			// Assuming a structure like NewMethodNotFoundError(message, cause, context).
			return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Invalid tool name format: %s. Expected 'serviceName_toolAction'.", req.Name), nil, map[string]interface{}{"toolName": req.Name})
		}
		serviceName = parts[0]
		specificArgs = req // Pass the whole request struct to CallTool for context.

	case "resources/read":
		var req mcptypes.ReadResourceRequest
		if err := json.Unmarshal(params, &req); err != nil {
			// Use mcperrors constants. Ensure mcperrors package defines NewInvalidParamsError.
			return nil, mcperrors.NewInvalidParamsError("invalid params structure for resources/read", err, nil)
		}
		// Extract service name from URI scheme (e.g., "rtm" from "rtm://lists").
		parsedURI, err := url.Parse(req.URI)
		if err != nil || parsedURI.Scheme == "" {
			// Use mcperrors constants. Ensure mcperrors package defines NewResourceError.
			return nil, mcperrors.NewResourceError(mcperrors.ErrResourceInvalid, fmt.Sprintf("Invalid or missing scheme in resource URI: %s", req.URI), err, map[string]interface{}{"uri": req.URI})
		}
		serviceName = parsedURI.Scheme
		specificArgs = req.URI // Pass the URI string to ReadResource.

	// Add cases for prompts/get, etc. if needed.

	default:
		// Use mcperrors constants. Ensure mcperrors package defines NewMethodNotFoundError.
		return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Unsupported method for service delegation: %s", method), nil, map[string]interface{}{"method": method})
	}

	// Find and call the service.
	service, found := s.GetService(serviceName)
	if !found {
		// Use mcperrors constants. Ensure mcperrors package defines NewServiceNotFoundError (or similar).
		// Using MethodNotFound for now as ServiceNotFound isn't defined in mcperrors.go snippet.
		return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Service '%s' not found to handle method '%s'", serviceName, method), nil, map[string]interface{}{"service": serviceName})
	}

	var result interface{}
	var callErr error

	// Delegate based on method.
	switch method {
	case "tools/call":
		toolReq := specificArgs.(mcptypes.CallToolRequest) // Type assertion.
		result, callErr = service.CallTool(ctx, toolReq.Name, toolReq.Arguments)
	case "resources/read":
		uri := specificArgs.(string) // Type assertion.
		result, callErr = service.ReadResource(ctx, uri)
		// Add cases for prompts/get, etc.

	}

	if callErr != nil {
		// Errors from the service call (e.g., internal RTM errors, resource not found).
		// should be returned here to be mapped by createErrorResponse.
		return nil, callErr
	}

	// Marshal the successful result from the service.
	// Use standard json.Marshal for consistency.
	return json.Marshal(result)
}
