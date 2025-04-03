// Package connection handles the state management and communication logic,
// including the server setup specific to the ConnectionManager.
package connection

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"

	// Import mcp for base Server type and base interfaces
	// IMPORTANT: Replace with the correct import path for your module
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// ConnectionServer wraps the base mcp.Server to use the ConnectionManager
// for handling connections, particularly stdio.
type ConnectionServer struct {
	// Embed the base server from the mcp package
	*mcp.Server
	// Use the ConnectionManager from this package
	connectionManager *Manager // Renamed from ConnectionManager
}

// NewConnectionServer creates a new server specialized for ConnectionManager usage.
// It takes the base mcp.Server which contains configurations and base managers.
func NewConnectionServer(baseServer *mcp.Server) (*ConnectionServer, error) {
	// Create server configuration specific to the connection manager
	// Uses types defined within this 'connection' package (ServerConfig)
	config := ServerConfig{ // Now uses ServerConfig from this package
		Name:            baseServer.Config().GetServerName(), // Assuming Config() method on mcp.Server
		Version:         baseServer.Version(),                // Assuming Version() method on mcp.Server
		RequestTimeout:  baseServer.RequestTimeout(),         // Assuming RequestTimeout() method
		ShutdownTimeout: baseServer.ShutdownTimeout(),        // Assuming ShutdownTimeout() method
		Capabilities: map[string]interface{}{
			"resources": map[string]interface{}{"list": true, "read": true},
			"tools":     map[string]interface{}{"list": true, "call": true},
		},
	}

	// Create adapters that satisfy the contracts defined in mcp package
	// using the base managers from the mcp.Server.
	// Note: Contracts are now defined in mcp.ResourceManagerContract, mcp.ToolManagerContract
	resourceManagerAdapter := &resourceManagerAdapter{rm: baseServer.ResourceManager()} // Assuming ResourceManager() method
	toolManagerAdapter := &toolManagerAdapter{tm: baseServer.ToolManager()}             // Assuming ToolManager() method

	// Create a new connection manager (defined in this package)
	// Pass the adapters which fulfill the required contracts.
	connectionManager := NewManager( // Uses NewManager from this package
		config,
		resourceManagerAdapter, // Implements mcp.ResourceManagerContract
		toolManagerAdapter,     // Implements mcp.ToolManagerContract
	)

	return &ConnectionServer{
		Server:            baseServer,
		connectionManager: connectionManager,
	}, nil
}

// startStdio starts the server using stdio transport, handled by ConnectionManager.
func (s *ConnectionServer) startStdio() error {
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Delegate directly to the ConnectionManager's handler (in this package)
		s.connectionManager.Handle(ctx, conn, req)
		// ConnectionManager handles replies internally via state machine actions.
		// Returning nil, nil indicates the request was accepted for processing.
		return nil, nil
	})

	// Use base server properties via embedded field access
	requestTimeout := s.RequestTimeout() // Assuming getter on mcp.Server

	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second),
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),
		jsonrpc.WithStdioDebug(true),
	}

	if err := jsonrpc.RunStdioServer(handler, stdioOpts...); err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to start stdio server"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"request_timeout": requestTimeout.String(),
				"read_timeout":    "120s",
				"write_timeout":   "30s",
			},
		)
	}
	return nil
}

// Start decides which transport to use based on configuration.
func (s *ConnectionServer) Start() error {
	transport := s.Transport() // Assuming Transport() getter on mcp.Server
	switch transport {
	case "http":
		// Fallback to the base server's HTTP implementation
		return s.Server.StartHTTP() // Assuming StartHTTP method on mcp.Server
	case "stdio":
		// Use the ConnectionManager-based stdio implementation
		return s.startStdio()
	default:
		return errors.Newf("unsupported transport type: %s", transport)
	}
}

// resourceManagerAdapter adapts the base mcp.ResourceManager to satisfy
// the mcp.ResourceManagerContract interface.
type resourceManagerAdapter struct {
	// Holds the base resource manager (type defined in mcp package)
	rm mcp.ResourceManager
}

// GetAllResourceDefinitions implements the mcp.ResourceManagerContract interface.
func (a *resourceManagerAdapter) GetAllResourceDefinitions() []definitions.ResourceDefinition {
	return a.rm.GetAllResourceDefinitions()
}

// ReadResource implements the mcp.ResourceManagerContract interface.
func (a *resourceManagerAdapter) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	return a.rm.ReadResource(ctx, name, args)
}

// toolManagerAdapter adapts the base mcp.ToolManager to satisfy
// the mcp.ToolManagerContract interface.
type toolManagerAdapter struct {
	// Holds the base tool manager (type defined in mcp package)
	tm mcp.ToolManager
}

// GetAllToolDefinitions implements the mcp.ToolManagerContract interface.
func (a *toolManagerAdapter) GetAllToolDefinitions() []definitions.ToolDefinition {
	return a.tm.GetAllToolDefinitions()
}

// CallTool implements the mcp.ToolManagerContract interface.
func (a *toolManagerAdapter) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return a.tm.CallTool(ctx, name, args)
}

// --- Helper methods assumed to exist on mcp.Server ---
// These are examples; adjust based on your actual mcp.Server structure

func (s *mcp.Server) Config() mcp.Config                   { return s.config }
func (s *mcp.Server) Version() string                      { return s.version }
func (s *mcp.Server) RequestTimeout() time.Duration        { return s.requestTimeout }
func (s *mcp.Server) ShutdownTimeout() time.Duration       { return s.shutdownTimeout }
func (s *mcp.Server) ResourceManager() mcp.ResourceManager { return s.resourceManager }
func (s *mcp.Server) ToolManager() mcp.ToolManager         { return s.toolManager }
func (s *mcp.Server) Transport() string                    { return s.transport }
func (s *mcp.Server) StartHTTP() error                     { return s.startHTTP() } // Assuming private startHTTP exists
