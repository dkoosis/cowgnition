// Package mcp implements the Model Context Protocol (MCP) server.
// file: internal/mcp/server.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
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
	transport       string           // transport: The transport type (http or stdio).
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
		transport:       "http",               // Default transport type.
	}, nil
}

// SetTransport sets the transport type for the server.
// Valid values are "http" and "stdio".
func (s *Server) SetTransport(transport string) error {
	if transport != "http" && transport != "stdio" {
		return fmt.Errorf("Server.SetTransport: invalid transport type: %s. Valid values are 'http' and 'stdio'", transport)
	}
	s.transport = transport
	return nil
}

// Start starts the MCP server using the configured transport.
// This is the main entry point for starting the server, which will use
// either HTTP or stdio transport based on the configuration.
//
// Returns:
//
//	error: An error if the server fails to start.
func (s *Server) Start() error {
	switch s.transport {
	case "http":
		return s.startHTTP()
	case "stdio":
		return s.startStdio()
	default:
		return fmt.Errorf("Server.Start: unsupported transport type: %s", s.transport)
	}
}

// startHTTP starts the MCP server using HTTP transport.
// This function sets up the HTTP server, registers the MCP handlers,
// and begins listening for incoming requests.
//
// Returns:
//
//	error: An error if the server fails to start.
func (s *Server) startHTTP() error {
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
	log.Printf("Server.startHTTP: starting MCP server with HTTP transport on %s", s.httpServer.Addr) // Log the server start.
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Server.startHTTP: failed to start server: %w", err)
	}
	return nil // Start the server and listen for requests.
}

// startStdio starts the MCP server using stdio transport.
// This function sets up the stdio-based JSON-RPC server for MCP communication.
// It's particularly useful for process-based integration like with Claude Desktop.
//
// Returns:
//
//	error: An error if the server fails to start.
func (s *Server) startStdio() error {
	// Create a JSON-RPC adapter
	adapter := jsonrpc.NewAdapter()

	// Register the MCP handlers
	s.RegisterJSONRPCHandlers(adapter)

	// Start the stdio server
	log.Printf("Server.startStdio: starting MCP server with stdio transport")
	if err := jsonrpc.RunStdioServer(adapter); err != nil {
		return fmt.Errorf("Server.startStdio: failed to start stdio server: %w", err)
	}

	return nil
}

// Stop stops the MCP server.
// This function gracefully shuts down the server regardless of transport type.
//
// Returns:
//
//	error: An error if the server fails to stop.
func (s *Server) Stop() error {
	if s.transport == "http" && s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("Server.Stop: failed to stop HTTP server: %w", err)
		}
	}

	// For stdio, there's nothing special to stop as the process itself
	// will be terminated when the parent process stops.

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

// Add this to internal/mcp/server.go

// RegisterJSONRPCHandlers registers the MCP handlers with the JSON-RPC adapter.
func (s *Server) RegisterJSONRPCHandlers(adapter *jsonrpc.Adapter) {
	// Register initialize handler
	adapter.RegisterHandler("initialize", s.handleJSONRPCInitialize)

	// Register resource handlers
	adapter.RegisterHandler("list_resources", s.handleJSONRPCListResources)
	adapter.RegisterHandler("read_resource", s.handleJSONRPCReadResource)

	// Register tool handlers
	adapter.RegisterHandler("list_tools", s.handleJSONRPCListTools)
	adapter.RegisterHandler("call_tool", s.handleJSONRPCCallTool)
}

// handleJSONRPCInitialize handles the MCP initialize request via JSON-RPC.
func (s *Server) handleJSONRPCInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("failed to decode initialize request: %v", err))
	}

	// Log initialization request
	log.Printf("MCP initialization requested by: %s (version: %s)",
		req.ServerName, req.ServerVersion)

	// Construct server information
	serverInfo := ServerInfo{
		Name:    s.config.GetServerName(),
		Version: s.version,
	}

	// Define capabilities
	capabilities := map[string]interface{}{
		"resources": map[string]interface{}{
			"list": true,
			"read": true,
		},
		"tools": map[string]interface{}{
			"list": true,
			"call": true,
		},
	}

	// Construct response
	response := InitializeResponse{
		ServerInfo:   serverInfo,
		Capabilities: capabilities,
	}

	return response, nil
}

// handleJSONRPCListResources handles the JSON-RPC list_resources request.
func (s *Server) handleJSONRPCListResources(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// No parameters needed for listing resources

	// Get resources from all registered providers
	resources := s.resourceManager.GetAllResourceDefinitions()

	response := ListResourcesResponse{
		Resources: resources,
	}

	return response, nil
}

// handleJSONRPCReadResource handles the JSON-RPC read_resource request.
func (s *Server) handleJSONRPCReadResource(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// Parse request parameters
	var req struct {
		Name string            `json:"name"`
		Args map[string]string `json:"args,omitempty"`
	}

	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("failed to decode read_resource request: %v", err))
	}

	// Validate required parameters
	if req.Name == "" {
		return nil, jsonrpc.NewInvalidParamsError("missing required resource name parameter")
	}

	// Read the resource
	content, mimeType, err := s.resourceManager.ReadResource(ctx, req.Name, req.Args)
	if err != nil {
		if err == ErrResourceNotFound {
			return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("resource not found: %s", req.Name))
		} else if err == ErrInvalidArguments {
			return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("invalid arguments for resource: %s", req.Name))
		}
		return nil, jsonrpc.NewInternalError(fmt.Errorf("failed to read resource: %w", err))
	}

	// Return the resource content
	response := ResourceResponse{
		Content:  content,
		MimeType: mimeType,
	}

	return response, nil
}

// handleJSONRPCListTools handles the JSON-RPC list_tools request.
func (s *Server) handleJSONRPCListTools(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// No parameters needed for listing tools

	// Get tools from all registered providers
	tools := s.toolManager.GetAllToolDefinitions()

	response := ListToolsResponse{
		Tools: tools,
	}

	return response, nil
}

// handleJSONRPCCallTool handles the JSON-RPC call_tool request.
func (s *Server) handleJSONRPCCallTool(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// Parse request parameters
	var req CallToolRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("failed to decode call_tool request: %v", err))
	}

	// Validate required parameters
	if req.Name == "" {
		return nil, jsonrpc.NewInvalidParamsError("missing required tool name parameter")
	}

	// Call the tool
	result, err := s.toolManager.CallTool(ctx, req.Name, req.Arguments)
	if err != nil {
		if err == ErrToolNotFound {
			return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("tool not found: %s", req.Name))
		} else if err == ErrInvalidArguments {
			return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("invalid arguments for tool: %s", req.Name))
		}
		return nil, jsonrpc.NewInternalError(fmt.Errorf("failed to call tool: %w", err))
	}

	// Return the tool result
	response := ToolResponse{
		Result: result,
	}

	return response, nil
}
