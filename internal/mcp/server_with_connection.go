// file: internal/mcp/server_with_connection.go
package mcp

import (
	"context"
	"fmt" // Import fmt
	// Import slog
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper
	"github.com/dkoosis/cowgnition/internal/mcp/connection"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// Initialize the logger at the package level
var logger = logging.GetLogger("mcp_server_with_connection")

// ConnectionServer enhances the regular Server with state machine-based connection management.
type ConnectionServer struct {
	*Server           // Embed the base server
	connectionManager *connection.Manager
}

// NewConnectionServer creates a server with state machine architecture.
func NewConnectionServer(server *Server) (*ConnectionServer, error) {
	logger.Debug("Creating new connection server")
	// Create server configuration for the connection manager
	config := connection.ServerConfig{
		Name:            server.config.GetServerName(),
		Version:         server.version,
		RequestTimeout:  server.requestTimeout,
		ShutdownTimeout: server.shutdownTimeout,
		// Define capabilities based on registered providers or configuration
		Capabilities: map[string]interface{}{
			"resources": map[string]interface{}{
				"list": true, // Assuming basic capabilities exist
				"read": true,
			},
			"tools": map[string]interface{}{
				"list": true,
				"call": true,
			},
			// TODO: Populate capabilities more dynamically if needed
		},
	}
	logger.Debug("Connection manager config created", "config", fmt.Sprintf("%+v", config))

	// Create resource manager adapter
	resourceAdapter := &resourceManagerAdapter{
		rm: server.resourceManager,
	}
	logger.Debug("Resource manager adapter created")

	// Create tool manager adapter
	toolAdapter := &toolManagerAdapter{
		tm: server.toolManager,
	}
	logger.Debug("Tool manager adapter created")

	// Create connection manager
	connManager := connection.NewManager(config, resourceAdapter, toolAdapter)
	logger.Info("Connection manager created successfully")

	return &ConnectionServer{
		Server:            server,
		connectionManager: connManager,
	}, nil
}

// startStdio starts the MCP server using stdio transport with state machine architecture.
func (s *ConnectionServer) startStdio() error {
	// Replace log.Printf with slog.Info
	logger.Info("Starting MCP connection server with state machine architecture on stdio transport")

	// Create a JSON-RPC handler that delegates to the connection manager
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Delegate to the connection manager's Handle method
		// Handle is asynchronous, so we don't expect a direct result/error here for the JSON-RPC response itself.
		// The connection manager sends responses/errors via the conn object.
		go s.connectionManager.Handle(ctx, conn, req) // Run handler logic in a goroutine? Check Handle impl.

		// According to jsonrpc2.HandlerWithError docs, returning (nil, nil) indicates
		// the request was accepted for asynchronous processing.
		return nil, nil
	})
	logger.Debug("JSON-RPC stdio handler configured to delegate to connection manager")

	// Set up stdio transport options
	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(s.requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second),  // Consider config
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),  // Consider config
		jsonrpc.WithStdioDebug(logging.IsDebugEnabled()), // Enable transport debug based on logging level
	}
	logger.Debug("Stdio transport options configured",
		"request_timeout", s.requestTimeout,
		"read_timeout", "120s",
		"write_timeout", "30s",
		"debug_logging", logging.IsDebugEnabled(),
	)

	// Start the stdio server
	if err := jsonrpc.RunStdioServer(handler, stdioOpts...); err != nil {
		// Add logging before returning the error
		logger.Error("Stdio server run failed", "error", fmt.Sprintf("%+v", err))

		// Apply context change from assessment example to the Wrap message
		wrappedErr := errors.Wrap(err, "ConnectionServer.startStdio: failed to start stdio server") // Added function context

		// Return the detailed cgerr (existing code structure was already good)
		return cgerr.ErrorWithDetails(
			wrappedErr, // Pass the wrapped error with context
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"request_timeout": s.requestTimeout.String(),
				"read_timeout":    "120s", // Keep consistent with options
				"write_timeout":   "30s",  // Keep consistent with options
			},
		)
	}

	logger.Info("Stdio transport finished.") // Log normal exit
	return nil
}

// Start starts the MCP server using the configured transport.
// Overrides the embedded Server's Start method.
func (s *ConnectionServer) Start() error {
	// Use the logger defined in this file
	logger.Info("Starting MCP Connection Server", "version", s.version, "transport", s.transport)
	switch s.transport {
	case "http":
		// For now, use the existing HTTP implementation from embedded Server
		// Log that we're falling back
		logger.Warn("HTTP transport selected, falling back to base server implementation (no connection state machine)")
		return s.Server.startHTTP() // Call embedded Server's method
	case "stdio":
		// Use our state machine-based stdio implementation in this file
		return s.startStdio()
	default:
		err := errors.Newf("unsupported transport type: %s", s.transport)
		logger.Error("Cannot start server, unsupported transport", "transport", s.transport, "error", fmt.Sprintf("%+v", err))
		return err
	}
}

// Stop stops the server.
// Overrides the embedded Server's Stop method.
func (s *ConnectionServer) Stop() error {
	logger.Info("Stopping MCP Connection Server...")
	// TODO: Add specific cleanup for connectionManager if needed
	// e.g., s.connectionManager.Shutdown() ?

	// Call the embedded server's Stop method for its cleanup
	err := s.Server.Stop()
	if err != nil {
		logger.Error("Error during base server stop", "error", fmt.Sprintf("%+v", err))
		// Decide whether to return this error or just log it
	}
	logger.Info("MCP Connection Server stopped.")
	return err // Return error from base server stop for now
}
