// file: internal/mcp/server.go
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Server represents the base MCP server structure.
// It handles configuration and basic transport setup.
// Request handling logic might be delegated (e.g., via ConnectionServer).
type Server struct {
	config          *config.Settings
	version         string
	transport       string
	httpServer      *http.Server    // Used only for HTTP transport
	resourceManager ResourceManager // Manages resources (interface)
	toolManager     ToolManager     // Manages tools (interface)
	requestTimeout  time.Duration
	shutdownTimeout time.Duration
}

// NewServer creates a new base MCP server.
func NewServer(cfg *config.Settings) (*Server, error) {
	if cfg == nil {
		return nil, cgerr.ErrorWithDetails(
			errors.New("config cannot be nil"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{"config": "nil"},
		)
	}

	// Create new resource and tool managers
	rm := NewResourceManager()
	tm := NewToolManager()
	if rm == nil || tm == nil {
		// Handle error: Failed to initialize managers
		return nil, errors.New("failed to initialize resource or tool manager")
	}

	server := &Server{
		config:          cfg,
		version:         "0.1.0",          // Default version, consider setting from build info
		transport:       "http",           // Default transport
		resourceManager: rm,               // Store interface, not pointer to interface
		toolManager:     tm,               // Store interface, not pointer to interface
		requestTimeout:  30 * time.Second, // Default request timeout
		shutdownTimeout: 5 * time.Second,  // Default shutdown timeout
	}

	return server, nil
}

// SetVersion sets the server version.
func (s *Server) SetVersion(version string) {
	s.version = version
}

// SetTransport sets the transport type ("http" or "stdio").
func (s *Server) SetTransport(transport string) error {
	if transport != "http" && transport != "stdio" {
		return cgerr.ErrorWithDetails(
			errors.Newf("unsupported transport: %s", transport),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"transport":     transport,
				"valid_options": []string{"http", "stdio"},
			},
		)
	}
	s.transport = transport
	return nil
}

// SetRequestTimeout sets the request timeout for handlers.
func (s *Server) SetRequestTimeout(timeout time.Duration) {
	s.requestTimeout = timeout
}

// SetShutdownTimeout sets the graceful shutdown timeout.
func (s *Server) SetShutdownTimeout(timeout time.Duration) {
	s.shutdownTimeout = timeout
}

// RegisterResourceProvider registers a resource provider with the ResourceManager.
func (s *Server) RegisterResourceProvider(provider ResourceProvider) {
	if s.resourceManager == nil {
		// Handle error: manager not initialized
		fmt.Println("Error: ResourceManager not initialized before registering provider") // Replace with proper logging/error
		return
	}
	s.resourceManager.RegisterProvider(provider)
}

// RegisterToolProvider registers a tool provider with the ToolManager.
func (s *Server) RegisterToolProvider(provider ToolProvider) {
	if s.toolManager == nil {
		// Handle error: manager not initialized
		fmt.Println("Error: ToolManager not initialized before registering provider") // Replace with proper logging/error
		return
	}
	s.toolManager.RegisterProvider(provider)
}

// Start starts the MCP server using the configured transport.
// Note: This starts the *base* server logic. If using ConnectionServer,
// ConnectionServer.Start() should be called instead, which overrides parts of this.
func (s *Server) Start() error {
	fmt.Printf("Attempting to start base Server with %s transport...\n", s.transport) // Added log
	switch s.transport {
	case "http":
		return s.startHTTP()
	case "stdio":
		return s.startStdio()
	default:
		return errors.Newf("unsupported transport in base Server Start: %s", s.transport)
	}
}

// startHTTP starts the MCP server with HTTP transport.
// Note: Handler registration is commented out, assuming ConnectionServer/Manager handles requests.
func (s *Server) startHTTP() error {
	// Create a JSON-RPC adapter
	adapter := jsonrpc.NewAdapter(jsonrpc.WithTimeout(s.requestTimeout))

	// Create an HTTP handler
	httpHandler := jsonrpc.NewHTTPHandler(adapter, jsonrpc.WithHTTPRequestTimeout(s.requestTimeout))

	// Create an HTTP server
	address := s.config.GetServerAddress() // Assume GetServerAddress exists
	s.httpServer = &http.Server{
		Addr:         address,
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,  // Consider making configurable
		WriteTimeout: 30 * time.Second,  // Consider making configurable
		IdleTimeout:  120 * time.Second, // Consider making configurable
	}

	// Start the HTTP server
	fmt.Printf("Starting base HTTP server on %s (Handler registration likely bypassed by ConnectionServer)\n", address)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to start HTTP server"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{"address": address},
		)
	}
	fmt.Println("Base HTTP server finished.") // Added log
	return nil
}

// startStdio starts the MCP server with stdio transport.
// Note: Handler registration is commented out, assuming ConnectionServer/Manager handles requests.
func (s *Server) startStdio() error {
	// Create a JSON-RPC adapter
	adapter := jsonrpc.NewAdapter(jsonrpc.WithTimeout(s.requestTimeout))

	// Set up stdio transport options
	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(s.requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second),
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),
		// Consider adding jsonrpc.WithStdioDebug based on config/flags
	}

	fmt.Println("Starting base stdio server (Handler registration likely bypassed by ConnectionServer)")
	// Start the stdio server using the adapter (if non-state-machine path needed)
	// For the ConnectionServer path, ConnectionServer.startStdio sets up its own handler.
	if err := jsonrpc.RunStdioServer(adapter, stdioOpts...); err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to start stdio server"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"request_timeout": s.requestTimeout.String(),
				"read_timeout":    "120s",
				"write_timeout":   "30s",
			},
		)
	}
	fmt.Println("Base stdio server finished.") // Added log
	return nil
}

// Stop gracefully stops the MCP server (currently only handles HTTP).
func (s *Server) Stop() error {
	if s.httpServer != nil {
		fmt.Println("Stopping HTTP server...")
		ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			return cgerr.ErrorWithDetails(
				errors.Wrap(err, "failed to shutdown HTTP server gracefully"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{"timeout": s.shutdownTimeout.String()},
			)
		}
		fmt.Println("HTTP server stopped.") // Added log
	} else {
		fmt.Println("Stop called, but no active HTTP server found.") // Added log
	}
	// TODO: Add logic to stop stdio transport if needed/possible
	return nil
}
