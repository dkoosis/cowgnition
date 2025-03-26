// Package mcp implements the Model Context Protocol (MCP) server.
// file: internal/mcp/server.go
package mcp

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
)

// Server represents an MCP server.
// It encapsulates the configuration, HTTP server, versioning, and resource/tool management.
type Server struct {
	config          *config.Settings // config: The server's configuration settings.
	httpServer      *http.Server     // httpServer: The underlying HTTP server.
	version         string           // version: The server's version.
	startTime       time.Time        // startTime: The server's start time, used for uptime calculations.
	resourceManager *ResourceManager // resourceManager: Manages resource providers and resources.
	toolManager     *ToolManager     // toolManager: Manages tool providers and tools.
}

// NewServer creates a new MCP server.
// This function initializes the server with its configuration, default version,
// start time, and resource/tool managers.
//
// cfg *config.Settings: The server configuration.
//
// Returns:
//
//	*Server: The new MCP server.
//	error:  An error, if any.
func NewServer(cfg *config.Settings) (*Server, error) {
	return &Server{
		config:          cfg,                  // Store the configuration.
		version:         "1.0.0",              // Default version.
		startTime:       time.Now(),           // Record the start time.
		resourceManager: NewResourceManager(), // Initialize the resource manager.
		toolManager:     NewToolManager(),     // Initialize the tool manager.
	}, nil
}

// Start starts the MCP server.
// This function sets up the HTTP server, registers the MCP handlers,
// and begins listening for incoming requests.
// It uses the server's configuration to determine the address to listen on.
//
// Returns:
//
//	error: An error if the server fails to start.
func (s *Server) Start() error {
	mux := http.NewServeMux() // Create a new HTTP multiplexer.

	// Register MCP protocol handlers
	mux.HandleFunc("/mcp/initialize", s.handleInitialize)        // Handler for MCP initialize requests.
	mux.HandleFunc("/mcp/list_resources", s.handleListResources) // Handler for listing resources.
	mux.HandleFunc("/mcp/read_resource", s.handleReadResource)   // Handler for reading a specific resource.
	mux.HandleFunc("/mcp/list_tools", s.handleListTools)         // Handler for listing tools.
	mux.HandleFunc("/mcp/call_tool", s.handleCallTool)           // Handler for calling a tool.

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.GetServerAddress(), // Use the configured server address.
		Handler:      mux,                         // Use the multiplexer.
		ReadTimeout:  15 * time.Second,            // Read timeout.
		WriteTimeout: 15 * time.Second,            // Write timeout.
		IdleTimeout:  60 * time.Second,            // Idle timeout.
	}

	// Start HTTP server
	log.Printf("Server.Start: starting MCP server on %s", s.httpServer.Addr) // Log the server start.
	if err := s.httpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("Server.Start: failed to start server: %w", err)
	}
	return nil // Start the server and listen for requests.
}

// Stop stops the MCP server.
// This function gracefully shuts down the HTTP server.
//
// Returns:
//
//	error: An error if the server fails to stop.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			return fmt.Errorf("Server.Stop: failed to stop server: %w", err)
		}
	}
	return nil
}

// SetVersion sets the server version.
// This function allows updating the server's version at runtime.
//
// version string: The new server version.
func (s *Server) SetVersion(version string) {
	s.version = version // Update the server version.
}

// GetUptime returns the server's uptime.
// This function calculates the duration since the server started.
//
// Returns:
//
//	time.Duration: The server's uptime.
func (s *Server) GetUptime() time.Duration {
	return time.Since(s.startTime) // Calculate and return the uptime.
}

// RegisterResourceProvider registers a resource provider.
// This function adds a ResourceProvider to the server's resource manager.
//
// provider ResourceProvider: The resource provider to register.
func (s *Server) RegisterResourceProvider(provider ResourceProvider) {
	s.resourceManager.RegisterProvider(provider) // Register the provider.
}

// RegisterToolProvider registers a tool provider.
// This function adds a ToolProvider to the server's tool manager.
//
// provider ToolProvider: The tool provider to register.
func (s *Server) RegisterToolProvider(provider ToolProvider) {
	s.toolManager.RegisterProvider(provider) // Register the provider.
}

// ErrorMsgEnhanced: 2025-03-26
