// file: internal/jsonrpc/stdio_transport.go
package jsonrpc

import (
	"context"
	"fmt" // Import fmt

	// Import slog.
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// Initialize the logger at the package level.
var stdioTransportLogger = logging.GetLogger("jsonrpc_stdio_transport")

// stdioPipe implements io.ReadWriteCloser using standard input/output.
type stdioPipe struct{}

// Read reads from stdin.
func (stdioPipe) Read(p []byte) (n int, err error) {
	// Reading from stdin is blocking, consider potential issues in concurrent scenarios
	n, err = os.Stdin.Read(p)
	return
}

// Write writes to stdout.
func (stdioPipe) Write(p []byte) (n int, err error) {
	// Writing to stdout should be safe concurrently if needed
	n, err = os.Stdout.Write(p)
	if err != nil {
		// Log errors writing to stdout? Less critical usually.
		stdioTransportLogger.Error("stdioPipe Write error", "error", fmt.Sprintf("%+v", err))
	}
	return
}

// Close is a no-op for stdin/stdout.
func (stdioPipe) Close() error {
	stdioTransportLogger.Debug("stdioPipe Close called (no-op for stdin/stdout)")
	// We don't actually close stdin/stdout as that would terminate the process
	return nil
}

// StdioTransport implements a stdio-based transport for JSON-RPC over MCP.
// ... (comments remain the same).
type StdioTransport struct {
	conn   *jsonrpc2.Conn
	closed bool
	debug  bool // Consider removing if debug logging is handled by slog levels
}

// NewStdioTransport creates a new StdioTransport.
func NewStdioTransport() *StdioTransport {
	stdioTransportLogger.Debug("Creating new StdioTransport")
	return &StdioTransport{
		// debug field will be set by WithDebug based on logging level later
	}
}

// WithDebug enables debug logging for the transport.
// DEPRECATED: Use slog levels instead. Retained for compatibility with RunStdioServer options for now.
func (t *StdioTransport) WithDebug(debug bool) *StdioTransport {
	// This might be overridden by global log level checks later
	t.debug = debug
	// logger.Debug("StdioTransport debug flag set", "debug", debug)
	return t
}

// Connect initializes the transport with a handler.
// ... (comments remain the same).
func (t *StdioTransport) Connect(ctx context.Context, handler jsonrpc2.Handler) (*jsonrpc2.Conn, error) {
	stdioTransportLogger.Debug("Connecting StdioTransport")
	// Use NewPlainObjectStream for newline-delimited JSON over stdio
	stream := jsonrpc2.NewPlainObjectStream(stdioPipe{})
	stdioTransportLogger.Debug("Created PlainObjectStream over stdioPipe")

	// Create connection with handler
	// Pass the root context; jsonrpc2 handles cancellation.
	conn := jsonrpc2.NewConn(ctx, stream, handler)
	t.conn = conn

	// Use slog levels for debug logging, check global level if possible
	// Replace log.Printf (Conceptual L79)
	if logging.IsDebugEnabled() { // Check if debug is enabled via logging config
		stdioTransportLogger.Debug("Stdio transport connected successfully (using PlainObjectStream)")
	}

	return conn, nil
}

// Close terminates the transport connection.
// ... (comments remain the same).
func (t *StdioTransport) Close() error {
	if t.closed {
		stdioTransportLogger.Debug("StdioTransport Close called, but already closed.")
		return nil
	}

	if t.conn != nil {
		t.closed = true
		// Replace log.Printf (Conceptual L107)
		stdioTransportLogger.Debug("Closing stdio transport connection")
		err := t.conn.Close()
		if err != nil {
			stdioTransportLogger.Error("Error closing jsonrpc2 connection", "error", fmt.Sprintf("%+v", err))
			// Return the close error
			return errors.Wrap(err, "StdioTransport.Close: failed to close underlying jsonrpc2 connection")
		}
		stdioTransportLogger.Debug("Stdio transport connection closed successfully.")
		return nil
	}
	stdioTransportLogger.Debug("StdioTransport Close called, but connection was nil.")
	return nil
}

// StdioTransportOption defines an option for StdioTransport configuration.
// ... (comments remain the same).
type StdioTransportOption func(*stdioTransportOptions)

// stdioTransportOptions holds configuration for StdioTransport.
type stdioTransportOptions struct {
	requestTimeout time.Duration
	readTimeout    time.Duration // Currently unused by jsonrpc2 stdio?
	writeTimeout   time.Duration // Currently unused by jsonrpc2 stdio?
	debug          bool          // To be replaced by slog level check
}

// WithStdioRequestTimeout sets the request timeout for stdio transport.
// Note: jsonrpc2 stdio might not directly support request timeouts like HTTP.
// This might need to be handled within the jsonrpc2.Handler implementation.
func WithStdioRequestTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.requestTimeout = timeout
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio request timeout value", "invalid_timeout", timeout)
		}
	}
}

// WithStdioReadTimeout sets the read timeout (potentially unused).
func WithStdioReadTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.readTimeout = timeout
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio read timeout value", "invalid_timeout", timeout)
		}
	}
}

// WithStdioWriteTimeout sets the write timeout (potentially unused).
func WithStdioWriteTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.writeTimeout = timeout
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio write timeout value", "invalid_timeout", timeout)
		}
	}
}

// WithStdioDebug enables debug logging (use slog instead).
func WithStdioDebug(debug bool) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.debug = debug // Store temporarily, but rely on slog IsDebugEnabled()
	}
}

// RunStdioServer runs a JSON-RPC server with stdio transport.
// ... (comments remain the same).
func RunStdioServer(handler jsonrpc2.Handler, options ...StdioTransportOption) error {
	opts := stdioTransportOptions{
		requestTimeout: 30 * time.Second, // Default, but maybe not directly applied by jsonrpc2 stdio
		readTimeout:    120 * time.Second,
		writeTimeout:   30 * time.Second,
		debug:          false, // Will check slog level instead
	}

	for _, option := range options {
		option(&opts)
	}

	// Replace log.Printf (Conceptual L139+).
	stdioTransportLogger.Info("Starting stdio JSON-RPC server",
		"request_timeout_config", opts.requestTimeout, // Log configured value, even if not directly used
		"read_timeout_config", opts.readTimeout,
		"write_timeout_config", opts.writeTimeout,
		"debug_via_options", opts.debug, // Log the option value passed
		"debug_via_slog", logging.IsDebugEnabled(), // Log effective debug state
	)

	// Use effective debug state from logging framework
	effectiveDebug := logging.IsDebugEnabled()

	// Create transport, passing effective debug state (though likely unused now).
	transport := NewStdioTransport().WithDebug(effectiveDebug)
	defer func() {
		stdioTransportLogger.Debug("Closing stdio transport in defer function")
		if err := transport.Close(); err != nil {
			// Replace log.Printf error log
			// Use %+v for detailed error including stack trace if available
			stdioTransportLogger.Error("Error closing stdio transport", "error", fmt.Sprintf("%+v", err))
		} else {
			stdioTransportLogger.Debug("Stdio transport closed successfully in defer")
		}
	}()

	// Create a root context that lives for the server's duration
	rootCtx := context.Background()

	// Connect the transport using the root context
	conn, err := transport.Connect(rootCtx, handler)
	if err != nil {
		// Add function context to wrap message
		wrappedErr := errors.Wrap(err, "RunStdioServer: failed to connect stdio transport")
		// Return detailed error (existing structure is good)
		return cgerr.ErrorWithDetails(
			wrappedErr, // Use the wrapped error with context
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"request_timeout_config": opts.requestTimeout.String(),
				"read_timeout_config":    opts.readTimeout.String(),
				"write_timeout_config":   opts.writeTimeout.String(),
			},
		)
	}

	// State tracking - aligns with state machine architecture
	connState := "connected"
	startTime := time.Now() // Track start time for duration logging

	// Get the disconnect notification channel from the connection
	disconnectChan := conn.DisconnectNotify()

	// Replace log.Printf (Conceptual L...)
	stdioTransportLogger.Info("Stdio transport connected, waiting for disconnect signal", "state", connState)

	// Create a heartbeat ticker if debug is enabled
	var ticker *time.Ticker
	if effectiveDebug {
		ticker = time.NewTicker(30 * time.Second) // Heartbeat interval
		defer ticker.Stop()                       // Ensure ticker is stopped
		stdioTransportLogger.Debug("Heartbeat ticker started", "interval", 30*time.Second)
	} else {
		// Create a dummy ticker channel that never fires if debug is off
		ticker = &time.Ticker{C: make(chan time.Time)} // Assign dummy ticker
	}

	// Block and wait for disconnect or handle heartbeats
	for {
		select {
		case <-disconnectChan:
			connState = "disconnected"
			duration := time.Since(startTime)
			// Replace log.Printf (Conceptual L...)
			stdioTransportLogger.Info("Stdio transport disconnected",
				"state", connState,
				"total_duration", duration,
			)
			return nil // Normal exit when connection closes

		case <-ticker.C: // Only receives if ticker was started (debug enabled)
			// Replace log.Printf heartbeat (Conceptual L...)
			stdioTransportLogger.Debug("Stdio connection heartbeat check",
				"state", connState,
				"active_duration", time.Since(startTime),
			)
			// Could add more health checks here if needed
		}
	}
}
