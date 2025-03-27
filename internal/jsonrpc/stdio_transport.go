// Package jsonrpc provides JSON-RPC 2.0 functionality for the MCP server.
// file: internal/jsonrpc/stdio_transport.go
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

	"github.com/sourcegraph/jsonrpc2"
)

// StdioTransport implements JSON-RPC 2.0 communication over standard input/output.
// This transport is specifically designed for MCP (Model Context Protocol) which
// requires communication between processes using stdio. It handles message
// framing and concurrency according to the MCP specification.
type StdioTransport struct {
	ctx       context.Context
	cancel    context.CancelFunc
	conn      *jsonrpc2.Conn
	handler   jsonrpc2.Handler
	reader    *bufio.Reader
	writer    *bufio.Writer
	writeMu   sync.Mutex
	contentLn string // Content-Length header used for message framing
}

// NewStdioTransport creates a new stdio transport for JSON-RPC.
// It sets up the transport to read from stdin and write to stdout.
func NewStdioTransport(handler jsonrpc2.Handler) *StdioTransport {
	ctx, cancel := context.WithCancel(context.Background())
	return &StdioTransport{
		ctx:       ctx,
		cancel:    cancel,
		handler:   handler,
		reader:    bufio.NewReader(os.Stdin),
		writer:    bufio.NewWriter(os.Stdout),
		contentLn: "Content-Length: ",
	}
}

// Start starts the transport and begins processing messages.
// It reads messages from stdin, processes them through the JSON-RPC handler,
// and writes responses to stdout.
func (t *StdioTransport) Start() error {
	stream := &stdioObjectStream{
		reader:    t.reader,
		writer:    t.writer,
		writeMu:   &t.writeMu,
		contentLn: t.contentLn,
	}

	t.conn = jsonrpc2.NewConn(t.ctx, stream, t.handler)

	// Wait for the connection to close
	<-t.conn.DisconnectNotify()
	return nil
}

// Stop stops the transport and cleans up resources.
func (t *StdioTransport) Stop() error {
	if t.conn != nil {
		if err := t.conn.Close(); err != nil {
			return fmt.Errorf("StdioTransport.Stop: failed to close connection: %w", err)
		}
	}

	t.cancel()
	return nil
}

// stdioObjectStream implements the jsonrpc2.ObjectStream interface for stdio.
type stdioObjectStream struct {
	reader    *bufio.Reader
	writer    *bufio.Writer
	writeMu   *sync.Mutex
	contentLn string
}

// WriteObject writes a JSON-RPC message to stdout with proper framing.
// It follows the Content-Length framing protocol used by MCP.
func (s *stdioObjectStream) WriteObject(obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("stdioObjectStream.WriteObject: failed to marshal object: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Write header with content length
	header := fmt.Sprintf("%s%d\r\n\r\n", s.contentLn, len(data))
	if _, err := s.writer.WriteString(header); err != nil {
		return fmt.Errorf("stdioObjectStream.WriteObject: failed to write header: %w", err)
	}

	// Write JSON data
	if _, err := s.writer.Write(data); err != nil {
		return fmt.Errorf("stdioObjectStream.WriteObject: failed to write data: %w", err)
	}

	// Flush to ensure the message is sent immediately
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("stdioObjectStream.WriteObject: failed to flush writer: %w", err)
	}

	return nil
}

// ReadObject reads a JSON-RPC message from stdin with proper framing.
// It handles the Content-Length framing protocol used by MCP.
func (s *stdioObjectStream) ReadObject(v interface{}) error {
	// Read headers until we find Content-Length
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Handle EOF gracefully
				return io.EOF
			}
			return fmt.Errorf("stdioObjectStream.ReadObject: failed to read header: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line indicates end of headers
			break
		}

		if strings.HasPrefix(line, s.contentLn) {
			// Parse content length
			lenStr := strings.TrimPrefix(line, s.contentLn)
			if _, err := fmt.Sscanf(lenStr, "%d", &contentLength); err != nil {
				return fmt.Errorf("stdioObjectStream.ReadObject: invalid Content-Length: %w", err)
			}
		}
	}

	if contentLength <= 0 {
		return fmt.Errorf("stdioObjectStream.ReadObject: Content-Length header missing or invalid")
	}

	// Read the exact number of bytes specified by Content-Length
	data := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, data); err != nil {
		return fmt.Errorf("stdioObjectStream.ReadObject: failed to read message body: %w", err)
	}

	// Unmarshal the JSON data
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("stdioObjectStream.ReadObject: failed to unmarshal JSON: %w", err)
	}

	return nil
}

// Close closes the stream.
func (s *stdioObjectStream) Close() error {
	return nil // No resources to close for stdio
}

// RunStdioServer is a helper function that creates and starts a JSON-RPC server with stdio transport.
// This is the main entry point for running an MCP server over stdio.
func RunStdioServer(adapter *Adapter) error {
	log.Println("Starting JSON-RPC server with stdio transport")

	// Create the transport
	transport := NewStdioTransport(adapter)

	// Start processing messages
	if err := transport.Start(); err != nil {
		return fmt.Errorf("RunStdioServer: failed to start transport: %w", err)
	}

	return nil
}
