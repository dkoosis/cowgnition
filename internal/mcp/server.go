// file: internal/mcp/server.go
package mcp

import (
	"context"
	"fmt" // Import fmt for error formatting.

	// Import slog.
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// Initialize the logger at the package level.
var serverLogger = logging.GetLogger("mcp_server") // Changed from logger to serverLogger.

// Config defines the interface for server configuration.
type Config interface {
	GetServerName() string
	GetServerAddress() string
}

// Server is the main MCP server implementation.
type Server struct {
	config          Config
	version         string
	transport       string
	requestTimeout  time.Duration
	shutdownTimeout time.Duration
	resourceManager ResourceManager
	toolManager     ToolManager
}

// NewServer creates a new MCP server instance.
func NewServer(config Config) (*Server, error) {
	// Log server creation? Optional, might be too noisy.
	// serverLogger.Debug("Creating new MCP server instance")
	return &Server{
		config:          config,
		version:         "1.0.0", // Default version
		transport:       "stdio", // Default transport
		requestTimeout:  30 * time.Second,
		shutdownTimeout: 5 * time.Second,
		resourceManager: NewResourceManager(),
		toolManager:     NewToolManager(),
	}, nil
}

// SetVersion sets the server version.
func (s *Server) SetVersion(version string) {
	s.version = version
}

// Version returns the server version.
func (s *Server) Version() string {
	return s.version
}

// SetTransport sets the transport type (http or stdio).
func (s *Server) SetTransport(transportType string) error {
	if transportType != "http" && transportType != "stdio" {
		// Standardized error creation using cgerr helper
		err := cgerr.NewInvalidArgumentsError(
			fmt.Sprintf("unsupported transport type: %s", transportType),
			map[string]interface{}{
				"transport_type": transportType,
				"valid_types":    []string{"http", "stdio"},
			},
		)
		serverLogger.Error("Invalid transport type specified", "transport", transportType, "error", fmt.Sprintf("%+v", err))
		return err
	}
	s.transport = transportType
	return nil
}

// Transport returns the current transport type.
func (s *Server) Transport() string {
	return s.transport
}

// SetRequestTimeout sets the request timeout.
func (s *Server) SetRequestTimeout(timeout time.Duration) {
	s.requestTimeout = timeout
}

// RequestTimeout returns the request timeout.
func (s *Server) RequestTimeout() time.Duration {
	return s.requestTimeout
}

// SetShutdownTimeout sets the shutdown timeout.
func (s *Server) SetShutdownTimeout(timeout time.Duration) {
	s.shutdownTimeout = timeout
}

// ShutdownTimeout returns the shutdown timeout.
func (s *Server) ShutdownTimeout() time.Duration {
	return s.shutdownTimeout
}

// ResourceManager returns the resource manager.
func (s *Server) ResourceManager() ResourceManager {
	return s.resourceManager
}

// ToolManager returns the tool manager.
func (s *Server) ToolManager() ToolManager {
	return s.toolManager
}

// RegisterResourceProvider registers a resource provider.
func (s *Server) RegisterResourceProvider(provider ResourceProvider) {
	s.resourceManager.RegisterProvider(provider)
	serverLogger.Info("Registered resource provider", "provider_type", fmt.Sprintf("%T", provider))
}

// RegisterToolProvider registers a tool provider.
func (s *Server) RegisterToolProvider(provider ToolProvider) {
	s.toolManager.RegisterProvider(provider)
	serverLogger.Info("Registered tool provider", "provider_type", fmt.Sprintf("%T", provider))
}

// Config returns the server configuration.
func (s *Server) Config() Config {
	return s.config
}

// Start starts the server with the configured transport.
func (s *Server) Start() error {
	serverLogger.Info("Starting MCP server", "version", s.version, "transport", s.transport)
	switch s.transport {
	case "http":
		return s.startHTTP()
	case "stdio":
		return s.startStdio()
	default:
		// This case should ideally be prevented by SetTransport validation
		// Standardized error creation using cgerr helper
		err := cgerr.NewInvalidArgumentsError(
			fmt.Sprintf("unsupported transport type during start: %s", s.transport),
			map[string]interface{}{
				"transport_type": s.transport,
				"valid_types":    []string{"http", "stdio"},
			},
		)
		serverLogger.Error("Cannot start server, unsupported transport", "transport", s.transport, "error", fmt.Sprintf("%+v", err))
		return err
	}
}

// startHTTP starts the HTTP transport server.
func (s *Server) startHTTP() error {
	serverLogger.Info("Starting HTTP transport...", "address", s.config.GetServerAddress()) // Assuming address is relevant for HTTP
	// Implementation for HTTP transport
	// Standardized error creation using cgerr helper
	err := cgerr.NewInternalError(
		"HTTP transport not yet implemented",
		nil,
		map[string]interface{}{
			"server_name": s.config.GetServerName(),
		},
	)
	serverLogger.Error("HTTP transport start failed", "error", fmt.Sprintf("%+v", err))
	return err // Return the error after logging
}

// startStdio starts the server using the stdio transport.
func (s *Server) startStdio() error {
	serverLogger.Info("Starting Stdio transport...") // Add info log for starting stdio

	// Create a JSON-RPC handler
	// TODO: Replace this placeholder handler with the actual MCP handler
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Basic handler implementation - should route to resource/tool managers
		serverLogger.Warn("Received request with placeholder stdio handler", "method", req.Method)
		// Standardized error creation using cgerr helper
		return nil, cgerr.NewMethodNotFoundError(
			req.Method,
			map[string]interface{}{
				"server_name":  s.config.GetServerName(),
				"handler_type": "placeholder",
			},
		)
	})

	// Set up stdio transport options
	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(s.requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second), // Consider making these configurable
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),
	}
	serverLogger.Debug("Stdio transport options configured",
		"request_timeout", s.requestTimeout,
		"read_timeout", "120s",
		"write_timeout", "30s",
	)

	// Start the stdio server
	if err := jsonrpc.RunStdioServer(handler, stdioOpts...); err != nil {
		// Log the error before wrapping and returning (as per assessment example intent)
		serverLogger.Error("Failed to run stdio server", "error", fmt.Sprintf("%+v", err))

		// Wrap the error using cgerr details
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to start stdio server"), // Keep the wrap
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"server_name":     s.config.GetServerName(),
				"request_timeout": s.requestTimeout.String(),
				// Add other relevant options if needed
			},
		)
	}

	serverLogger.Info("Stdio transport finished.") // Log normal exit if RunStdioServer returns nil
	return nil
}

// Stop stops the server.
func (s *Server) Stop() error {
	serverLogger.Info("Stopping MCP server...")
	// Implementation for stopping the server (e.g., closing connections, context cancellation)
	// Depending on the transport, might need specific shutdown logic.
	return nil
}
