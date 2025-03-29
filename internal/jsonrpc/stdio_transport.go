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
// WriteObject writes a JSON-RPC message with proper Content-Length header.
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

// ReadObject reads a JSON-RPC message from stdin with proper framing.
// It handles the Content-Length framing protocol used by MCP.
// Modified ReadObject to better handle different message formats.
func (s *stdioObjectStream) ReadObject(v interface{}) error {
	if s.debug {
		log.Printf("stdioObjectStream.ReadObject: Starting to read message with timeout %v", s.readTimeout)
	}

	// Create a channel for the read result with a timeout
	resultCh := make(chan readResult, 1)

	// Start read operation in a goroutine
	go func() {
		// Try to peek at the first byte to see if it's direct JSON
		firstByte, err := s.reader.Peek(1)
		if err == nil && len(firstByte) > 0 && firstByte[0] == '{' {
			// This appears to be direct JSON
			if s.debug {
				log.Printf("stdioObjectStream.ReadObject: Detected direct JSON message")
			}

			// Read the entire line
			line, err := s.reader.ReadString('\n')
			if err != nil && err != io.EOF {
				resultCh <- readResult{nil, err, false}
				return
			}

			// Trim any whitespace
			line = strings.TrimSpace(line)
			if s.debug {
				log.Printf("stdioObjectStream.ReadObject: Read direct JSON: %s", line)
			}

			resultCh <- readResult{[]byte(line), nil, false}
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

// readMessage reads a complete message and sends the result to the provided channel.
// This helper function reduces the complexity of ReadObject.
func (s *stdioObjectStream) readMessage(resultCh chan<- readResult) {
	// Read headers to get Content-Length
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
}

// readHeaders reads and parses the message headers to extract Content-Length.
// This helper function reduces the complexity of readMessage.
func (s *stdioObjectStream) readHeaders() (int, error) {
	var contentLength int

	// Read headers until we find Content-Length or an empty line
	for {
		if s.debug {
			log.Printf("stdioObjectStream.readHeaders: Reading header line")
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			return 0, err // This could be io.EOF or another error
		}

		line = strings.TrimSpace(line)
		if s.debug {
			log.Printf("stdioObjectStream.readHeaders: Read header line: %q", line)
		}

		// Empty line indicates end of headers
		if line == "" {
			if s.debug {
				log.Printf("stdioObjectStream.readHeaders: End of headers reached")
			}
			break
		}

		// Parse Content-Length if present
		if strings.HasPrefix(line, s.contentLn) {
			lenStr := strings.TrimPrefix(line, s.contentLn)
			if _, err := fmt.Sscanf(lenStr, "%d", &contentLength); err != nil {
				return 0, cgerr.ErrorWithDetails(
					errors.Wrap(err, "invalid Content-Length"),
					cgerr.CategoryRPC,
					cgerr.CodeParseError,
					map[string]interface{}{
						"content_length_str": lenStr,
					},
				)
			}
			if s.debug {
				log.Printf("stdioObjectStream.readHeaders: Parsed Content-Length: %d", contentLength)
			}
		}
	}

	// Validate Content-Length
	if contentLength <= 0 {
		return 0, cgerr.ErrorWithDetails(
			errors.New("Content-Length header missing or invalid"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"content_length": contentLength,
			},
		)
	}

	return contentLength, nil
}

// readMessageBody reads the message body based on the provided Content-Length.
// This helper function reduces the complexity of readMessage.
func (s *stdioObjectStream) readMessageBody(contentLength int) ([]byte, error) {
	if s.debug {
		log.Printf("stdioObjectStream.readMessageBody: Reading %d bytes of message body", contentLength)
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

	// Unmarshal the JSON data
	if err := json.Unmarshal(res.data, v); err != nil {
		if s.debug {
			log.Printf("stdioObjectStream.processReadResult: Failed to unmarshal JSON: %v", err)
		}
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to unmarshal JSON"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"data_size":   len(res.data),
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	if s.debug {
		log.Printf("stdioObjectStream.processReadResult: Successfully unmarshalled object")
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
