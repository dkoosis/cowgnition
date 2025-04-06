// Package jsonrpc provides implementations for JSON-RPC 2.0 communication,
// including transport mechanisms like stdio and HTTP.
package jsonrpc

import (
	"context"
	"fmt"  // Used for formatting log messages and errors.
	"os"   // Provides access to standard input (Stdin) and standard output (Stdout).
	"time" // Used for timeouts and heartbeat ticker.

	"github.com/cockroachdb/errors"                           // Error handling library.
	"github.com/dkoosis/cowgnition/internal/logging"          // Project's structured logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // Project's custom error types.
	"github.com/sourcegraph/jsonrpc2"                         // Core JSON-RPC 2.0 library.
)

// stdioTransportLogger initializes the structured logger for the jsonrpc stdio transport layer.
var stdioTransportLogger = logging.GetLogger("jsonrpc_stdio_transport")

// stdioPipe implements the io.ReadWriteCloser interface using the process's
// standard input (os.Stdin) and standard output (os.Stdout).
// This allows jsonrpc2 to treat stdin/stdout as a communication channel.
type stdioPipe struct{}

// Read reads data from standard input (os.Stdin).
// Note: Reading from stdin is typically blocking.
func (stdioPipe) Read(p []byte) (n int, err error) {
	n, err = os.Stdin.Read(p)
	// No extra logging here; errors are handled by the caller (jsonrpc2).
	return
}

// Write writes data to standard output (os.Stdout).
// Note: Concurrent writes to os.Stdout are generally safe.
func (stdioPipe) Write(p []byte) (n int, err error) {
	n, err = os.Stdout.Write(p)
	if err != nil {
		// Log errors during stdout write attempt, although they might be rare.
		stdioTransportLogger.Error("stdioPipe failed to write to os.Stdout.", "error", fmt.Sprintf("%+v", err))
	}
	return
}

// Close implements the io.Closer interface for stdioPipe.
// It's a no-op because closing the actual os.Stdin or os.Stdout is generally
// not desired or meaningful for a transport layer using them. The underlying
// jsonrpc2 connection handles shutdown signaling.
func (stdioPipe) Close() error {
	stdioTransportLogger.Debug("stdioPipe Close called (no-op for stdin/stdout).")
	return nil
}

// StdioTransport manages a JSON-RPC 2.0 connection over standard input/output.
// It uses jsonrpc2.NewPlainObjectStream, which expects newline-delimited JSON messages.
// NOTE: this means NO content-length headers, unlike other transports.
type StdioTransport struct {
	conn   *jsonrpc2.Conn // The underlying jsonrpc2 connection instance.
	closed bool           // Flag indicating if the transport's Close method has been called.
	// debug flag is deprecated in favor of checking the global logging level (e.g., logging.IsDebugEnabled).
	// It was previously used for conditional logic like heartbeats.
	debug bool
}

// NewStdioTransport creates a new, unconnected StdioTransport instance.
func NewStdioTransport() *StdioTransport {
	stdioTransportLogger.Debug("Creating new StdioTransport instance.")
	// The 'debug' field might be set via WithDebug for backward compatibility,
	// but effective debug state should be checked via the logging framework.
	return &StdioTransport{}
}

// WithDebug sets the debug flag on the transport instance.
// DEPRECATED: Debug behavior should be controlled by the global structured logging level.
// This method is retained temporarily for compatibility but may be removed in the future.
// Use the logging configuration to enable debug messages instead.
func (t *StdioTransport) WithDebug(debug bool) *StdioTransport {
	// This flag might be checked internally, but logging checks are preferred.
	t.debug = debug
	stdioTransportLogger.Warn("StdioTransport.WithDebug is deprecated; use logging levels.")
	return t
}

// Connect establishes the JSON-RPC connection using the stdioPipe and the provided handler.
// It initializes the underlying jsonrpc2.Conn using a PlainObjectStream, suitable for
// newline-delimited JSON over stdin/stdout.
// The provided context is used by jsonrpc2.NewConn for its internal setup and handling.
func (t *StdioTransport) Connect(ctx context.Context, handler jsonrpc2.Handler) (*jsonrpc2.Conn, error) {
	stdioTransportLogger.Debug("Connecting StdioTransport.")
	// Create the stream wrapper using our stdioPipe implementation.
	// NewPlainObjectStream handles newline-delimited JSON suitable for stdio.
	stream := jsonrpc2.NewPlainObjectStream(stdioPipe{})
	stdioTransportLogger.Debug("Created PlainObjectStream over stdioPipe.")

	// Create the actual jsonrpc2 connection.
	// The context passed here allows jsonrpc2 to handle cancellation signals.
	conn := jsonrpc2.NewConn(ctx, stream, handler)
	t.conn = conn    // Store the connection instance.
	t.closed = false // Mark as not closed upon successful connection.

	// Log connection success, checking effective debug level from the logging framework.
	if logging.IsDebugEnabled() {
		stdioTransportLogger.Debug("Stdio transport connected successfully (using PlainObjectStream).")
	}

	return conn, nil
}

// Close terminates the underlying jsonrpc2 connection if it's active and marks the
// transport as closed. It's safe to call multiple times.
// Returns an error if closing the jsonrpc2 connection fails.
func (t *StdioTransport) Close() error {
	if t.closed {
		stdioTransportLogger.Debug("StdioTransport Close called, but already marked as closed.")
		return nil
	}

	// Check if the connection was successfully established before trying to close.
	if t.conn != nil {
		// Mark as closed immediately to prevent race conditions.
		t.closed = true
		stdioTransportLogger.Debug("Closing underlying stdio jsonrpc2 connection.")
		// Attempt to close the jsonrpc2 connection.
		err := t.conn.Close()
		if err != nil {
			stdioTransportLogger.Error("Error closing stdio jsonrpc2 connection.", "error", fmt.Sprintf("%+v", err))
			// Wrap the error from jsonrpc2.Conn.Close for context.
			return errors.Wrap(err, "StdioTransport.Close: failed to close underlying jsonrpc2 connection")
		}
		stdioTransportLogger.Debug("Stdio transport connection closed successfully.")
		return nil
	}

	// If Close is called before Connect or after a failed Connect, conn might be nil.
	stdioTransportLogger.Debug("StdioTransport Close called, but connection was not established (conn is nil).")
	t.closed = true // Mark as closed even if conn was nil.
	return nil
}

// StdioTransportOption defines a function type for configuring stdioTransportOptions
// using the functional options pattern.
type StdioTransportOption func(*stdioTransportOptions)

// stdioTransportOptions holds configuration values for the StdioTransport,
// primarily used during the initialization phase in RunStdioServer.
type stdioTransportOptions struct {
	// requestTimeout specifies a requested timeout for operations.
	// Note: This might not be directly enforced by the stdio transport layer itself,
	// unlike HTTP timeouts. Timeout logic might need implementation within the handler.
	requestTimeout time.Duration
	// readTimeout specifies a requested read timeout.
	// Note: Likely not directly used by jsonrpc2 with stdioPipe.
	readTimeout time.Duration
	// writeTimeout specifies a requested write timeout.
	// Note: Likely not directly used by jsonrpc2 with stdioPipe.
	writeTimeout time.Duration
	// debug flag, set by options. DEPRECATED: Use logging levels.
	debug bool
}

// WithStdioRequestTimeout returns an option to set the desired request timeout.
// WARNING: The stdio transport based on jsonrpc2's PlainObjectStream might not
// directly support or enforce this timeout. It's stored for configuration logging
// but may require handler-level implementation for actual enforcement.
func WithStdioRequestTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.requestTimeout = timeout
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio request timeout value (non-positive).", "invalid_timeout", timeout)
		}
	}
}

// WithStdioReadTimeout returns an option to set the desired read timeout.
// WARNING: This timeout is likely NOT directly enforced by the underlying stdio transport.
// It's stored for configuration logging only.
func WithStdioReadTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.readTimeout = timeout
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio read timeout value (non-positive).", "invalid_timeout", timeout)
		}
	}
}

// WithStdioWriteTimeout returns an option to set the desired write timeout.
// WARNING: This timeout is likely NOT directly enforced by the underlying stdio transport.
// It's stored for configuration logging only.
func WithStdioWriteTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.writeTimeout = timeout
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio write timeout value (non-positive).", "invalid_timeout", timeout)
		}
	}
}

// WithStdioDebug returns an option to set the debug flag.
// DEPRECATED: Use logging configuration to control debug output globally.
// This option is retained temporarily for compatibility.
func WithStdioDebug(debug bool) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.debug = debug // Store option value.
		stdioTransportLogger.Warn("WithStdioDebug option is deprecated; use logging levels.")
	}
}

// RunStdioServer sets up and runs a JSON-RPC 2.0 server that communicates over
// standard input and standard output using newline-delimited JSON messages.
// It takes the core request handler and optional configuration settings.
// This function blocks until the JSON-RPC connection is terminated (e.g., by the client
// closing stdin, or an internal error causing jsonrpc2 to disconnect).
// It returns nil on normal disconnection, or an error if setup fails.
func RunStdioServer(handler jsonrpc2.Handler, options ...StdioTransportOption) error {
	// Initialize default options.
	opts := stdioTransportOptions{
		// Note: These defaults are stored but might not be enforced by the transport itself.
		requestTimeout: 30 * time.Second,
		readTimeout:    120 * time.Second,
		writeTimeout:   30 * time.Second,
		debug:          false, // Default debug flag state (deprecated).
	}

	// Apply provided options to override defaults.
	for _, option := range options {
		option(&opts)
	}

	// Determine the effective debug state based on the logging framework, not the deprecated option.
	effectiveDebug := logging.IsDebugEnabled()

	// Log server startup with configured options and effective debug state.
	stdioTransportLogger.Info("Starting stdio JSON-RPC server.",
		"request_timeout_config", opts.requestTimeout.String(), // Log configured values.
		"read_timeout_config", opts.readTimeout.String(),
		"write_timeout_config", opts.writeTimeout.String(),
		"debug_via_options", opts.debug, // Log the deprecated option value passed.
		"debug_via_slog", effectiveDebug, // Log the actual debug state used.
	)

	// Create the transport instance. The debug flag passed here is mostly informational now.
	transport := NewStdioTransport().WithDebug(effectiveDebug)
	// Ensure the transport connection is closed when RunStdioServer exits.
	defer func() {
		stdioTransportLogger.Debug("Closing stdio transport in RunStdioServer defer function.")
		if err := transport.Close(); err != nil {
			// Log error during deferred close. Use %+v for detailed stack trace if available.
			stdioTransportLogger.Error("Error closing stdio transport during deferred cleanup.", "error", fmt.Sprintf("%+v", err))
		} else {
			stdioTransportLogger.Debug("Stdio transport closed successfully during defer.")
		}
	}()

	// Create a background context for the server's lifetime.
	// jsonrpc2 uses this context for managing the connection.
	rootCtx := context.Background()

	// Establish the connection using the transport.
	conn, err := transport.Connect(rootCtx, handler)
	if err != nil {
		// Wrap the connection error with context.
		wrappedErr := errors.Wrap(err, "RunStdioServer: failed to connect stdio transport")
		// Return a structured error including configuration details.
		return cgerr.ErrorWithDetails(
			wrappedErr,
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"request_timeout_config": opts.requestTimeout.String(),
				"read_timeout_config":    opts.readTimeout.String(),
				"write_timeout_config":   opts.writeTimeout.String(),
			},
		)
	}

	// Track connection state and start time for logging.
	connState := "connected"
	startTime := time.Now()

	// Get the channel that signals when the jsonrpc2 connection disconnects.
	disconnectChan := conn.DisconnectNotify()

	stdioTransportLogger.Info("Stdio transport connected, server running. Waiting for disconnect signal.", "state", connState)

	// Set up a heartbeat ticker only if debug logging is enabled.
	var ticker *time.Ticker
	// Use a label for clarity when initializing the dummy ticker.
	dummyTickerChan := make(chan time.Time) // Channel that never receives.
	if effectiveDebug {
		ticker = time.NewTicker(30 * time.Second) // Send a tick every 30 seconds.
		defer ticker.Stop()                       // Ensure resources are released.
		stdioTransportLogger.Debug("Debug heartbeat ticker started.", "interval", "30s")
	} else {
		// If debug is off, use a dummy ticker that never fires.
		ticker = &time.Ticker{C: dummyTickerChan}
	}

	// Main loop: wait for disconnection or process heartbeat ticks (if enabled).
	for {
		select {
		case <-disconnectChan:
			// The underlying jsonrpc2 connection has closed.
			connState = "disconnected"
			duration := time.Since(startTime)
			stdioTransportLogger.Info("Stdio transport disconnected. Server exiting.",
				"state", connState,
				"total_duration", duration.String(),
			)
			// Return nil for normal shutdown/disconnection.
			return nil

		case <-ticker.C:
			// This case only runs if the real ticker was started (effectiveDebug is true).
			stdioTransportLogger.Debug("Stdio connection heartbeat.",
				"state", connState,
				"active_duration", time.Since(startTime).String(),
			)
			// Can add additional health checks within the heartbeat if needed.
		}
	}
}
