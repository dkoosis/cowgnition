// internal/mcp/server.go
package mcp

import (
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/connection"
)

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
		return errors.Newf("unsupported transport type: %s", transportType)
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
}

// RegisterToolProvider registers a tool provider.
func (s *Server) RegisterToolProvider(provider ToolProvider) {
	s.toolManager.RegisterProvider(provider)
}

// Config returns the server configuration.
func (s *Server) Config() Config {
	return s.config
}

// Start starts the server with the configured transport.
func (s *Server) Start() error {
	// Create a connection server that uses the state machine architecture
	connServer, err := connection.NewConnectionServer(s)
	if err != nil {
		return err
	}

	return connServer.Start()
}

// startHTTP starts the HTTP transport server (implementation for connection package to use).
func (s *Server) startHTTP() error {
	// Implementation for HTTP transport
	return errors.New("HTTP transport not yet implemented")
}

// Stop stops the server.
func (s *Server) Stop() error {
	// Implementation for stopping the server
	return nil
}
