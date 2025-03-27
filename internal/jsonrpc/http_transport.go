// internal/jsonrpc/http_transport.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// HTTPHandler handles JSON-RPC over HTTP requests.
type HTTPHandler struct {
	handler         jsonrpc2.Handler
	requestTimeout  time.Duration
	shutdownTimeout time.Duration
}

// HTTPHandlerOption defines a function that configures an HTTPHandler.
type HTTPHandlerOption func(*HTTPHandler)

// WithHTTPRequestTimeout sets the request timeout for HTTP handlers.
func WithHTTPRequestTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.requestTimeout = timeout
	}
}

// WithHTTPShutdownTimeout sets the shutdown timeout for HTTP handlers.
func WithHTTPShutdownTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.shutdownTimeout = timeout
	}
}

// NewHTTPHandler creates a new HTTP handler for JSON-RPC requests.
func NewHTTPHandler(handler jsonrpc2.Handler, opts ...HTTPHandlerOption) *HTTPHandler {
	h := &HTTPHandler{
		handler:         handler,
		requestTimeout:  DefaultTimeout,
		shutdownTimeout: 5 * time.Second,
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// ServeHTTP implements the http.Handler interface.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodErr := cgerr.ErrorWithDetails(
			errors.Newf("method %s not allowed", r.Method),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"allowed_method": "POST",
				"actual_method":  r.Method,
				"path":           r.URL.Path,
			},
		)
		log.Printf("httputils.ServeHTTP: %+v", methodErr)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create a cancellable context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	// Create a stream from the request and response
	stream := &httpStream{
		reader: r.Body,
		writer: w,
	}

	// Create a connection
	conn := jsonrpc2.NewConn(ctx, stream, h.handler)

	// Wait for the request to complete or timeout
	select {
	case <-conn.DisconnectNotify():
		// Normal completion
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			// Create a timeout error with details
			timeoutErr := cgerr.NewTimeoutError(
				"HTTP request timed out",
				map[string]interface{}{
					"timeout_seconds": h.requestTimeout.Seconds(),
					"path":            r.URL.Path,
					"remote_addr":     r.RemoteAddr,
				},
			)
			log.Printf("httputils.ServeHTTP: %+v", timeoutErr)

			// Send timeout response
			w.WriteHeader(http.StatusGatewayTimeout)
			_, writeErr := w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"request timed out"},"id":null}`))
			if writeErr != nil {
				writeErrWithDetails := cgerr.ErrorWithDetails(
					errors.Wrap(writeErr, "failed to write timeout error response"),
					cgerr.CategoryRPC,
					cgerr.CodeInternalError,
					map[string]interface{}{
						"path":        r.URL.Path,
						"remote_addr": r.RemoteAddr,
					},
				)
				log.Printf("httputils.ServeHTTP: %+v", writeErrWithDetails)
			}
		}
	}
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
		return cgerr.ErrorWithDetails(
			errors.New("connection closed: pipe is closed"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"object_type": fmt.Sprintf("%T", obj),
			},
		)
	}

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

	s.writer.Header().Set("Content-Type", "application/json")
	_, err = s.writer.Write(data)
	if err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to write response"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"data_size": len(data),
			},
		)
	}

	return nil
}

// ReadObject reads a JSON-RPC message from the HTTP request.
func (s *httpStream) ReadObject(v interface{}) error {
	if s.closed {
		return cgerr.ErrorWithDetails(
			errors.New("connection closed: pipe is closed"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	data, err := io.ReadAll(s.reader)
	if err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read request body"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to unmarshal JSON"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"data_size":   len(data),
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	return nil
}

// Close closes the stream.
func (s *httpStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.reader.Close()
}
