// internal/jsonrpc/http_transport.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io" // Import slog
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// Initialize the logger at the package level
var logger = logging.GetLogger("jsonrpc_http_transport")

// HTTPHandler handles JSON-RPC over HTTP requests.
type HTTPHandler struct {
	handler         jsonrpc2.Handler
	requestTimeout  time.Duration
	shutdownTimeout time.Duration // Currently unused, consider for graceful shutdown
}

// HTTPHandlerOption defines a function that configures an HTTPHandler.
type HTTPHandlerOption func(*HTTPHandler)

// WithHTTPRequestTimeout sets the request timeout for HTTP handlers.
func WithHTTPRequestTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if timeout > 0 {
			h.requestTimeout = timeout
			// Log setting application? Potentially noisy. Debug level if desired.
			// logger.Debug("HTTP request timeout set", "timeout", timeout)
		} else {
			logger.Warn("Ignoring invalid HTTP request timeout value", "invalid_timeout", timeout)
		}
	}
}

// WithHTTPShutdownTimeout sets the shutdown timeout for HTTP handlers.
func WithHTTPShutdownTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if timeout > 0 {
			h.shutdownTimeout = timeout
			// logger.Debug("HTTP shutdown timeout set", "timeout", timeout)
		} else {
			logger.Warn("Ignoring invalid HTTP shutdown timeout value", "invalid_timeout", timeout)
		}
	}
}

// NewHTTPHandler creates a new HTTP handler for JSON-RPC requests.
func NewHTTPHandler(handler jsonrpc2.Handler, opts ...HTTPHandlerOption) *HTTPHandler {
	h := &HTTPHandler{
		handler:         handler,
		requestTimeout:  DefaultTimeout,
		shutdownTimeout: 5 * time.Second, // Default shutdown timeout
	}
	logger.Debug("Initializing new JSON-RPC HTTP Handler", "default_request_timeout", h.requestTimeout, "default_shutdown_timeout", h.shutdownTimeout)

	// Apply options
	for _, opt := range opts {
		opt(h)
	}
	logger.Debug("HTTP Handler options applied", "final_request_timeout", h.requestTimeout, "final_shutdown_timeout", h.shutdownTimeout)

	return h
}

// ServeHTTP implements the http.Handler interface.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestLogger := logger.With("method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
	requestLogger.Debug("Handling HTTP request")

	if r.Method != http.MethodPost {
		methodErr := cgerr.ErrorWithDetails(
			// Add handler context to error message
			errors.Newf("HTTPHandler.ServeHTTP: method %s not allowed", r.Method),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"allowed_method": "POST",
				"actual_method":  r.Method,
				// No need for path/remote_addr here, already in logger context
			},
		)
		// Replace log.Printf with logger.Error (Conceptual L50)
		requestLogger.Error("HTTP method not allowed", "error", fmt.Sprintf("%+v", methodErr))
		// Send standard HTTP error
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create a cancellable context with timeout for the request processing
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel() // Ensure resources are released

	// Create a stream wrapper for the HTTP request/response
	stream := &httpStream{
		reader: r.Body,
		writer: w,
		// closed defaults to false
	}

	// Create a new jsonrpc2 connection using the stream and the registered handler
	conn := jsonrpc2.NewConn(ctx, stream, h.handler)
	requestLogger.Debug("JSON-RPC connection created over HTTP stream")

	// Wait for the connection to disconnect (request processed) or context to finish (timeout/cancel)
	select {
	case <-conn.DisconnectNotify():
		requestLogger.Debug("JSON-RPC connection disconnected normally")
		// Normal completion or handled internally by jsonrpc2 sending response/error
	case <-ctx.Done():
		requestLogger.Warn("HTTP request context done", "reason", ctx.Err())
		// Context finished, check if it was due to timeout
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Create a timeout error with details
			timeoutErr := cgerr.NewTimeoutError(
				// Add handler context to error message
				fmt.Sprintf("HTTPHandler.ServeHTTP: request timed out after %s", h.requestTimeout),
				map[string]interface{}{
					"timeout_seconds": h.requestTimeout.Seconds(),
					// No need for path/remote_addr here, already in logger context
				},
			)
			// Replace log.Printf with logger.Error (Conceptual L80)
			requestLogger.Error("HTTP request timed out", "error", fmt.Sprintf("%+v", timeoutErr))

			// Attempt to send a standard JSON-RPC timeout error response
			// Note: Headers might have already been written by jsonrpc2 internals if processing started.
			// This write might fail or be ignored. jsonrpc2 might handle timeouts internally too.
			// Consider if jsonrpc2 needs specific timeout handling configuration.
			// Best effort:
			if !headersWritten(w) { // Simple check to avoid superfluous WriteHeader
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGatewayTimeout) // Use 504 for timeout
			}
			// Use the cgerr helper to format the error
			rpcErr := cgerr.ToJSONRPCError(timeoutErr)
			// Marshal the standard error
			errBody, marshalErr := json.Marshal(rpcErr)
			if marshalErr != nil {
				requestLogger.Error("Failed to marshal timeout error response body", "marshal_error", fmt.Sprintf("%+v", marshalErr))
				// Cannot send specific error if marshalling failed. Header might already be sent.
				return
			}

			_, writeErr := w.Write(errBody)
			if writeErr != nil {
				// Failed to write the timeout response body itself
				writeErrWithDetails := cgerr.ErrorWithDetails(
					// Add handler context to error message
					errors.Wrap(writeErr, "HTTPHandler.ServeHTTP: failed to write timeout error response body"),
					cgerr.CategoryRPC,
					cgerr.CodeInternalError,
					map[string]interface{}{
						"original_error_code": rpcErr.Code, // Log original intended code
						// path/remote_addr in logger context
					},
				)
				// Replace log.Printf with logger.Error (Conceptual L126)
				requestLogger.Error("Failed to write timeout error response", "write_error", fmt.Sprintf("%+v", writeErrWithDetails))
			}
		}
		// Handle other context errors? (e.g., context.Canceled) Currently just logs.
	}
	requestLogger.Debug("Finished handling HTTP request")
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
		// Add method context to error message
		return cgerr.ErrorWithDetails(
			errors.New("httpStream.WriteObject: connection closed"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"object_type": fmt.Sprintf("%T", obj),
			},
		)
	}

	data, err := json.Marshal(obj)
	if err != nil {
		// Add method context to Wrap message
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "httpStream.WriteObject: failed to marshal object"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"object_type": fmt.Sprintf("%T", obj),
			},
		)
	}

	// Ensure Content-Type is set before writing
	// This might conflict if jsonrpc2 internals also set headers.
	// If headers were already written, this is a no-op.
	s.writer.Header().Set("Content-Type", "application/json")

	_, err = s.writer.Write(data)
	if err != nil {
		// Add method context to Wrap message
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "httpStream.WriteObject: failed to write response"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"data_size": len(data),
			},
		)
	}
	// Should we flush here? http.ResponseWriter might buffer.
	// if f, ok := s.writer.(http.Flusher); ok {
	// 	f.Flush()
	// }

	return nil
}

// ReadObject reads a JSON-RPC message from the HTTP request.
func (s *httpStream) ReadObject(v interface{}) error {
	if s.closed {
		// Add method context to error message
		return cgerr.ErrorWithDetails(
			errors.New("httpStream.ReadObject: connection closed"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	data, err := io.ReadAll(s.reader)
	// Check if the error is due to closing the stream normally
	if s.closed && errors.Is(err, http.ErrBodyReadAfterClose) {
		// This can happen if Close() is called while ReadObject is waiting.
		// Return io.EOF or similar standard stream closed error? jsonrpc2 expects io.EOF.
		logger.Debug("httpStream.ReadObject: Read after close detected, returning io.EOF")
		return io.EOF
	}
	if err != nil {
		// Add method context to Wrap message
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "httpStream.ReadObject: failed to read request body"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError, // Reading error maps to parse error
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	// Handle empty request body - jsonrpc2 might handle this, but good practice
	if len(data) == 0 {
		logger.Warn("httpStream.ReadObject: Received empty request body")
		return cgerr.ErrorWithDetails(
			errors.New("httpStream.ReadObject: empty request body"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	if err := json.Unmarshal(data, v); err != nil {
		// Add method context to Wrap message
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "httpStream.ReadObject: failed to unmarshal JSON"),
			cgerr.CategoryRPC,
			cgerr.CodeParseError,
			map[string]interface{}{
				"data_size":   len(data),
				"target_type": fmt.Sprintf("%T", v),
				// Consider adding a snippet of the invalid JSON? (Risk of logging sensitive data)
				// "data_snippet": string(data[:min(len(data), 100)]),
			},
		)
	}

	return nil
}

// Close closes the stream (specifically the request body reader).
func (s *httpStream) Close() error {
	logger.Debug("Closing HTTP stream")
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.reader.Close()
	if err != nil {
		logger.Error("Error closing HTTP stream reader", "error", fmt.Sprintf("%+v", err))
		// Return the error from closing the reader
		return errors.Wrap(err, "httpStream.Close: failed to close underlying reader")
	}
	return nil
}

// Helper function to check if headers have been written (simple heuristic)
func headersWritten(w http.ResponseWriter) bool {
	// Accessing ResponseWriter internal state is fragile.
	// This is a placeholder; a wrapper is better.
	// For now, assume they might have been written if Content-Type is set.
	return w.Header().Get("Content-Type") != ""
}
