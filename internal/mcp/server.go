// internal/mcp/server.go
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

// Server represents an MCP server.
type Server struct {
	config          *config.Settings
	version         string
	transport       string
	httpServer      *http.Server
	resourceManager *ResourceManager
	toolManager     *ToolManager
	requestTimeout  time.Duration
	shutdownTimeout time.Duration
}

// NewServer creates a new MCP server.
func NewServer(config *config.Settings) (*Server, error) {
	if config == nil {
		return nil, cgerr.ErrorWithDetails(
			errors.New("config cannot be nil"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"config": "nil",
			},
		)
	}

	server := &Server{
		config:          config,
		version:         "0.1.0", // Default version
		transport:       "http",  // Default transport
		resourceManager: NewResourceManager(),
		toolManager:     NewToolManager(),
		requestTimeout:  30 * time.Second, // Default request timeout
		shutdownTimeout: 5 * time.Second,  // Default shutdown timeout
	}

	return server, nil
}

// SetVersion sets the server version.
func (s *Server) SetVersion(version string) {
	s.version = version
}

// SetTransport sets the transport type.
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

// SetRequestTimeout sets the request timeout.
func (s *Server) SetRequestTimeout(timeout time.Duration) {
	s.requestTimeout = timeout
}

// SetShutdownTimeout sets the shutdown timeout.
func (s *Server) SetShutdownTimeout(timeout time.Duration) {
	s.shutdownTimeout = timeout
}

// RegisterResourceProvider registers a resource provider.
func (s *Server) RegisterResourceProvider(provider ResourceProvider) {
	s.resourceManager.RegisterProvider(provider)
}

// RegisterToolProvider registers a tool provider.
func (s *Server) RegisterToolProvider(provider ToolProvider) {
	s.toolManager.RegisterProvider(provider)
}

// Start starts the MCP server.
func (s *Server) Start() error {
	switch s.transport {
	case "http":
		return s.startHTTP()
	case "stdio":
		return s.startStdio()
	default:
		return errors.Newf("unsupported transport: %s", s.transport)
	}
}

// startHTTP starts the MCP server with HTTP transport.
func (s *Server) startHTTP() error {
	// Create a JSON-RPC adapter
	adapter := jsonrpc.NewAdapter(jsonrpc.WithTimeout(s.requestTimeout))

	// Register handlers
	s.registerHandlers(adapter)

	// Create an HTTP handler
	httpHandler := jsonrpc.NewHTTPHandler(adapter, jsonrpc.WithHTTPRequestTimeout(s.requestTimeout))

	// Create an HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.GetServerAddress(),
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start the HTTP server
	fmt.Printf("Starting HTTP server on %s\n", s.config.GetServerAddress())
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to start HTTP server"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"address": s.config.GetServerAddress(),
			},
		)
	}

	return nil
}

// startStdio starts the MCP server with stdio transport.
func (s *Server) startStdio() error {
	// Create a JSON-RPC adapter
	adapter := jsonrpc.NewAdapter(jsonrpc.WithTimeout(s.requestTimeout))

	// Register handlers
	s.registerHandlers(adapter)

	// Set up stdio transport options
	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(s.requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second), // Increase to 2 minutes
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),
	}

	// Start the stdio server
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

	return nil
}

// Stop stops the MCP server.
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
				map[string]interface{}{
					"timeout": s.shutdownTimeout.String(),
				},
			)
		}
	}

	return nil
}

// registerHandlers registers all handlers with the adapter.
// This is used for the non-state-machine implementation.
func (s *Server) registerHandlers(adapter *jsonrpc.Adapter) {
	// Register initialize handler
	adapter.RegisterHandler("initialize", s.handleInitialize)

	// Register resource handlers
	adapter.RegisterHandler("list_resources", s.handleListResources)
	adapter.RegisterHandler("read_resource", s.handleReadResource)

	// Register tool handlers
	adapter.RegisterHandler("list_tools", s.handleListTools)
	adapter.RegisterHandler("call_tool", s.handleCallTool)
}

// cmd/server/server.go
// createAndConfigureServerWithStateMachine creates and configures an MCP server with state machine architecture.
func createAndConfigureServerWithStateMachine(cfg *config.Settings, transportType string, requestTimeout, shutdownTimeout time.Duration) (*mcp.ConnectionServer, error) {
	// Create the base server first
	baseServer, err := createAndConfigureServer(cfg, transportType, requestTimeout, shutdownTimeout)
	if err != nil {
		return nil, err
	}

	// Wrap it with the connection server
	connectionServer, err := mcp.NewConnectionServer(baseServer)
	if err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "createAndConfigureServerWithStateMachine: failed to create connection server"),
			cgerr.CategoryConfig,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"server_name": cfg.GetServerName(),
				"transport":   transportType,
			},
		)
	}

	return connectionServer, nil
}
