// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/rtm"
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
	validator       schema.ValidatorInterface
	connectionState *ConnectionState
	rtmService      *rtm.Service
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

	// Connect metrics collector to RTM
	if rtmService != nil {
		rtm.SetMetricsCollector(mcp.GetMetricsCollector())
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

// file: internal/mcp/mcp_server.go (partial update)

// GetResources returns all available MCP resources from all providers
func (s *Server) GetResources() []Resource {
	resources := s.GetServerResources() // Add server resources

	// Add RTM resources if available
	if s.rtmService != nil {
		resources = append(resources, s.rtmService.GetResources()...)
	}

	return resources
}

// ReadResource handles resource read requests
func (s *Server) ReadResource(ctx context.Context, uri string) ([]interface{}, error) {
	startTime := time.Now()
	var result []interface{}
	var err error

	// Route based on URI prefix
	if strings.HasPrefix(uri, "rtm://") {
		if s.rtmService != nil {
			result, err = s.rtmService.ReadResource(ctx, uri)
		} else {
			err = errors.New("RTM service not available")
		}
	} else if strings.HasPrefix(uri, "cowgnition://server/") {
		result, err = s.ReadServerResource(ctx, uri)
	} else {
		err = errors.Newf("unknown resource URI scheme: %s", uri)
	}

	// Record request metrics
	s.RecordRequestMetrics("ReadResource:"+uri, startTime, err)

	return result, err
}

// serverProcessing is the actual implementation, located in mcp_server_processing.go.
// func (s *Server) serverProcessing(ctx context.Context, handlerFunc mcptypes.MessageHandler) error { ... }

// handleMessage is the final handler in the middleware chain, located in mcp_server_processing.go.
// func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) { ... }
// file: internal/mcp/mcp_server.go

// GetServerResources returns server-specific resources without including service resources
func (s *Server) GetServerResources() []Resource {
	// Return server-specific resources
	// These are resources that are directly provided by the server itself,
	// not by integrated services like RTM
	return []Resource{
		{
			Name:        "Server Status",
			URI:         "cowgnition://server/status",
			Description: "Information about the server status and metrics",
			MimeType:    "application/json",
		},
		{
			Name:        "Server Version",
			URI:         "cowgnition://server/version",
			Description: "Version information for the server and its components",
			MimeType:    "application/json",
		},
		// Add any other server-specific resources
	}
}

// ReadServerResource handles reading resources directly provided by the server
func (s *Server) ReadServerResource(ctx context.Context, uri string) ([]interface{}, error) {
	// Handle server-specific resource URIs
	switch uri {
	case "cowgnition://server/status":
		return s.getServerStatusResource(), nil
	case "cowgnition://server/version":
		return s.getServerVersionResource(), nil
	default:
		return nil, errors.Newf("unknown server resource: %s", uri)
	}
}

// getServerStatusResource returns the server status information as a resource
func (s *Server) getServerStatusResource() []interface{} {
	// Create a status object with relevant information
	status := map[string]interface{}{
		"uptime":      time.Since(s.startTime).String(),
		"startTime":   s.startTime.Format(time.RFC3339),
		"initialized": s.connectionState.IsInitialized(),
	}

	// Return as a text resource
	return []interface{}{
		TextResourceContents{
			ResourceContents: ResourceContents{
				URI:      "cowgnition://server/status",
				MimeType: "application/json",
			},
			Text: mustMarshalJSON(status),
		},
	}
}

// getServerVersionResource returns version information as a resource
func (s *Server) getServerVersionResource() []interface{} {
	// Create a version info object
	versionInfo := map[string]interface{}{
		"serverName":      s.config.Server.Name,
		"protocolVersion": s.connectionState.GetProtocolVersion(),
		// Add other version information
	}

	// Return as a text resource
	return []interface{}{
		TextResourceContents{
			ResourceContents: ResourceContents{
				URI:      "cowgnition://server/version",
				MimeType: "application/json",
			},
			Text: mustMarshalJSON(versionInfo),
		},
	}
}

// mustMarshalJSON marshals data to JSON and panics on error
// Only used internally where we control the input values
func mustMarshalJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal JSON: %v", err))
	}
	return string(data)
}

// file: internal/mcp/mcp_server.go

// Add these methods to the Server struct:

// GetServerResources returns server-specific resources only
func (s *Server) GetServerResources() []Resource {
	// Server-specific resources
	return []Resource{
		{
			Name:        "Server Status",
			URI:         "cowgnition://server/status",
			Description: "Information about the server status and metrics",
			MimeType:    "application/json",
		},
		// Add other server resources as needed
	}
}

// ReadServerResource handles reading resources directly provided by the server
func (s *Server) ReadServerResource(ctx context.Context, uri string) ([]interface{}, error) {
	switch uri {
	case "cowgnition://server/status":
		// Create server status resource
		status := map[string]interface{}{
			"uptime":     time.Since(s.startTime).String(),
			"startTime":  s.startTime.Format(time.RFC3339),
			"configured": s.config != nil,
		}

		// Convert to JSON
		jsonData, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal server status")
		}

		// Return as text resource
		return []interface{}{
			TextResourceContents{
				ResourceContents: ResourceContents{
					URI:      uri,
					MimeType: "application/json",
				},
				Text: string(jsonData),
			},
		}, nil

	default:
		return nil, errors.Newf("unknown server resource: %s", uri)
	}
}
