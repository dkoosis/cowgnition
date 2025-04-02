// internal/jsonrpc/stdio_transport.go
package jsonrpc

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// stdioPipe implements io.ReadWriteCloser using standard input/output.
type stdioPipe struct{}

// Read reads from stdin.
func (stdioPipe) Read(p []byte) (n int, err error) {
	return os.Stdin.Read(p)
}

// Write writes to stdout.
func (stdioPipe) Write(p []byte) (n int, err error) {
	return os.Stdout.Write(p)
}

// Close is a no-op for stdin/stdout.
func (stdioPipe) Close() error {
	// We don't actually close stdin/stdout as that would terminate the process
	return nil
}

// StdioTransport implements a stdio-based transport for JSON-RPC over MCP.
// It uses plain (newline-delimited) JSON messages without Content-Length headers
// as specified in the MCP protocol.
type StdioTransport struct {
	conn   *jsonrpc2.Conn
	closed bool
	debug  bool
}

// NewStdioTransport creates a new StdioTransport.
// The transport is not connected until Connect is called.
func NewStdioTransport() *StdioTransport {
	return &StdioTransport{
		debug: false,
	}
}

// WithDebug enables debug logging for the transport.
func (t *StdioTransport) WithDebug(debug bool) *StdioTransport {
	t.debug = debug
	return t
}

// Connect initializes the transport with a handler.
// It creates a jsonrpc2.Conn using a plaintext stream over stdin/stdout.
//
// ctx context.Context: The context for the connection.
// handler jsonrpc2.Handler: The handler for JSON-RPC requests.
//
// Returns:
//
//	*jsonrpc2.Conn: The established connection.
//	error: An error if connection fails.
func (t *StdioTransport) Connect(ctx context.Context, handler jsonrpc2.Handler) (*jsonrpc2.Conn, error) {
	// Use NewPlainObjectStream for newline-delimited JSON over stdio
	stream := jsonrpc2.NewPlainObjectStream(stdioPipe{})

	// Create connection with handler
	conn := jsonrpc2.NewConn(ctx, stream, handler)
	t.conn = conn

	if t.debug {
		log.Printf("Connected stdio transport (using NewPlainObjectStream for MCP newline-delimited JSON)")
	}

	return conn, nil
}

// Close terminates the transport connection.
// It's safe to call this method multiple times.
//
// Returns:
//
//	error: An error if closing fails.
func (t *StdioTransport) Close() error {
	if t.closed {
		return nil
	}

	if t.conn != nil {
		t.closed = true
		if t.debug {
			log.Printf("Closing stdio transport connection")
		}
		return t.conn.Close()
	}

	return nil
}

// StdioTransportOption defines an option for StdioTransport configuration.
// These are used to customize the behavior of the transport.
type StdioTransportOption func(*stdioTransportOptions)

// stdioTransportOptions holds configuration for StdioTransport.
type stdioTransportOptions struct {
	requestTimeout time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	debug          bool
}

// WithStdioRequestTimeout sets the request timeout for stdio transport.
// This controls how long a request can take before timing out.
//
// timeout time.Duration: The timeout duration.
//
// Returns:
//
//	StdioTransportOption: An option function.
func WithStdioRequestTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.requestTimeout = timeout
	}
}

// WithStdioReadTimeout sets the read timeout for stdio transport.
// This controls how long the transport will wait for data to be read.
//
// timeout time.Duration: The timeout duration.
//
// Returns:
//
//	StdioTransportOption: An option function.
func WithStdioReadTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.readTimeout = timeout
	}
}

// WithStdioWriteTimeout sets the write timeout for stdio transport.
// This controls how long the transport will attempt to write data.
//
// timeout time.Duration: The timeout duration.
//
// Returns:
//
//	StdioTransportOption: An option function.
func WithStdioWriteTimeout(timeout time.Duration) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.writeTimeout = timeout
	}
}

// WithStdioDebug enables or disables debug logging for stdio transport.
// When enabled, additional log messages will be printed.
//
// debug bool: Whether to enable debug logging.
//
// Returns:
//
//	StdioTransportOption: An option function.
func WithStdioDebug(debug bool) StdioTransportOption {
	return func(opts *stdioTransportOptions) {
		opts.debug = debug
	}
}

// RunStdioServer runs a JSON-RPC server with stdio transport.
// This will block until the connection is closed by the client or process termination.
//
// handler jsonrpc2.Handler: The handler for JSON-RPC requests.
// options ...StdioTransportOption: Optional configuration options.
//
// Returns:
//
//	error: An error if the server fails to run.
func RunStdioServer(handler jsonrpc2.Handler, options ...StdioTransportOption) error {
	opts := stdioTransportOptions{
		requestTimeout: 30 * time.Second,
		readTimeout:    120 * time.Second, // Increased to 2 minutes for better stability
		writeTimeout:   30 * time.Second,
		debug:          false,
	}

	for _, option := range options {
		option(&opts)
	}

	if opts.debug {
		log.Printf("Starting stdio JSON-RPC server with timeouts (request: %s, read: %s, write: %s)",
			opts.requestTimeout, opts.readTimeout, opts.writeTimeout)
	}

	transport := NewStdioTransport().WithDebug(opts.debug)
	defer func() {
		if err := transport.Close(); err != nil && opts.debug {
			log.Printf("Error closing transport: %v", err)
		}
	}()

	// Create a root context that will live for the duration of the server
	rootCtx := context.Background()

	// Connect the transport
	conn, err := transport.Connect(rootCtx, handler)
	if err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to connect stdio transport"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"request_timeout": opts.requestTimeout.String(),
				"read_timeout":    opts.readTimeout.String(),
				"write_timeout":   opts.writeTimeout.String(),
			},
		)
	}

	// State tracking - aligns with state machine architecture
	connState := "connected"
	lastActivity := time.Now()

	// Get the disconnect notification channel
	disconnectChan := conn.DisconnectNotify()

	if opts.debug {
		log.Printf("MCP Connection established: state=%s", connState)
	}

	// Create a heartbeat ticker to monitor connection health
	// This is important for detecting and logging connection issues
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Block until explicit disconnection - crucial for MCP protocol
	for {
		select {
		case <-disconnectChan:
			connState = "disconnected"
			if opts.debug {
				log.Printf("MCP Connection state change: %s (after %s of activity)",
					connState, time.Since(lastActivity))
			}
			return nil

		case <-ticker.C:
			if opts.debug {
				log.Printf("MCP Connection heartbeat: state=%s, active_duration=%s",
					connState, time.Since(lastActivity))
			}
		}
	}
}
