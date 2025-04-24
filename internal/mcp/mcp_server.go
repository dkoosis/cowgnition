// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server.go

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/services"
	"github.com/dkoosis/cowgnition/internal/transport"
	// Assuming core handler definitions are in this package or imported.
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

	// Service Registry.
	services    map[string]services.Service // Map service name (e.g., "rtm") to its implementation.
	serviceLock sync.RWMutex                // Mutex to protect the services map.

	connectionState *ConnectionState

	// Handler Instances
	coreHandler *Handler // Pointer to the core handler instance (defined in handlers_core.go)
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

	server := &Server{
		config:          cfg,
		options:         opts,
		logger:          logger.WithField("component", "mcp_server"),
		validator:       validator,
		startTime:       startTime,
		connectionState: connState,
		services:        make(map[string]services.Service), // Initialize the service registry.
		coreHandler:     NewHandler(cfg, validator, startTime, connState, logger),
	}

	server.logger.Info("MCP Server instance created (services need registration).")
	return server, nil
}

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

	// Setup validation middleware.
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

	// Add SkipTypes for core notifications that don't need schema validation.
	validationOpts.SkipTypes = map[string]bool{
		"notifications/initialized": true,
		"notifications/cancelled":   true, // Skip validation for this now handled notification
		"notifications/progress":    true,
		"exit":                      true,
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
	// Ensure serverProcessing is defined in mcp_server_processing.go
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

// routeMessage is the final handler in the middleware chain.
// It parses the message, validates sequence, and routes to appropriate handlers or services.
func (s *Server) routeMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"` // Use RawMessage to check presence/null.
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"` // Use RawMessage.
	}

	if err := json.Unmarshal(msgBytes, &request); err != nil {
		s.logger.Error("Internal error: Failed to unmarshal validated message in routeMessage.", "error", err)
		reqID := extractRequestID(s.logger, msgBytes) // Ensure extractRequestID is defined in error_handling file
		idToUseInResponse := reqID
		if idToUseInResponse == nil || string(idToUseInResponse) == "null" {
			idToUseInResponse = json.RawMessage("0")
		}
		// Ensure createErrorResponse is defined in error_handling file
		return s.createErrorResponse(msgBytes, errors.Wrap(err, "internal error: failed to parse validated message"), idToUseInResponse)
	}

	if s.connectionState == nil {
		s.logger.Error("Internal error: connectionState is nil in routeMessage.")
		reqID := extractRequestID(s.logger, msgBytes) // Ensure extractRequestID is defined in error_handling file
		idToUseInResponse := reqID
		if idToUseInResponse == nil || string(idToUseInResponse) == "null" {
			idToUseInResponse = json.RawMessage("0")
		}
		// Ensure createErrorResponse is defined in error_handling file
		return s.createErrorResponse(msgBytes, errors.New("internal server error: connection state missing"), idToUseInResponse)
	}

	if err := s.connectionState.ValidateMethodSequence(request.Method); err != nil {
		s.logger.Warn("Method sequence validation failed.", "method", request.Method, "state", s.connectionState.CurrentState(), "error", err)
		return nil, err // Error is returned to be handled by handleProcessingError
	}

	isNotification := (request.ID == nil || string(request.ID) == "null")
	s.logger.Debug("Routing message.", "method", request.Method, "id", string(request.ID), "isNotification", isNotification)

	var resultBytes json.RawMessage
	var handlerErr error

	if s.coreHandler == nil {
		s.logger.Error("Internal error: s.coreHandler is nil in routeMessage.")
		handlerErr = errors.New("internal server error: core handler not available")
	} else {
		params := request.Params

		switch request.Method {
		// --- Core Handlers ---
		case "ping":
			resultBytes, handlerErr = s.coreHandler.handlePing(ctx, params)
		case "initialize":
			resultBytes, handlerErr = s.coreHandler.handleInitialize(ctx, params)
		case "notifications/initialized":
			handlerErr = s.coreHandler.handleNotificationsInitialized(ctx, params)
			resultBytes = nil
		case "shutdown":
			if s.connectionState != nil {
				s.connectionState.SetShutdown()
			}
			resultBytes, handlerErr = s.coreHandler.handleShutdown(ctx, params)
		case "exit":
			handlerErr = s.coreHandler.handleExit(ctx, params)
			resultBytes = nil
		case "$/cancelRequest":
			handlerErr = s.coreHandler.handleCancelRequest(ctx, params)
			resultBytes = nil
		// --- Added case for notifications/cancelled ---
		case "notifications/cancelled":
			s.logger.Info("Received (but ignoring) notifications/cancelled.")
			// TODO: Implement actual cancellation logic if needed by looking up request ID etc.
			handlerErr = nil // It's a notification, successful handling is nil error
			resultBytes = nil

		// --- Aggregation Handlers ---
		case "tools/list":
			resultBytes, handlerErr = s.handleListTools(ctx)
		case "resources/list":
			resultBytes, handlerErr = s.handleListResources(ctx)
		case "prompts/list":
			resultBytes, handlerErr = s.handleListPrompts(ctx)

		// --- Delegation Handlers ---
		case "tools/call", "resources/read", "prompts/get": // Group delegation cases
			resultBytes, handlerErr = s.handleServiceDelegation(ctx, request.Method, params)

		// --- Other Methods ---
		// TODO: Add handlers for logging/setLevel, completion/complete etc.

		default:
			s.logger.Warn("Method not found during routing.", "method", request.Method)
			handlerErr = mcperrors.NewProtocolError(mcperrors.ErrMethodNotFound, fmt.Sprintf("Method not found: %s", request.Method), nil, nil)
		}
	}

	if handlerErr != nil {
		s.logger.Warn("Error returned from routed handler.", "method", request.Method, "error", fmt.Sprintf("%+v", handlerErr))
		return nil, handlerErr // Propagate error
	}

	if isNotification {
		s.logger.Debug("Processed notification, no response needed.", "method", request.Method)
		if resultBytes != nil {
			s.logger.Warn("Handler for notification returned non-nil response bytes, discarding.", "method", request.Method)
		}
		return nil, nil
	}

	if resultBytes == nil {
		s.logger.Debug("Handler returned nil result bytes for request, using JSON null.", "method", request.Method, "id", string(request.ID))
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

	s.logger.Debug("Successfully processed request, returning response.", "method", request.Method, "id", string(request.ID))
	return respBytes, nil
}

// handleListTools aggregates tools from all registered services.
func (s *Server) handleListTools(_ context.Context) (json.RawMessage, error) {
	allTools := []mcptypes.Tool{}
	s.serviceLock.RLock()
	for _, service := range s.services {
		allTools = append(allTools, service.GetTools()...)
	}
	s.serviceLock.RUnlock()

	result := mcptypes.ListToolsResult{
		Tools: allTools,
	}
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

	result := mcptypes.ListResourcesResult{
		Resources: allResources, // Ensure `json:"resources"` tag exists in mcptypes.ListResourcesResult
	}
	return json.Marshal(result)
}

// handleListPrompts aggregates prompts from all registered services.
func (s *Server) handleListPrompts(_ context.Context) (json.RawMessage, error) {
	allPrompts := []mcptypes.Prompt{}
	s.serviceLock.RLock()
	for _, service := range s.services {
		_ = service // Avoid unused error if GetPrompts is missing.
	}
	s.serviceLock.RUnlock()
	if len(s.services) > 0 && len(allPrompts) == 0 {
		s.logger.Warn("Prompts/list called, but GetPrompts not implemented or returned no prompts from services.")
	}

	result := mcptypes.ListPromptsResult{
		Prompts: allPrompts,
	}
	return json.Marshal(result)
}

// handleServiceDelegation routes requests like tools/call and resources/read to the correct service.
func (s *Server) handleServiceDelegation(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	var serviceName string
	var specificArgs interface{} // To hold parsed args/URI.

	// --- Determine Service and Parse Args ---
	switch method {
	case "tools/call":
		var req mcptypes.CallToolRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcperrors.NewInvalidParamsError("invalid params structure for tools/call", err, nil)
		}
		parts := strings.SplitN(req.Name, "_", 2)
		if len(parts) < 2 || parts[0] == "" {
			return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Invalid tool name format: %s. Expected 'serviceName_toolAction'", req.Name), nil, map[string]interface{}{"toolName": req.Name})
		}
		serviceName = parts[0]
		specificArgs = req
	case "resources/read":
		var req mcptypes.ReadResourceRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcperrors.NewInvalidParamsError("invalid params structure for resources/read", err, nil)
		}
		parsedURI, err := url.Parse(req.URI)
		if err != nil || parsedURI.Scheme == "" {
			return nil, mcperrors.NewResourceError(mcperrors.ErrResourceInvalid, fmt.Sprintf("Invalid or missing scheme in resource URI: %s", req.URI), err, map[string]interface{}{"uri": req.URI})
		}
		serviceName = parsedURI.Scheme
		specificArgs = req.URI
	case "prompts/get":
		var req mcptypes.GetPromptRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcperrors.NewInvalidParamsError("invalid params structure for prompts/get", err, nil)
		}
		parts := strings.SplitN(req.Name, "_", 2)
		if len(parts) >= 2 && parts[0] != "" {
			serviceName = parts[0]
		} else {
			return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Invalid prompt name format: %s. Expected 'serviceName_promptAction'", req.Name), nil, map[string]interface{}{"promptName": req.Name})
		}
		specificArgs = req
	default:
		return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Unsupported method for service delegation: %s", method), nil, map[string]interface{}{"method": method})
	}

	// --- Find and Call Service ---
	service, found := s.GetService(serviceName)
	if !found {
		return nil, mcperrors.NewMethodNotFoundError(fmt.Sprintf("Service '%s' not found to handle method '%s'", serviceName, method), nil, map[string]interface{}{"service": serviceName})
	}

	var resultData interface{} // Use generic interface for service result
	var callErr error

	// --- Delegate Call ---
	switch method {
	case "tools/call":
		toolReq := specificArgs.(mcptypes.CallToolRequest)
		resultData, callErr = service.CallTool(ctx, toolReq.Name, toolReq.Arguments)
	case "resources/read":
		uri := specificArgs.(string)
		resultData, callErr = service.ReadResource(ctx, uri)
	case "prompts/get":
		promptReq := specificArgs.(mcptypes.GetPromptRequest)
		resultData, callErr = service.GetPrompt(ctx, promptReq.Name, promptReq.Arguments)
	}

	if callErr != nil {
		return nil, callErr // Propagate errors from the service call
	}

	// --- CORRECTED: Wrap result in appropriate MCP type before marshalling ---
	var finalResult interface{}
	switch method {
	case "tools/call":
		// CallTool should return the correct *mcptypes.CallToolResult struct already
		finalResult = resultData
	case "resources/read":
		// ReadResource returns []interface{}, wrap it in ReadResourceResult
		contents, ok := resultData.([]interface{})
		if !ok {
			s.logger.Error("Internal error: ReadResource returned non-slice type unexpectedly.", "type", fmt.Sprintf("%T", resultData))
			return nil, mcperrors.NewInternalError("internal error processing resource data", nil, nil)
		}
		finalResult = mcptypes.ReadResourceResult{Contents: contents} // Wrap the slice
	case "prompts/get":
		// GetPrompt should return the correct *mcptypes.GetPromptResult struct already
		finalResult = resultData
	default:
		finalResult = resultData // Pass through if type unknown (should not happen)
	}

	// Marshal the final wrapped result struct
	return json.Marshal(finalResult)
}

// --- Helper functions like serverProcessing, processNextMessage, createErrorResponse, etc. ---
// --- are intentionally REMOVED from this file as they should exist in ---
// --- mcp_server_processing.go and mcp_server_error_handling.go respectively. ---
