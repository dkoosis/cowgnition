// Package jsonrpc provides implementations for JSON-RPC 2.0 communication,
// including transport mechanisms like stdio and HTTP.
package jsonrpc

import (
	"context"
	"fmt"
	"io" // Needed for io.EOF check.
	"os"
	"time"

	"github.com/cockroachdb/errors" // Using cockroachdb/errors for potential wrapping.
	"github.com/dkoosis/cowgnition/internal/logging"

	// Assuming cgerr might be needed for RunStdioServer error return type.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// stdioTransportLogger initializes the structured logger for this package.
var stdioTransportLogger = logging.GetLogger("jsonrpc_stdio_transport")

// stdioPipe implements the io.ReadWriteCloser interface using os.Stdin/os.Stdout.
// It acts as a raw pipe for the jsonrpc2 library, passing bytes through.
type stdioPipe struct{}

// Read reads data from standard input (os.Stdin).
// jsonrpc2.NewPlainObjectStream expects newline-delimited JSON messages from stdin.
func (s stdioPipe) Read(p []byte) (n int, err error) {
	n, err = os.Stdin.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		// Log actual errors, EOF is expected on graceful close.
		stdioTransportLogger.Error("stdioPipe Read error.", "error", fmt.Sprintf("%+v", err))
	} else if n > 0 && logging.IsDebugEnabled() {
		// Avoid logging full buffer 'p' as it might contain old data past 'n' bytes.
		// Log only the bytes actually read in this call.
		stdioTransportLogger.Debug("stdioPipe Read data from stdin.", "bytes_read", n, "data_sample", string(p[:min(n, 100)]))
	}
	return n, err
}

// Write writes data directly to standard output (os.Stdout).
// It logs the raw bytes being sent *by* the jsonrpc2 library before writing.
func (s stdioPipe) Write(p []byte) (n int, err error) {
	// Log the raw bytes being sent *before* writing for debugging purposes.
	if logging.IsDebugEnabled() {
		// This logs the exact data received from the jsonrpc2 connection layer to be written.
		stdioTransportLogger.Debug("stdioPipe Writing raw message bytes to stdout.",
			"raw_data", string(p), // Assumes UTF-8 data for logging, which JSON is.
			"byte_count", len(p))
	}

	// Directly write the byte slice 'p' as provided by the jsonrpc2 library.
	n, err = os.Stdout.Write(p)
	if err != nil {
		// Log errors encountered during the write operation.
		stdioTransportLogger.Error("stdioPipe Write error.", "error", fmt.Sprintf("%+v", err), "bytes_intended", len(p), "bytes_written", n)
	} else if n != len(p) {
		// Log a warning if the number of bytes written doesn't match the expected length.
		stdioTransportLogger.Warn("stdioPipe Write incomplete.", "bytes_expected", len(p), "bytes_written", n)
	}

	// Flushing stdout is typically not required when writing to pipes or terminals.
	// if f, ok := os.Stdout.(interface{ Flush() error }); ok {
	//     f.Flush()
	// }

	return n, err // Return the result of os.Stdout.Write.
}

// Close is a no-op for stdioPipe.
// Closing os.Stdin/os.Stdout is generally managed by the operating system or the parent process.
func (s stdioPipe) Close() error {
	stdioTransportLogger.Debug("stdioPipe Close called (no-op).")
	return nil
}

// min is a helper function for limiting log sample sizes.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StdioTransport manages a JSON-RPC 2.0 connection over standard input/output.
// It wraps the jsonrpc2.Conn instance.
type StdioTransport struct {
	conn   *jsonrpc2.Conn // The underlying jsonrpc2 connection instance.
	closed bool           // Flag indicating if the transport's Close method has been called.
	// 'debug' field is deprecated; rely on logging levels instead.
}

// NewStdioTransport creates a new, unconnected StdioTransport instance.
func NewStdioTransport() *StdioTransport {
	stdioTransportLogger.Debug("Creating new StdioTransport instance.")
	return &StdioTransport{}
}

// WithDebug sets the debug flag on the transport instance.
// DEPRECATED: Control debug behavior via the global structured logging level.
func (t *StdioTransport) WithDebug(debug bool) *StdioTransport {
	stdioTransportLogger.Warn("StdioTransport.WithDebug is deprecated; use logging levels.")
	// Avoid setting deprecated fields if they are no longer used internally.
	return t
}

// Connect establishes the JSON-RPC connection using stdioPipe and the provided handler.
// It uses jsonrpc2.NewPlainObjectStream for newline-delimited JSON suitable for stdio.
func (t *StdioTransport) Connect(ctx context.Context, handler jsonrpc2.Handler) (*jsonrpc2.Conn, error) {
	stdioTransportLogger.Debug("Connecting StdioTransport...")

	// Create the stream wrapper using our stdioPipe implementation.
	stream := jsonrpc2.NewPlainObjectStream(stdioPipe{})
	stdioTransportLogger.Debug("Created PlainObjectStream over stdioPipe.")

	// Optionally add connection options for verbose message logging via jsonrpc2.
	// connOpts := []jsonrpc2.ConnOpt{
	//     jsonrpc2.LogMessages(logging.NewJsonrpc2Logger(stdioTransportLogger.With("component", "jsonrpc2_conn"))),
	// }
	// conn := jsonrpc2.NewConn(ctx, stream, handler, connOpts...)

	// Create the connection without verbose internal message logging by default.
	conn := jsonrpc2.NewConn(ctx, stream, handler)
	if conn == nil {
		// Although NewConn doesn't explicitly return an error, check for nil defensively.
		err := errors.New("jsonrpc2.NewConn returned nil connection unexpectedly")
		stdioTransportLogger.Error("Failed to create jsonrpc2 connection.", "error", err)
		return nil, err
	}

	t.conn = conn
	t.closed = false

	stdioTransportLogger.Info("Stdio transport connected successfully.")
	return conn, nil
}

// Close terminates the underlying jsonrpc2 connection if it's active.
// It is safe to call multiple times.
func (t *StdioTransport) Close() error {
	if t.closed {
		stdioTransportLogger.Debug("StdioTransport Close called, but already marked as closed.")
		return nil
	}

	if t.conn == nil {
		// If Close is called before Connect or after a failed Connect.
		stdioTransportLogger.Debug("StdioTransport Close called, but connection was not established or already nil.")
		t.closed = true // Ensure marked as closed even if conn was nil.
		return nil
	}

	stdioTransportLogger.Debug("Closing underlying stdio jsonrpc2 connection...")
	// Mark closed immediately to prevent race conditions if Close is called concurrently.
	t.closed = true
	err := t.conn.Close() // Attempt to close the jsonrpc2 connection.
	if err != nil {
		// Log the detailed error, potentially including stack trace if using cockroachdb/errors.
		stdioTransportLogger.Error("Error closing stdio jsonrpc2 connection.", "error", fmt.Sprintf("%+v", err))
		// Wrap the error for upstream callers.
		return errors.Wrap(err, "StdioTransport.Close failed")
	}

	stdioTransportLogger.Info("Stdio transport connection closed successfully.")
	return nil
}

// StdioTransportOption defines a function type for configuring stdioTransportOptions.
type StdioTransportOption func(*stdioTransportOptions)

// stdioTransportOptions holds configuration values for the StdioTransport.
type stdioTransportOptions struct {
	requestTimeout time.Duration // Informational, likely not enforced by stdio transport.
	readTimeout    time.Duration // Informational, likely not enforced by stdio transport.
	writeTimeout   time.Duration // Informational, likely not enforced by stdio transport.
	debug          bool          // Deprecated flag.
}

// WithStdioRequestTimeout returns an option to set the desired request timeout.
// WARNING: Timeout likely not enforced by this transport layer. Requires handler implementation.
func WithStdioRequestTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.requestTimeout = timeout
			stdioTransportLogger.Debug("Stdio request timeout configured (informational).", "timeout", timeout)
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio request timeout value (non-positive).", "invalid_timeout", timeout)
		}
	}
}

// WithStdioReadTimeout returns an option to set the desired read timeout.
// WARNING: Timeout likely NOT enforced by this transport layer.
func WithStdioReadTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.readTimeout = timeout
			stdioTransportLogger.Debug("Stdio read timeout configured (informational).", "timeout", timeout)
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio read timeout value (non-positive).", "invalid_timeout", timeout)
		}
	}
}

// WithStdioWriteTimeout returns an option to set the desired write timeout.
// WARNING: Timeout likely NOT enforced by this transport layer.
func WithStdioWriteTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		if timeout > 0 {
			opts.writeTimeout = timeout
			stdioTransportLogger.Debug("Stdio write timeout configured (informational).", "timeout", timeout)
		} else {
			stdioTransportLogger.Warn("Ignoring invalid stdio write timeout value (non-positive).", "invalid_timeout", timeout)
		}
	}
}

// WithStdioDebug returns an option to set the deprecated debug flag.
// DEPRECATED: Use logging configuration to control debug output.
func WithStdioDebug(debug bool) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.debug = debug // Store option value for potential backward compatibility checks.
		stdioTransportLogger.Warn("WithStdioDebug option is deprecated; use logging levels.")
	}
}

// RunStdioServer sets up and runs a JSON-RPC 2.0 server over stdin/stdout.
// It blocks until the connection terminates. It returns nil on normal disconnection
// or an error if setup fails.
func RunStdioServer(handler jsonrpc2.Handler, options ...StdioTransportOption) error {
	// Initialize default options.
	opts := stdioTransportOptions{
		requestTimeout: 30 * time.Second,  // Default informational timeout.
		readTimeout:    120 * time.Second, // Default informational timeout.
		writeTimeout:   30 * time.Second,  // Default informational timeout.
		debug:          false,             // Deprecated default.
	}
	// Apply provided functional options.
	for _, option := range options {
		option(&opts)
	}

	// Base debug behavior on the logging framework's state.
	effectiveDebug := logging.IsDebugEnabled()

	stdioTransportLogger.Info("Starting stdio JSON-RPC server.",
		"config_request_timeout", opts.requestTimeout.String(),
		"config_read_timeout", opts.readTimeout.String(),
		"config_write_timeout", opts.writeTimeout.String(),
		"config_debug_option_passed", opts.debug, // Log the value passed via deprecated option.
		"effective_debug_logging", effectiveDebug,
	)

	// Create the transport instance.
	transport := NewStdioTransport()
	// If the deprecated 'debug' flag is still needed for *non-logging* conditional logic:
	// transport = transport.WithDebug(opts.debug) // Pass the configured value.

	// Ensure the transport connection is closed when RunStdioServer exits.
	defer func() {
		stdioTransportLogger.Debug("RunStdioServer defer: Closing stdio transport.")
		if err := transport.Close(); err != nil {
			stdioTransportLogger.Error("Error closing stdio transport during deferred cleanup.", "error", fmt.Sprintf("%+v", err))
		} else {
			stdioTransportLogger.Debug("Stdio transport closed successfully during defer.")
		}
	}()

	// Create a background context for the server's lifetime.
	// Use context.WithCancel if graceful shutdown signaling is needed via context.
	rootCtx := context.Background()

	// Establish the connection using the transport.
	conn, err := transport.Connect(rootCtx, handler)
	if err != nil {
		// Wrap the connection error with context.
		wrappedErr := errors.Wrap(err, "RunStdioServer: failed to connect stdio transport")
		stdioTransportLogger.Error("Failed to establish stdio connection.", "error", fmt.Sprintf("%+v", wrappedErr))
		// Return a structured error if using cgerr, otherwise return wrappedErr.
		return cgerr.ErrorWithDetails(
			wrappedErr,
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"transport":              "stdio",
				"request_timeout_config": opts.requestTimeout.String(), // Include relevant config in error details.
				"read_timeout_config":    opts.readTimeout.String(),
				"write_timeout_config":   opts.writeTimeout.String(),
			},
		) // Or: return wrappedErr
	}

	// Server successfully started.
	startTime := time.Now()
	disconnectChan := conn.DisconnectNotify() // Channel signals when connection is closed.
	stdioTransportLogger.Info("Stdio server running. Waiting for disconnection signal.")

	// Optional heartbeat logging ticker (only active if debug logging is enabled).
	var tickerC <-chan time.Time
	if effectiveDebug {
		// Use a reasonable interval for heartbeat logging.
		heartbeatInterval := 60 * time.Second
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop() // Ensure ticker resources are released.
		tickerC = ticker.C
		stdioTransportLogger.Debug("Stdio connection heartbeat ticker started.", "interval", heartbeatInterval.String())
	} else {
		// Use a nil channel if debug logging is off; select will block indefinitely on it.
		tickerC = nil
	}

	// Main loop: Wait for disconnection or heartbeat tick.
	// This assumes RunStdioServer runs until the client disconnects or an internal error causes disconnect.
	for {
		select {
		case <-disconnectChan:
			// The underlying jsonrpc2 connection's main loop has exited.
			duration := time.Since(startTime)
			// We cannot retrieve a specific error cause directly here just from the channel closing.
			// Errors leading to disconnection (e.g., I/O errors, handler panics, context cancellation)
			// should have been logged previously when they occurred.
			// This signal simply means the connection's run loop has finished.
			stdioTransportLogger.Info("Stdio transport disconnected.",
				"total_duration", duration.String(),
			)
			// Return nil to indicate the server loop completed normally after disconnection.
			return nil

		case <-tickerC:
			// This case only executes if tickerC is not nil (i.e., effectiveDebug is true).
			stdioTransportLogger.Debug("Stdio connection heartbeat.",
				"active_duration", time.Since(startTime).String(),
			)
			// Continue loop to wait for next event.
		}
	}
	// This point is typically unreachable if the loop only exits via disconnectChan.
}
