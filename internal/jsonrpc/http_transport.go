// file: internal/jsonrpc/http_transport.go
// Package jsonrpc provides implementations for JSON-RPC 2.0 communication,
// including transport mechanisms like HTTP.
package jsonrpc

import (
	"context"
	"encoding/json" // For marshaling and unmarshaling JSON-RPC messages.
	"fmt"           // For formatting log messages and errors.
	"io"            // For io.ReadCloser, io.ReadAll, io.EOF.
	"net/http"      // For HTTP server implementation (Handler, Request, ResponseWriter).
	"time"          // For request timeouts.

	"github.com/cockroachdb/errors"                           // Error handling library.
	"github.com/dkoosis/cowgnition/internal/logging"          // Project's structured logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // Project's custom error types.
	"github.com/sourcegraph/jsonrpc2"                         // Core JSON-RPC 2.0 library.
)

// httpTransportLogger initializes the structured logger for the jsonrpc HTTP transport layer.
var httpTransportLogger = logging.GetLogger("jsonrpc_http_transport")

// HTTPHandler implements an http.Handler that serves JSON-RPC 2.0 requests
// over HTTP, using the sourcegraph/jsonrpc2 library for protocol handling.
type HTTPHandler struct {
	// handler is the core jsonrpc2.Handler that processes incoming requests.
	handler jsonrpc2.Handler
	// requestTimeout is the maximum duration allowed for processing a single HTTP request,
	// including JSON-RPC request handling.
	requestTimeout time.Duration
	// shutdownTimeout is the duration allowed for graceful shutdown.
	// Note: This is currently configured but not actively used in ServeHTTP's shutdown logic.
	// Consider integrating it with http.Server's Shutdown method if applicable.
	shutdownTimeout time.Duration
}

// HTTPHandlerOption defines a function type used for configuring an HTTPHandler
// instance upon creation using the Option pattern.
type HTTPHandlerOption func(*HTTPHandler)

// WithHTTPRequestTimeout returns an HTTPHandlerOption that sets the request processing timeout.
// If the provided timeout is zero or negative, a warning is logged, and the default is kept.
func WithHTTPRequestTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if timeout > 0 {
			h.requestTimeout = timeout
			// Debug log for applied setting (can be noisy).
			// httpTransportLogger.Debug("HTTP request timeout set.", "timeout", timeout)
		} else {
			httpTransportLogger.Warn("Ignoring invalid (non-positive) HTTP request timeout value.", "invalid_timeout", timeout)
		}
	}
}

// WithHTTPShutdownTimeout returns an HTTPHandlerOption that sets the graceful shutdown timeout.
// If the provided timeout is zero or negative, a warning is logged, and the default is kept.
// Note: This timeout's usage depends on the server's shutdown implementation.
func WithHTTPShutdownTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if timeout > 0 {
			h.shutdownTimeout = timeout
			// httpTransportLogger.Debug("HTTP shutdown timeout set.", "timeout", timeout)
		} else {
			httpTransportLogger.Warn("Ignoring invalid (non-positive) HTTP shutdown timeout value.", "invalid_timeout", timeout)
		}
	}
}

// NewHTTPHandler creates a new HTTPHandler instance for serving JSON-RPC requests.
// It takes a jsonrpc2.Handler to process requests and optional configuration functions.
func NewHTTPHandler(handler jsonrpc2.Handler, opts ...HTTPHandlerOption) *HTTPHandler {
	// Initialize handler with default timeouts.
	h := &HTTPHandler{
		handler:         handler,
		requestTimeout:  DefaultTimeout,  // Default defined in this package.
		shutdownTimeout: 5 * time.Second, // Sensible default for shutdown.
	}
	httpTransportLogger.Debug("Initializing new JSON-RPC HTTP Handler.", "default_request_timeout", h.requestTimeout, "default_shutdown_timeout", h.shutdownTimeout)

	// Apply any provided options to override defaults.
	for _, opt := range opts {
		opt(h)
	}
	httpTransportLogger.Info("JSON-RPC HTTP Handler configured.", "final_request_timeout", h.requestTimeout, "final_shutdown_timeout", h.shutdownTimeout)

	return h
}

// ServeHTTP implements the standard http.Handler interface. It handles incoming HTTP requests,
// validates the method, sets up a JSON-RPC connection over an HTTP stream,
// manages request timeouts, and delegates processing to the underlying jsonrpc2.Handler.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create a logger instance with request-specific context.
	requestLogger := httpTransportLogger.With("method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
	requestLogger.Debug("Handling incoming HTTP request.")

	// Enforce POST method for JSON-RPC requests according to common practice.
	if r.Method != http.MethodPost {
		methodErr := cgerr.ErrorWithDetails(
			errors.Newf("JSON-RPC requires POST method, received %s.", r.Method),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest, // Mapping: Incorrect HTTP method is an invalid request structure.
			map[string]interface{}{
				"allowed_method": http.MethodPost,
				"actual_method":  r.Method,
			},
		)
		requestLogger.Warn("HTTP method not allowed.", "error", fmt.Sprintf("%+v", methodErr))
		// Respond with standard HTTP error.
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Create a request context with a deadline based on the configured timeout.
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	// Ensure cancel is called eventually to release resources associated with the context.
	defer cancel()

	// Create an adapter that implements jsonrpc2.ObjectStream using the HTTP request body and response writer.
	stream := &httpStream{
		reader: r.Body,
		writer: w,
		// closed defaults to false.
	}

	// Establish a new JSON-RPC connection over the HTTP stream.
	// The connection uses the request's cancellable context and the provided handler.
	conn := jsonrpc2.NewConn(ctx, stream, h.handler)
	requestLogger.Debug("JSON-RPC connection created over HTTP stream.")

	// Block until the connection is closed (either normally or due to context cancellation/timeout).
	select {
	case <-conn.DisconnectNotify():
		// Connection closed normally. This could be after sending a response or due to an internal error handled by jsonrpc2.
		requestLogger.Debug("JSON-RPC connection disconnected normally.")
	case <-ctx.Done():
		// The request context finished before the connection disconnected.
		requestLogger.Warn("HTTP request context done before connection disconnect.", "reason", ctx.Err())
		// Check if the context finished specifically due to the timeout.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Create a specific timeout error conforming to project standards.
			timeoutErr := cgerr.NewTimeoutError(
				fmt.Sprintf("JSON-RPC request timed out after %s.", h.requestTimeout),
				map[string]interface{}{
					"timeout_duration": h.requestTimeout.String(),
				},
			)
			requestLogger.Error("HTTP request processing timed out.", "error", fmt.Sprintf("%+v", timeoutErr))

			// Attempt to send a JSON-RPC error response indicating the timeout.
			// This is best-effort, as headers might have already been written by the jsonrpc2 library
			// if the request processing started but didn't finish in time.
			if !headersWritten(w) { // Use heuristic check to avoid duplicate WriteHeader calls.
				w.Header().Set("Content-Type", "application/json")
				// Use 504 Gateway Timeout for request processing timeouts.
				w.WriteHeader(http.StatusGatewayTimeout)
			} else {
				requestLogger.Warn("Cannot set timeout headers/status; headers already written.")
			}

			// Convert the application timeout error to a standard JSON-RPC error object.
			rpcErr := cgerr.ToJSONRPCError(timeoutErr)
			// Marshal the JSON-RPC error response.
			errBody, marshalErr := json.Marshal(rpcErr)
			if marshalErr != nil {
				// Log failure to marshal the error, can't send it to client.
				requestLogger.Error("Failed to marshal timeout JSON-RPC error response body.", "marshal_error", fmt.Sprintf("%+v", marshalErr))
				return // Exit, nothing more can be sent.
			}

			// Attempt to write the marshaled error body to the response writer.
			_, writeErr := w.Write(errBody)
			if writeErr != nil {
				// Log failure to write the error body.
				writeErrWithDetails := cgerr.ErrorWithDetails(
					errors.Wrap(writeErr, "failed to write timeout error response body"),
					cgerr.CategoryRPC,
					cgerr.CodeInternalError,
					map[string]interface{}{
						"original_error_code": rpcErr.Code,
					},
				)
				requestLogger.Error("Failed to write timeout JSON-RPC error response.", "write_error", fmt.Sprintf("%+v", writeErrWithDetails))
			}
		}
		// Could potentially handle other ctx.Err() cases like context.Canceled if needed.
	}
	requestLogger.Debug("Finished handling HTTP request.")
}

// httpStream adapts an HTTP request/response pair (io.ReadCloser for request body,
// http.ResponseWriter for response) to the jsonrpc2.ObjectStream interface, required by jsonrpc2.Conn.
type httpStream struct {
	reader io.ReadCloser       // Reads from the HTTP request body.
	writer http.ResponseWriter // Writes to the HTTP response body.
	closed bool                // Flag indicating if the stream has been closed.
}

// WriteObject marshals the given object (typically a jsonrpc2.Request or jsonrpc2.Response)
// to JSON and writes it to the HTTP response writer. It sets the Content-Type header.
// Returns an error if marshaling or writing fails, or if the stream is already closed.
func (s *httpStream) WriteObject(obj interface{}) error {
	if s.closed {
		return cgerr.ErrorWithDetails(
			errors.New("write attempt on closed httpStream."),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"object_type": fmt.Sprintf("%T", obj),
			},
		)
	}

	// Marshal the JSON-RPC message object to bytes.
	data, err := json.Marshal(obj)
	if err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to marshal JSON-RPC object for HTTP response."),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError, // Marshaling failure is an internal server error.
			map[string]interface{}{
				"object_type": fmt.Sprintf("%T", obj),
			},
		)
	}

	// Ensure the Content-Type header is set. This might be redundant if headers
	// were already written, but setting it here ensures it's attempted before the first write.
	// Note: http.ResponseWriter implicitly calls WriteHeader(http.StatusOK) on the first Write
	// if WriteHeader hasn't been called explicitly. Setting headers after the first write is a no-op.
	s.writer.Header().Set("Content-Type", "application/json")

	// Write the JSON data to the HTTP response.
	_, err = s.writer.Write(data)
	if err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to write JSON-RPC response to HTTP stream."),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError, // Failure to write response is an internal server error.
			map[string]interface{}{
				"data_size": len(data),
			},
		)
	}

	// Consider flushing the writer if intermediate responses are needed (e.g., for streaming results).
	// For typical request/response, flushing might not be necessary as Close() or request end handles it.
	// if f, ok := s.writer.(http.Flusher); ok {
	//  f.Flush()
	// }

	return nil
}

// ReadObject reads the entire HTTP request body, unmarshals it as JSON into the provided
// interface `v` (typically a *jsonrpc2.Request).
// Returns io.EOF if the stream is closed normally during a read attempt.
// Returns an error if reading, unmarshaling fails, the body is empty, or the stream was already closed.
func (s *httpStream) ReadObject(v interface{}) error {
	if s.closed {
		// Check if closed explicitly before attempting read.
		httpTransportLogger.Debug("httpStream.ReadObject: Read attempt on explicitly closed stream, returning io.EOF.")
		return io.EOF // Return io.EOF consistent with stream closure expectations.
	}

	// Read the entire request body.
	data, err := io.ReadAll(s.reader)

	// Check for errors during read. Specifically handle the case where Close() might have been called concurrently.
	if err != nil {
		// If the stream was closed while ReadAll was blocked, it might return http.ErrBodyReadAfterClose.
		// We map this condition to io.EOF as expected by jsonrpc2 for a closed stream.
		if s.closed && errors.Is(err, http.ErrBodyReadAfterClose) {
			httpTransportLogger.Debug("httpStream.ReadObject: Read after close detected, returning io.EOF.")
			return io.EOF
		}
		// For other read errors, wrap and return them.
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to read JSON-RPC request body from HTTP stream."),
			cgerr.CategoryRPC,
			cgerr.CodeParseError, // Error reading the request body implies a parsing/request formation issue.
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	// Check for an empty request body, which is usually invalid for JSON-RPC.
	if len(data) == 0 {
		httpTransportLogger.Warn("httpStream.ReadObject: Received empty request body.")
		// Consider if io.EOF is more appropriate, but InvalidRequest seems clearer.
		return cgerr.ErrorWithDetails(
			errors.New("received empty request body."),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"target_type": fmt.Sprintf("%T", v),
			},
		)
	}

	// Unmarshal the JSON data into the provided struct `v`.
	if err := json.Unmarshal(data, v); err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to unmarshal JSON-RPC request from HTTP body."),
			cgerr.CategoryRPC,
			cgerr.CodeParseError, // JSON unmarshaling failure is a classic ParseError.
			map[string]interface{}{
				"data_size":   len(data),
				"target_type": fmt.Sprintf("%T", v),
				// Avoid logging data snippet by default due to potential sensitive info. Enable if debugging carefully.
				// "data_snippet": string(data[:min(len(data), 100)]),
			},
		)
	}

	// Successfully read and unmarshaled the object.
	return nil
}

// Close marks the stream as closed and closes the underlying HTTP request body reader.
// It's essential to release resources associated with the request body.
// It returns the error from closing the reader, if any.
func (s *httpStream) Close() error {
	httpTransportLogger.Debug("Closing HTTP stream.")
	if s.closed {
		// Already closed, do nothing.
		return nil
	}
	// Mark as closed first to prevent race conditions with concurrent reads/writes.
	s.closed = true
	// Close the request body reader.
	err := s.reader.Close()
	if err != nil {
		httpTransportLogger.Error("Error closing HTTP stream reader (request body).", "error", fmt.Sprintf("%+v", err))
		// Wrap the error for context.
		return errors.Wrap(err, "httpStream.Close: failed to close underlying reader")
	}
	return nil
}

// headersWritten provides a **highly unreliable heuristic** to guess if HTTP headers
// have already been written to the ResponseWriter.
// WARNING: Accessing or relying on the internal state of http.ResponseWriter is fragile
// and breaks encapsulation. This implementation (checking Content-Type) is a poor guess
// and might be incorrect in many scenarios (e.g., if headers were written without Content-Type,
// or if Content-Type was set but headers not yet flushed).
// A robust solution requires a custom ResponseWriter wrapper that explicitly tracks
// whether WriteHeader or the first Write call has occurred. Use this function with extreme caution,
// primarily for optimistic checks where failure is gracefully handled (e.g., avoiding a superfluous WriteHeader call).
func headersWritten(w http.ResponseWriter) bool {
	// Very fragile check: assumes setting Content-Type implies headers might be written,
	// or that headers are written before Content-Type is typically set. Both assumptions can be wrong.
	return w.Header().Get("Content-Type") != ""
}

// Helper function for data snippet logging (example, not used by default).
// func min(a, b int) int {
//  if a < b {
//      return a
//  }
//  return b
// }
