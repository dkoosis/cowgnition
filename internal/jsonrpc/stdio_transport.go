// internal/jsonrpc/stdio_transport.go
package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/textproto" // ADDED: Import textproto for standard header parsing
	"os"
	"strconv" // ADDED: For more robust string to int conversion
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
		contentLn:      "Content-Length", // CHANGED: Removed trailing colon and space for proper header matching
		requestTimeout: DefaultTimeout,
		readTimeout:    120 * time.Second, // INCREASED: Doubled from 60s to 120s for better tolerance of slow connections
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
	isEOF bool
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
		// IMPROVED: Use more canonical header format with colon directly after header name
		// Write header with content length - THIS IS CRITICAL
		header := fmt.Sprintf("%s: %d\r\n\r\n", s.contentLn, len(data))
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

// ReadObject reads a JSON-RPC message from stdin with proper framing.
// It handles the Content-Length framing protocol used by MCP, but also
// attempts to handle direct JSON messages for better compatibility.
func (s *stdioObjectStream) ReadObject(v interface{}) error {
	if s.debug {
		log.Printf("stdioObjectStream.ReadObject: Starting to read message with timeout %v", s.readTimeout)
	}

	// Create a channel for the read result with a timeout
	resultCh := make(chan readResult, 1)

	// Start read operation in a goroutine
	go func() {
		// IMPROVED: First try to peek at the first byte to determine the message format
		firstByte, err := s.reader.Peek(1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				resultCh <- readResult{nil, nil, true}
			} else {
				resultCh <- readResult{nil, err, false}
			}
			return
		}

		// Check if this appears to be direct JSON (starting with '{')
		// ADDED: Direct JSON handling for better compatibility with clients
		// that don't use the Content-Length framing
		if len(firstByte) > 0 && firstByte[0] == '{' {
			if s.debug {
				log.Printf("stdioObjectStream.ReadObject: Detected direct JSON message")
			}

			// Read the entire line or until we get a complete JSON object
			data, err := s.readDirectJSON()
			if err != nil {
				resultCh <- readResult{nil, err, false}
				return
			}

			resultCh <- readResult{data, nil, false}
			return
		}

		// Standard Content-Length header approach
		contentLength, err := s.readHeaders()
		if err != nil {
			if errors.Is(err, io.EOF) {
				resultCh <- readResult{nil, nil, true}
				return
			}
			resultCh <- readResult{nil, err, false}
			return
		}

		// Read message body based on Content-Length
		data, err := s.readMessageBody(contentLength)
		if err != nil {
			resultCh <- readResult{nil, err, false}
			return
		}

		resultCh <- readResult{data, nil, false}
	}()

	// Wait for result or timeout
	select {
	case res := <-resultCh:
		return s.processReadResult(res, v)
	case <-time.After(s.readTimeout):
		if s.debug {
			log.Printf("stdioObjectStream.ReadObject: Read timeout after %v", s.readTimeout)
		}
		return cgerr.NewTimeoutError(
			"read operation timed out",
			map[string]interface{}{
				"timeout":      s.readTimeout.String(),
				"target_type":  fmt.Sprintf("%T", v),
				"read_timeout": s.readTimeout,
			},
		)
	}
}

// readDirectJSON reads a complete JSON object directly from the stream.
// REFACTORED: Reduced cyclomatic complexity by simplifying and factoring out logic.
func (s *stdioObjectStream) readDirectJSON() ([]byte, error) {
	var (
		data       []byte
		braceCount int
		foundBrace bool
	)

	// State tracking for JSON parsing
	state := &jsonParserState{}

	// Keep reading until we have a complete JSON object
	for {
		buffer := make([]byte, 1) // Read one byte at a time
		n, err := s.reader.Read(buffer)
		if err != nil {
			// Handle EOF specially - it's ok if we've found a complete object
			if err == io.EOF && braceCount == 0 && foundBrace {
				break
			}
			return nil, err
		}

		if n == 0 {
			continue
		}

		char := buffer[0]
		data = append(data, char)

		// Update parsing state based on the current character
		state.update(char)

		// Only count braces outside of strings
		if !state.inQuote {
			if char == '{' {
				braceCount++
				foundBrace = true
			} else if char == '}' {
				braceCount--
			}

			// If we've found a matching closing brace, we're done
			if foundBrace && braceCount == 0 {
				break
			}
		}
	}

	if s.debug {
		log.Printf("stdioObjectStream.readDirectJSON: Read JSON: %s", string(data))
	}

	return data, nil
}

// jsonParserState tracks the state of JSON parsing to handle quoting and escaping.
type jsonParserState struct {
	inQuote    bool
	escapeNext bool
}

// update updates the parser state based on the current character.
func (s *jsonParserState) update(char byte) {
	if char == '"' && !s.escapeNext {
		s.inQuote = !s.inQuote
	}

	if char == '\\' && s.inQuote && !s.escapeNext {
		s.escapeNext = true
	} else {
		s.escapeNext = false
	}
}

// readHeaders reads and parses the message headers to extract Content-Length.
// Now uses textproto for standard header parsing.
func (s *stdioObjectStream) readHeaders() (int, error) {
	if s.debug {
		log.Printf("stdioObjectStream.readHeaders: Reading headers using textproto")
	}

	// Create a textproto reader from our bufio reader
	tr := textproto.NewReader(s.reader)

	// ReadMIMEHeader parses headers until a blank line
	headers, err := tr.ReadMIMEHeader()
	if err != nil {
		return 0, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read headers"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	if s.debug {
		log.Printf("stdioObjectStream.readHeaders: Headers read: %v", headers)
	}

	// Get Content-Length header, normalizing on case
	contentLengthStr := headers.Get(s.contentLn)
	if contentLengthStr == "" {
		return 0, cgerr.ErrorWithDetails(
			errors.New("Content-Length header missing"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"headers": headers,
			},
		)
	}

	// Parse Content-Length value
	contentLength, err := strconv.Atoi(contentLengthStr)
	if err != nil {
		return 0, cgerr.ErrorWithDetails(
			errors.Wrap(err, "invalid Content-Length"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"content_length_str": contentLengthStr,
			},
		)
	}

	// Validate Content-Length
	if contentLength <= 0 {
		return 0, cgerr.ErrorWithDetails(
			errors.New("Content-Length must be positive"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"content_length": contentLength,
			},
		)
	}

	if s.debug {
		log.Printf("stdioObjectStream.readHeaders: Parsed Content-Length: %d", contentLength)
	}

	return contentLength, nil
}

// readMessageBody reads the message body based on the provided Content-Length.
func (s *stdioObjectStream) readMessageBody(contentLength int) ([]byte, error) {
	if s.debug {
		log.Printf("stdioObjectStream.readMessageBody: Reading %d bytes of message body", contentLength)
	}

	// Safety check for very large content lengths
	if contentLength > 100*1024*1024 { // 100MB limit
		return nil, cgerr.ErrorWithDetails(
			errors.New("Content-Length too large"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"content_length": contentLength,
				"max_allowed":    100 * 1024 * 1024,
			},
		)
	}

	data := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, data); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read message body"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"expected_bytes": contentLength,
				"error":          err.Error(),
			},
		)
	}

	if s.debug {
		log.Printf("stdioObjectStream.readMessageBody: Successfully read message: %s", string(data))
	}

	return data, nil
}

// processReadResult processes the result of a read operation.
// This helper function reduces the complexity of ReadObject.
func (s *stdioObjectStream) processReadResult(res readResult, v interface{}) error {
	if res.isEOF {
		if s.debug {
			log.Printf("stdioObjectStream.processReadResult: Returning EOF")
		}
		return io.EOF
	}

	if res.err != nil {
		if s.debug {
			log.Printf("stdioObjectStream.processReadResult: Returning error: %v", res.err)
		}
		return res.err
	}

	// Added more information about the received data
	if s.debug {
		log.Printf("stdioObjectStream.processReadResult: Processing %d bytes of data", len(res.data))
	}

	// Unmarshal the JSON data
	if err := json.Unmarshal(res.data, v); err != nil {
		if s.debug {
			log.Printf("stdioObjectStream.processReadResult: Failed to unmarshal JSON: %v", err)
			log.Printf("stdioObjectStream.processReadResult: Problematic JSON: %s", string(res.data))
		}
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to unmarshal JSON"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"data_size":   len(res.data),
				"target_type": fmt.Sprintf("%T", v),
				"data_sample": string(res.data[:min(100, len(res.data))]), // Include a sample of the data
			},
		)
	}

	if s.debug {
		log.Printf("stdioObjectStream.processReadResult: Successfully unmarshalled object")
	}
	return nil
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
