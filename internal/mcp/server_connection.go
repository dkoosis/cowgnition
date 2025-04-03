// file: internal/mcp/server_connection.go
package mcp

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	"github.com/dkoosis/cowgnition/internal/mcp/connection"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// ConnectionServer extends the Server to use the ConnectionManager.
type ConnectionServer struct {
	*Server
	connectionManager *connection.ConnectionManager
}

// NewConnectionServer creates a new server with state machine architecture.
func NewConnectionServer(server *Server) (*ConnectionServer, error) {
	// Create server configuration from the existing server
	config := connection.ServerConfig{
		Name:            server.config.GetServerName(),
		Version:         server.version,
		RequestTimeout:  server.requestTimeout,
		ShutdownTimeout: server.shutdownTimeout,
		Capabilities: map[string]interface{}{
			"resources": map[string]interface{}{
				"list": true,
				"read": true,
			},
			"tools": map[string]interface{}{
				"list": true,
				"call": true,
			},
		},
	}

	// Create adapter for the resource and tool managers
	resourceManager := &resourceManagerAdapter{rm: server.resourceManager}
	toolManager := &toolManagerAdapter{tm: server.toolManager}

	// Create a new connection manager
	connectionManager := connection.NewConnectionManager(
		config,
		resourceManager,
		toolManager,
	)

	return &ConnectionServer{
		Server:            server,
		connectionManager: connectionManager,
	}, nil
}

// startStdio starts the MCP server using stdio transport with state machine architecture.
func (s *ConnectionServer) startStdio() error {
	// Create a JSON-RPC handler that delegates to the connection manager
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Handle the request using the connection manager
		s.connectionManager.Handle(ctx, conn, req)

		// The connection manager handles the response directly, so we return nil here
		return nil, nil
	})

	// Set up the stdio transport
	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(s.requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second), // Increase to 2 minutes
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),
		jsonrpc.WithStdioDebug(true), // Enable debug logging
	}

	// Start the stdio server
	if err := jsonrpc.RunStdioServer(handler, stdioOpts...); err != nil {
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

// Start starts the MCP server using the configured transport.
func (s *ConnectionServer) Start() error {
	switch s.transport {
	case "http":
		// For now, we'll use the existing HTTP implementation
		return s.Server.startHTTP()
	case "stdio":
		// Use our new state machine-based stdio implementation
		return s.startStdio()
	default:
		return errors.Newf("unsupported transport type: %s", s.transport)
	}
}

// resourceManagerAdapter adapts the Server's ResourceManager to the connection.ResourceManagerContract interface
type resourceManagerAdapter struct {
	rm ResourceManager
}

// GetAllResourceDefinitions implements the connection.ResourceManagerContract interface
func (a *resourceManagerAdapter) GetAllResourceDefinitions() []definitions.ResourceDefinition {
	// Return typed slice directly, not converting to interface{}
	return a.rm.GetAllResourceDefinitions()
}

// ReadResource implements the connection.ResourceManagerContract interface
func (a *resourceManagerAdapter) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	// Call through to the underlying ResourceManager
	// Already returns the expected string, string, error types
	return a.rm.ReadResource(ctx, name, args)
}

// toolManagerAdapter adapts the Server's ToolManager to the connection.ToolManagerContract interface
type toolManagerAdapter struct {
	tm ToolManager
}

// GetAllToolDefinitions implements the connection.ToolManagerContract interface
func (a *toolManagerAdapter) GetAllToolDefinitions() []definitions.ToolDefinition {
	// Return typed slice directly, not converting to interface{}
	return a.tm.GetAllToolDefinitions()
}

// CallTool implements the connection.ToolManagerContract interface
func (a *toolManagerAdapter) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	// Call through to the underlying ToolManager
	// Already returns string, error as required
	return a.tm.CallTool(ctx, name, args)
}
