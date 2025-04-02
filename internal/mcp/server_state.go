// internal/mcp/server_state.go
package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/jsonrpc"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// ServerWithStateMachine extends the Server to use the ConnectionManager.
type ServerWithStateMachine struct {
	*Server
	connectionManager *ConnectionManager
}

// NewServerWithStateMachine creates a new server with state machine architecture.
func NewServerWithStateMachine(server *Server) *ServerWithStateMachine {
	// Create server configuration from the existing server
	config := ServerConfig{
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

	// Create a new connection manager
	connectionManager := NewConnectionManager(
		config,
		server.resourceManager,
		server.toolManager,
	)

	return &ServerWithStateMachine{
		Server:            server,
		connectionManager: connectionManager,
	}
}

// startStdio starts the MCP server using stdio transport with state machine architecture.
func (s *ServerWithStateMachine) startStdio() error {
	// Create a JSON-RPC adapter using the connection manager as the handler
	adapter := jsonrpc.NewAdapter(jsonrpc.WithTimeout(s.requestTimeout))

	// Create a wrapper handler that delegates to the connection manager
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Delegate to the connection manager
		resultCh := make(chan struct {
			result interface{}
			err    error
		}, 1)

		go func() {
			// Execute in a goroutine to allow for timeout handling
			s.connectionManager.Handle(ctx, conn, req)

			// Since Handle doesn't return the result directly, we'll send nil to the channel
			// to indicate completion
			resultCh <- struct {
				result interface{}
				err    error
			}{nil, nil}
		}()

		// Wait for result or timeout
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil, cgerr.NewTimeoutError(
					"Request processing timed out",
					map[string]interface{}{
						"method":      req.Method,
						"timeout_sec": s.requestTimeout.Seconds(),
					},
				)
			}
			return nil, cgerr.ErrorWithDetails(
				errors.Wrap(ctx.Err(), "context error"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"method": req.Method,
				},
			)
		case <-resultCh:
			// Connection manager has processed the request
			// It will have sent the response directly
			return nil, nil
		}
	})

	// Register the handler with the adapter
	adapter.RegisterHandler("*", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		// We won't use this handler directly as we're using the connection manager
		return nil, errors.New("unexpected direct handler call")
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
func (s *ServerWithStateMachine) Start() error {
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
