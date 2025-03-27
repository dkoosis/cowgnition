// Package jsonrpc provides JSON-RPC 2.0 functionality for the MCP server.
// file: internal/jsonrpc/http_transport.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/sourcegraph/jsonrpc2"
)

// HTTPHandler handles JSON-RPC over HTTP requests.
type HTTPHandler struct {
	handler jsonrpc2.Handler
}

// NewHTTPHandler creates a new HTTP handler for JSON-RPC requests.
func NewHTTPHandler(handler jsonrpc2.Handler) *HTTPHandler {
	return &HTTPHandler{
		handler: handler,
	}
}

// ServeHTTP implements the http.Handler interface.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Create a stream from the request and response
	stream := &httpStream{
		reader: r.Body,
		writer: w,
	}

	// Create a connection
	conn := jsonrpc2.NewConn(ctx, stream, h.handler)
	<-conn.DisconnectNotify()
}

// httpStream implements the jsonrpc2.ObjectStream interface for HTTP.
type httpStream struct {
	reader io.ReadCloser
	writer http.ResponseWriter
	closed bool
}

// WriteObject writes a JSON-RPC message to the HTTP response.
func (s *httpStream) WriteObject(obj interface{}) error {
	if s.closed {
		return io.ErrClosedPipe
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	s.writer.Header().Set("Content-Type", "application/json")
	_, err = s.writer.Write(data)
	return err
}

// ReadObject reads a JSON-RPC message from the HTTP request.
func (s *httpStream) ReadObject(v interface{}) error {
	if s.closed {
		return io.ErrClosedPipe
	}

	data, err := io.ReadAll(s.reader)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// Close closes the stream.
func (s *httpStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.reader.Close()
}
