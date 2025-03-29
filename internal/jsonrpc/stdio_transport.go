// internal/jsonrpc/stdio_transport.go
package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// StdioTransport implements JSON-RPC 2.0 communication over standard input/output.
// This transport is specifically designed for MCP (Model Context Protocol) which
// requires communication between processes using stdio. It handles message
// framing and concurrency according to the MCP specification.
type StdioTransport struct {
	ctx            context.Context
	cancel         context.CancelFunc
	conn           *jsonrpc2.Conn
	handler        jsonrpc2.Handler
	reader         *bufio.Reader
	writer         *bufio.Writer
	writeMu        sync.Mutex
	contentLn      string // Content-Length header used for message framing
	requestTimeout time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	debug          bool // Enable debug logging
}

// StdioTransportOption defines a function that configures a StdioTransport.
type StdioTransportOption func(*StdioTransport)

// WithStdioRequestTimeout sets the request timeout for the StdioTransport.
func WithStdioRequestTimeout(timeout time.Duration) StdioTransportOption {
	return func(t *StdioTransport) {
		t.requestTimeout = timeout
	}
}

// WithStdioReadTimeout sets the read timeout for the StdioTransport.
func WithStdioReadTimeout(timeout time.Duration) StdioTransportOption {
	return func(t *StdioTransport) {
		t.readTimeout = timeout
	}
}

// WithStdioWriteTimeout sets the write timeout for the StdioTransport.
func WithStdioWriteTimeout(timeout time.Duration) StdioTransportOption {
	return func(t *StdioTransport) {
		t.writeTimeout = timeout
	}
}

// WithStdioDebug enables or disables debug logging for the StdioTransport.
func WithStdioDebug(debug bool) StdioTransportOption {
	return func(t *StdioTransport) {
		t.debug = debug
	}
}

// NewStdioTransport creates a new stdio transport for JSON-RPC.
// It sets up the transport to read from stdin and write to stdout.
func NewStdioTransport(handler jsonrpc2.Handler, opts ...StdioTransportOption) *StdioTransport {
	ctx, cancel := context.WithCancel(context.Background())
	t := &StdioTransport{
		ctx:            ctx,
		cancel:         cancel,
		handler:        handler,
		reader:         bufio.NewReader(os.Stdin),
		writer:         bufio.NewWriter(os.Stdout),
		contentLn:      "Content-Length: ",
		requestTimeout: DefaultTimeout,
		readTimeout:    60 * time.Second, // Increased from 30s to 60s
		writeTimeout:   30 * time.Second,
		debug:          false, // Default to false
	}

	// Apply options
	for _, opt := range opts {
		opt(t)
	}

	return t
}

// Start starts the transport and begins processing messages.
// It reads messages from stdin, processes them through the JSON-RPC handler,
// and writes responses to stdout.
func (t *StdioTransport) Start() error {
	if t.debug {
		log.Printf("StdioTransport: Starting transport with read timeout %v", t.readTimeout)
	}

	stream := &stdioObjectStream{
		reader:       t.reader,
		writer:       t.writer,
		writeMu:      &t.writeMu,
		contentLn:    t.contentLn,
		readTimeout:  t.readTimeout,
		writeTimeout: t.writeTimeout,
		debug:        t.debug,
	}

	t.conn = jsonrpc2.NewConn(t.ctx, stream, t.handler)
	if t.debug {
		log.Printf("StdioTransport: Connection established, waiting for messages")
	}

	// Wait for the connection to close
	<-t.conn.DisconnectNotify()
	if t.debug {
		log.Printf("StdioTransport: Connection closed")
	}
	return nil
}

// Stop stops the transport and cleans up resources.
func (t *StdioTransport) Stop() error {
	if t.debug {
		log.Printf("StdioTransport: Stopping transport")
	}

	if t.conn != nil {
		if err := t.conn.Close(); err != nil {
			return cgerr.ErrorWithDetails(
				errors.Wrap(err, "failed to close connection"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"transport_type": "stdio",
				},
			)
		}
	}

	t.cancel()
	return nil
}

// readResult represents the result of a read operation.
type readResult struct {
	data  []byte
	err   error
	isEOF bool // Explicitly track clean EOF conditions
}

// stdioObjectStream implements the jsonrpc2.ObjectStream interface for stdio.
type stdioObjectStream struct {
	reader       *bufio.Reader
	writer       *bufio.Writer
	writeMu      *sync.Mutex
	contentLn    string
	readTimeout  time.Duration
	writeTimeout time.Duration
	debug        bool
}

// WriteObject writes a JSON-RPC message to stdout with proper framing.
// It follows the Content-Length framing protocol used by MCP.
func (s *stdioObjectStream) WriteObject(obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to marshal object"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"object_type": fmt.Sprintf("%T", obj),
			},
		)
	}

	if s.debug {
		log.Printf("stdioObjectStream.WriteObject: Sending message: %s", string(data))
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Use a write deadline if possible
	done := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		// Write header with content length - THIS IS CRITICAL
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
		if s.debug {
			log.Printf("stdioObjectStream.WriteObject: Writing header: %s", header)
		}

		if _, err := s.writer.WriteString(header); err != nil {
			errCh <- cgerr.ErrorWithDetails(
				errors.Wrap(err, "failed to write header"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"header": header,
				},
			)
			return
		}

		// Write JSON data
		if _, err := s.writer.Write(data); err != nil {
			errCh <- cgerr.ErrorWithDetails(
				errors.Wrap(err, "failed to write data"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"data_size": len(data),
				},
			)
			return
		}

		// Flush to ensure the message is sent immediately
		if err := s.writer.Flush(); err != nil {
			errCh <- cgerr.ErrorWithDetails(
				errors.Wrap(err, "failed to flush writer"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"data_size": len(data),
				},
			)
			return
		}

		if s.debug {
			log.Printf("stdioObjectStream.WriteObject: Message written and flushed successfully")
		}

		close(done)
	}()

	select {
	case <-done:
		return nil
	case err := <-errCh:
		return err
	case <-time.After(s.writeTimeout):
		return cgerr.NewTimeoutError(
			"write operation timed out",
			map[string]interface{}{
				"timeout":   s.writeTimeout.String(),
				"data_size": len(data),
			},
		)
	}
}

// ReadObject reads a JSON-RPC message from stdin, strictly adhering to the
// Content-Length header framing protocol. It includes detailed logging and
// robust error handling.
func (s *stdioObjectStream) ReadObject(v interface{}) error {
	if s.debug {
		// Use a consistent prefix for easier log filtering
		log.Printf("[DEBUG][stdioStream] ReadObject: Waiting to read message (timeout: %v)...", s.readTimeout)
	}

	resultCh := make(chan readResult, 1)

	// Read operation runs in a goroutine to allow for timeout.
	go func() {
		// Step 1: Read headers to find Content-Length
		contentLength, err := s.readHeaders()
		if err != nil {
			// Check for clean EOF specifically
			if errors.Is(err, io.EOF) {
				if s.debug {
					log.Printf("[DEBUG][stdioStream] ReadObject: Clean EOF detected while reading headers.")
				}
				resultCh <- readResult{nil, nil, true} // Signal clean EOF
				return
			}
			// Log and return other header reading errors
			log.Printf("[ERROR][stdioStream] ReadObject: Error reading headers: %+v", err) // Log full wrapped error
			resultCh <- readResult{nil, err, false}
			return
		}
		if s.debug {
			log.Printf("[DEBUG][stdioStream] ReadObject: Successfully parsed Content-Length: %d", contentLength)
		}

		// Step 2: Read the exact message body size
		data, err := s.readMessageBody(contentLength)
		if err != nil {
			log.Printf("[ERROR][stdioStream] ReadObject: Error reading message body (expected %d bytes): %+v", contentLength, err)
			resultCh <- readResult{nil, err, false}
			return
		}
		if s.debug {
			log.Printf("[DEBUG][stdioStream] ReadObject: Successfully read %d bytes for message body.", len(data))
		}

		// Step 3: Send the successfully read data for unmarshalling
		resultCh <- readResult{data, nil, false}
	}()

	// Wait for the read operation to complete or timeout
	select {
	case res := <-resultCh:
		// processReadResult handles unmarshalling, EOF propagation, and error returns
		return s.processReadResult(res, v)

	case <-time.After(s.readTimeout):
		if s.debug {
			log.Printf("[WARN][stdioStream] ReadObject: Read operation timed out after %v", s.readTimeout)
		}
		return cgerr.NewTimeoutError( // Return a specific timeout error
			fmt.Sprintf("read operation timed out after %v", s.readTimeout),
			map[string]interface{}{
				"timeout_duration": s.readTimeout.String(),
				"target_type":      fmt.Sprintf("%T", v),
			},
		)
	}
}

// readHeaders reads and parses message headers until the empty line separator,
// extracting the Content-Length value.
func (s *stdioObjectStream) readHeaders() (int, error) {
	contentLength := -1 // Use -1 to signify "not found yet"
	if s.debug {
		log.Printf("[DEBUG][stdioStream] readHeaders: Starting header read loop.")
	}

	for {
		// bufio.Reader.ReadString handles \n and \r\n endings correctly.
		line, err := s.reader.ReadString('\n')
		if err != nil {
			// If EOF occurs *before* the empty separator line, it's either a
			// clean closure (if no data read yet) or an unexpected closure.
			// Return io.EOF to signal potential clean closure upstream.
			if errors.Is(err, io.EOF) {
				log.Printf("[INFO][stdioStream] readHeaders: EOF encountered while reading header line.")
				return 0, io.EOF // Signal EOF
			}
			// Other read errors are transport problems.
			return 0, cgerr.ErrorWithDetails(
				errors.Wrap(err, "transport error reading header line"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError, // Treat as internal transport error
				map[string]interface{}{"partial_line_read": line},
			)
		}

		// Remove surrounding whitespace including potential \r
		trimmedLine := strings.TrimSpace(line)
		if s.debug {
			log.Printf("[DEBUG][stdioStream] readHeaders: Read header line: %q", trimmedLine)
		}

		// Empty line marks the end of the header section
		if trimmedLine == "" {
			if s.debug {
				log.Printf("[DEBUG][stdioStream] readHeaders: Empty line detected, ending header read.")
			}
			break // Exit header loop
		}

		// Check for the Content-Length header (case-sensitive based on s.contentLn)
		if strings.HasPrefix(trimmedLine, s.contentLn) {
			lenStr := strings.TrimPrefix(trimmedLine, s.contentLn)
			var cl int
			// Use Sscanf for robust integer parsing from the remaining string part.
			if _, err := fmt.Sscanf(lenStr, "%d", &cl); err != nil || cl < 0 {
				// Invalid format or negative length
				return 0, cgerr.ErrorWithDetails(
					errors.Wrap(err, "invalid Content-Length header value"),
					cgerr.CategoryRPC,
					cgerr.CodeParseError,
					map[string]interface{}{"header_line": trimmedLine},
				)
			}
			contentLength = cl
			if s.debug {
				log.Printf("[DEBUG][stdioStream] readHeaders: Found Content-Length: %d", contentLength)
			}
		}
		// Other headers are simply ignored without the unnecessary empty "else if s.debug" branch
	}

	// After loop, check if Content-Length was actually found
	if contentLength == -1 {
		return 0, cgerr.ErrorWithDetails(
			errors.New("mandatory Content-Length header missing"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError, // It's a parsing/protocol violation
			nil,
		)
	}

	return contentLength, nil
}

// readMessageBody reads exactly 'contentLength' bytes from the reader.
// Uses io.ReadFull for robustness.
func (s *stdioObjectStream) readMessageBody(contentLength int) ([]byte, error) {
	if contentLength == 0 {
		if s.debug {
			log.Printf("[DEBUG][stdioStream] readMessageBody: Content-Length is 0, returning empty body.")
		}
		return []byte{}, nil // Return empty slice, not nil
	}

	if s.debug {
		log.Printf("[DEBUG][stdioStream] readMessageBody: Attempting to read exactly %d bytes.", contentLength)
	}

	data := make([]byte, contentLength)
	// io.ReadFull is crucial: it returns an error (ErrUnexpectedEOF) if
	// the stream ends before reading the full 'contentLength' bytes.
	n, err := io.ReadFull(s.reader, data)
	if err != nil {
		readBytes := n // Capture bytes read even on error
		// If EOF/UnexpectedEOF happens here, it means the stream died after
		// headers promised 'contentLength' bytes, which is a framing error.
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, cgerr.ErrorWithDetails(
				errors.Wrap(err, "stream closed unexpectedly while reading message body"),
				cgerr.CategoryRPC,
				cgerr.CodeParseError, // Framing/parsing issue
				map[string]interface{}{
					"expected_bytes": contentLength,
					"read_bytes":     readBytes,
				},
			)
		}
		// Other read errors are likely transport issues.
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "transport error reading message body"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"expected_bytes": contentLength,
				"read_bytes":     readBytes,
			},
		)
	}

	// Optional logging of received data is handled elsewhere
	return data, nil
}

// processReadResult handles the outcome from the read goroutine, performing
// unmarshalling or propagating errors/EOF.
func (s *stdioObjectStream) processReadResult(res readResult, v interface{}) error {
	// Case 1: Clean EOF detected during header reading
	if res.isEOF {
		if s.debug {
			log.Printf("[INFO][stdioStream] processReadResult: Propagating clean EOF.")
		}
		return io.EOF // Signal clean stream closure to jsonrpc2 library
	}

	// Case 2: Error occurred during readHeaders or readMessageBody
	if res.err != nil {
		// Error already logged where it occurred, just return it.
		if s.debug {
			log.Printf("[DEBUG][stdioStream] processReadResult: Propagating read error: %+v", res.err)
		}
		return res.err
	}

	// Case 3: Data successfully read, attempt to unmarshal
	if s.debug {
		log.Printf("[DEBUG][stdioStream] processReadResult: Attempting to unmarshal %d bytes into %T", len(res.data), v)
	}
	if err := json.Unmarshal(res.data, v); err != nil {
		// Log the raw data that failed (truncated) - THIS IS KEY FOR DEBUGGING
		logData := string(res.data)
		const maxLogData = 512 // Limit log size
		if len(logData) > maxLogData {
			logData = logData[:maxLogData] + "...(truncated)"
		}
		log.Printf("[ERROR][stdioStream] processReadResult: Failed to unmarshal JSON: %v. Raw Data: %q", err, logData)

		// Wrap and return a specific deserialization error
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "JSON unmarshal failed"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError, // It's a parse error according to JSON-RPC spec
			map[string]interface{}{
				"data_size":   len(res.data),
				"target_type": fmt.Sprintf("%T", v),
				"data_prefix": logData, // Include sample data in error details
			},
		)
	}

	// Success
	if s.debug {
		log.Printf("[DEBUG][stdioStream] processReadResult: Successfully unmarshalled message.")
	}
	return nil
}

// Close closes the stream.
func (s *stdioObjectStream) Close() error {
	if s.debug {
		log.Printf("stdioObjectStream.Close: Closing stream")
	}
	return nil // No resources to close for stdio
}

// RunStdioServer is a helper function that creates and starts a JSON-RPC server with stdio transport.
// This is the main entry point for running an MCP server over stdio.
func RunStdioServer(adapter *Adapter, opts ...StdioTransportOption) error {
	log.Println("Starting JSON-RPC server with stdio transport")

	// Create the transport with provided options
	transport := NewStdioTransport(adapter, opts...)

	// Start processing messages
	if err := transport.Start(); err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to start transport"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"transport_type": "stdio",
			},
		)
	}

	return nil
}
