// file: internal/mcp/server_with_connection.go
package mcp

import (
	"context"
	"log"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	"github.com/dkoosis/cowgnition/internal/mcp/connection"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// ConnectionServer enhances the regular Server with state machine-based connection management.
type ConnectionServer struct {
	*Server
	connectionManager *connection.Manager
}

// NewConnectionServer creates a server with state machine architecture.
func NewConnectionServer(server *Server) (*ConnectionServer, error) {
	// Create server configuration for the connection manager
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

	// Create resource manager adapter
	resourceAdapter := &resourceManagerAdapter{
		rm: server.resourceManager,
	}

	// Create tool manager adapter
	toolAdapter := &toolManagerAdapter{
		tm: server.toolManager,
	}

	// Create connection manager
	connManager := connection.NewManager(config, resourceAdapter, toolAdapter)

	return &ConnectionServer{
		Server:            server,
		connectionManager: connManager,
	}, nil
}

// startStdio starts the MCP server using stdio transport with state machine architecture.
func (s *ConnectionServer) startStdio() error {
	log.Printf("Starting MCP server with state machine architecture on stdio transport")

	// Create a JSON-RPC handler that delegates to the connection manager
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Delegate to the connection manager's Handle method
		s.connectionManager.Handle(ctx, conn, req)

		// This handler doesn't return a response directly - responses are sent by the connection manager
		return nil, nil
	})

	// Set up stdio transport options
	stdioOpts := []jsonrpc.StdioTransportOption{
		jsonrpc.WithStdioRequestTimeout(s.requestTimeout),
		jsonrpc.WithStdioReadTimeout(120 * time.Second),
		jsonrpc.WithStdioWriteTimeout(30 * time.Second),
		jsonrpc.WithStdioDebug(true), // Enable debug logging for transport
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
		// For now, use the existing HTTP implementation
		// In a future update, we can implement HTTP with state machine
		return s.Server.startHTTP()
	case "stdio":
		// Use our state machine-based stdio implementation
		return s.startStdio()
	default:
		return errors.Newf("unsupported transport type: %s", s.transport)
	}
}

// Stop stops the server.
func (s *ConnectionServer) Stop() error {
	// Additional cleanup might be needed here
	return s.Server.Stop()
}
