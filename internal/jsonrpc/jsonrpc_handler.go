// internal/jsonrpc/jsonrpc_handler.go
// internal/jsonrpc/adapter.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"      // Import fmt
	"log/slog" // Import slog
	"strings"  // Import strings
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// Initialize the logger at the package level
var logger = logging.GetLogger("jsonrpc_adapter")

// DefaultTimeout defines the default timeout duration for JSON-RPC requests.
const DefaultTimeout = 30 * time.Second

// Handler is a function that handles a JSON-RPC method call.
type Handler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// Adapter wraps the sourcegraph/jsonrpc2 library to provide JSON-RPC 2.0
// functionality for the MCP server.
type Adapter struct {
	handlers       map[string]Handler
	requestTimeout time.Duration
}

// AdapterOption defines a function that configures an Adapter.
type AdapterOption func(*Adapter)

// WithTimeout sets the request timeout duration for the adapter.
func WithTimeout(timeout time.Duration) AdapterOption {
	return func(a *Adapter) {
		if timeout > 0 {
			a.requestTimeout = timeout
			logger.Debug("Adapter request timeout set", "timeout", timeout)
		} else {
			logger.Warn("Ignoring invalid timeout value", "invalid_timeout", timeout)
		}
	}
}

// NewAdapter creates a new JSON-RPC adapter with the provided options.
func NewAdapter(opts ...AdapterOption) *Adapter {
	a := &Adapter{
		handlers:       make(map[string]Handler),
		requestTimeout: DefaultTimeout,
	}
	logger.Debug("Initializing new JSON-RPC Adapter", "default_timeout", DefaultTimeout)

	// Apply options
	for _, opt := range opts {
		opt(a)
	}
	logger.Debug("Adapter options applied", "final_timeout", a.requestTimeout, "handler_count", len(a.handlers))

	return a
}

// RegisterHandler registers a handler function for a specific method.
func (a *Adapter) RegisterHandler(method string, handler Handler) {
	if method == "" || handler == nil {
		logger.Error("Attempted to register handler with empty method or nil handler", "method", method, "handler_is_nil", handler == nil)
		return
	}
	logger.Info("Registering JSON-RPC handler", "method", method)
	a.handlers[method] = handler
}

// Handle implements the jsonrpc2.Handler interface.
// It determines if the request is a notification or a standard request and routes accordingly.
func (a *Adapter) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	methodLogger := logger.With("method", req.Method, "req_id", req.ID, "is_notification", req.Notif)
	methodLogger.Debug("Handling incoming JSON-RPC request")

	if !req.Notif {
		// It's a request requiring a response
		handler, ok := a.getHandler(ctx, conn, req, methodLogger) // Pass logger down
		if !ok {
			// Error response already sent by getHandler
			methodLogger.Warn("Handler not found for request")
			return
		}
		// Execute the handler with timeout logic
		a.executeHandler(ctx, conn, req, handler, methodLogger) // Pass logger down
	} else {
		// It's a notification, no response needed
		if handler, ok := a.handlers[req.Method]; ok {
			// Handle notification if the method exists
			methodLogger.Debug("Handling notification")
			a.handleNotification(ctx, handler, req, methodLogger) // Pass logger down
		} else {
			// Optional: Log unknown notification methods if desired
			methodLogger.Debug("Ignoring notification for unknown method")
		}
	}
}

// getHandler finds the handler or sends a MethodNotFound error.
func (a *Adapter) getHandler(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, methodLogger *slog.Logger) (Handler, bool) {
	handler, ok := a.handlers[req.Method]
	if !ok {
		methodLogger.Warn("Method not found")
		properties := map[string]interface{}{
			"request_id": req.ID, // Keep ID for correlation
		}
		// Use the cgerr helper, which creates a categorized error
		methodNotFoundError := cgerr.NewMethodNotFoundError(req.Method, properties)

		if err := a.sendErrorResponse(ctx, conn, req, methodNotFoundError, methodLogger); err != nil {
			// Replace log.Printf with logger.Error (Conceptual L63)
			methodLogger.Error("Error sending MethodNotFound response", "send_error", fmt.Sprintf("%+v", err), "original_error", fmt.Sprintf("%+v", methodNotFoundError))
		}
		return nil, false // Indicate handler not found
	}
	methodLogger.Debug("Handler found")
	return handler, true
}

// executeHandler runs a request handler with timeout logic.
func (a *Adapter) executeHandler(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, handler Handler, methodLogger *slog.Logger) {
	methodLogger.Debug("Executing handler", "timeout", a.requestTimeout)
	// Create a timeout context for the specific request handler execution
	timeoutCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel() // Ensure cancellation signal propagates

	// Channel to receive the result or error from the handler goroutine
	resultCh := make(chan struct {
		result interface{}
		err    error
	}, 1) // Buffered channel of size 1

	// Execute the actual handler logic in a separate goroutine
	go a.runHandler(timeoutCtx, req, handler, resultCh, methodLogger)

	// Wait for either the handler to complete or the timeout to occur
	select {
	case <-timeoutCtx.Done():
		// Timeout occurred or parent context cancelled
		methodLogger.Warn("Context done while waiting for handler result", "context_error", timeoutCtx.Err())
		a.handleTimeout(ctx, conn, req, timeoutCtx, methodLogger) // Pass logger down

	case res := <-resultCh:
		// Handler completed within the timeout
		methodLogger.Debug("Handler finished execution", "has_error", res.err != nil)
		a.processResult(ctx, conn, req, res.result, res.err, methodLogger) // Pass logger down
	}
}

// handleNotification executes a notification handler (fire-and-forget).
func (a *Adapter) handleNotification(ctx context.Context, handler Handler, req *jsonrpc2.Request, methodLogger *slog.Logger) {
	var params json.RawMessage // Handle nil params gracefully
	if req.Params != nil {
		params = *req.Params
	}

	// Execute the handler in a goroutine as notifications don't block
	go func() {
		// Create a timeout context specific to this notification handler
		timeoutCtx, cancel := context.WithTimeout(context.Background(), a.requestTimeout) // Use Background context for notifications
		defer cancel()

		methodLogger.Debug("Running notification handler")
		if _, err := handler(timeoutCtx, params); err != nil {
			// Log any error from the notification handler
			// Replace log.Printf with logger.Error (Conceptual L108)
			methodLogger.Error("Error executing notification handler", "error", fmt.Sprintf("%+v", err))
		} else {
			methodLogger.Debug("Notification handler completed")
		}
	}()
}

// runHandler is the goroutine function that executes the registered handler.
func (a *Adapter) runHandler(ctx context.Context, req *jsonrpc2.Request, handler Handler, resultCh chan<- struct {
	result interface{}
	err    error
}, methodLogger *slog.Logger) {

	var params json.RawMessage
	if req.Params != nil {
		params = *req.Params
	}

	// Defer a panic handler
	defer func() {
		if r := recover(); r != nil {
			panicErr := errors.Errorf("panic recovered in JSON-RPC handler for method '%s': %v", req.Method, r)
			panicErr = errors.WithStack(panicErr) // Add stack trace to the panic error
			methodLogger.Error("Panic recovered during handler execution", "panic_value", r, "error", fmt.Sprintf("%+v", panicErr))
			// Send the panic error back through the channel
			resultCh <- struct {
				result interface{}
				err    error
			}{nil, cgerr.NewInternalError(panicErr, map[string]interface{}{"method": req.Method})}
		}
	}()

	methodLogger.Debug("Calling registered handler function")
	result, err := handler(ctx, params) // Execute the actual handler code

	// Send the result (or error) back to the waiting executeHandler function
	resultCh <- struct {
		result interface{}
		err    error
	}{result, err}
	methodLogger.Debug("Sent result/error back to executeHandler")
}

// handleTimeout sends a timeout error response if appropriate.
func (a *Adapter) handleTimeout(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, timeoutCtx context.Context, methodLogger *slog.Logger) {
	// Don't send errors for notifications
	if req.Notif {
		methodLogger.Debug("Timeout occurred for notification, no response sent")
		return
	}

	// Check if the context timed out specifically (vs parent cancellation)
	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		methodLogger.Warn("Request handler timed out")
		properties := map[string]interface{}{
			"method":      req.Method,
			"timeout_sec": a.requestTimeout.Seconds(),
			"request_id":  req.ID, // Include ID for correlation
		}
		// Use the cgerr helper
		timeoutErr := cgerr.NewTimeoutError(
			fmt.Sprintf("Handler execution exceeded timeout (%s)", a.requestTimeout),
			properties,
		)

		if err := a.sendErrorResponse(ctx, conn, req, timeoutErr, methodLogger); err != nil {
			// Replace log.Printf with logger.Error (Conceptual L84)
			methodLogger.Error("Error sending timeout response", "send_error", fmt.Sprintf("%+v", err), "original_error", fmt.Sprintf("%+v", timeoutErr))
		}
	} else {
		// Context was canceled for a reason other than timeout (e.g., client disconnected)
		methodLogger.Info("Handler context cancelled", "reason", timeoutCtx.Err())
		// Optionally, send a generic error or just log
	}
}

// processResult sends the success or error response back to the client.
func (a *Adapter) processResult(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, result interface{}, handlerErr error, methodLogger *slog.Logger) {
	// If the handler returned an error, send an error response
	if handlerErr != nil {
		methodLogger.Warn("Handler returned an error", "handler_error", fmt.Sprintf("%+v", handlerErr))
		if err := a.sendErrorResponse(ctx, conn, req, handlerErr, methodLogger); err != nil {
			// Log failure to send the error response itself
			// Replace log.Printf with logger.Error (Conceptual L148)
			methodLogger.Error("Error sending handler error response", "send_error", fmt.Sprintf("%+v", err), "original_handler_error", fmt.Sprintf("%+v", handlerErr))
		}
		return
	}

	// If it was a request (not notification), send the successful result
	if !req.Notif {
		methodLogger.Debug("Sending successful response")
		if err := conn.Reply(ctx, req.ID, result); err != nil {
			// Failed to send the success response - this is an internal/transport error
			// Add function context to Wrapf message
			wrappedErr := errors.Wrapf(err, "Adapter.processResult: failed to send successful response for method %s", req.Method)
			// Add cgerr details for categorization
			detailedErr := cgerr.ErrorWithDetails(wrappedErr, cgerr.CategoryRPC, cgerr.CodeInternalError,
				map[string]interface{}{
					"method":     req.Method,
					"request_id": req.ID,
				})
			// Replace log.Printf with logger.Error (Conceptual L165)
			methodLogger.Error("Error sending successful response", "error", fmt.Sprintf("%+v", detailedErr))
		} else {
			methodLogger.Info("Successfully sent response")
		}
	} else {
		// Should not happen for notifications if logic is correct, but log if it does
		methodLogger.Warn("processResult called for a notification, which should not happen")
	}
}

// sendErrorResponse sanitizes and sends a JSON-RPC error response.
func (a *Adapter) sendErrorResponse(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, originalErr error, methodLogger *slog.Logger) error {
	// Don't send responses for notifications
	if req.Notif {
		methodLogger.Debug("Attempted to send error for notification, skipping.", "original_error_type", fmt.Sprintf("%T", originalErr))
		return nil
	}

	// Log the full, original error with stack trace etc. for server-side debugging *first*.
	// This is the most important log call for capturing internal errors.
	// Replace log.Printf with logger.Error (Conceptual L204)
	methodLogger.Error("Sending error response", "full_error", fmt.Sprintf("%+v", originalErr))

	// Convert the internal error to a client-safe JSON-RPC error object
	rpcErr := cgerr.ToJSONRPCError(originalErr)

	// Add request ID if available (helps client correlate)
	if req.ID != nil {
		rpcErr.ID = req.ID // Set the ID on the jsonrpc2.Error struct itself
	}

	methodLogger.Debug("Sending sanitized error to client", "rpc_error_code", rpcErr.Code, "rpc_error_message", rpcErr.Message)
	// Send the sanitized error response to the client using ReplyWithError
	sendErr := conn.ReplyWithError(ctx, req.ID, rpcErr) // ReplyWithError uses the ID internally too
	if sendErr != nil {
		// Log error during the *sending* of the error response
		sendErr = errors.Wrapf(sendErr, "Adapter.sendErrorResponse: failed to send error reply for method %s", req.Method)
		methodLogger.Error("Failed to send error reply via connection", "send_error", fmt.Sprintf("%+v", sendErr), "original_error_type", fmt.Sprintf("%T", originalErr))
	}

	return sendErr // Return error related to sending, not the original error
}

// Helper function to check if a string might contain sensitive information (remains the same)
func containsSensitiveKeyword(key string) bool {
	// Use lowercase comparison for broader matching
	lowerKey := strings.ToLower(key)
	sensitiveKeywords := []string{"token", "password", "secret", "key", "auth", "credential", "session"} // Add more if needed
	for _, keyword := range sensitiveKeywords {
		if strings.Contains(lowerKey, keyword) {
			return true
		}
	}
	return false
}

// NewInvalidParamsError creates a new InvalidParams error with properties.
// Consider moving this to cgerr package if it's generally useful.
func NewInvalidParamsError(details string, properties map[string]interface{}) error {
	// Ensure the cgerr helper is used correctly
	return cgerr.NewInvalidArgumentsError(details, properties)
}

// NewInternalError creates a new InternalError with properties.
// Consider moving this to cgerr package if it's generally useful.
func NewInternalError(err error, properties map[string]interface{}) error {
	// Add function context to Wrapf message
	wrappedErr := errors.Wrapf(err, "jsonrpc.NewInternalError: internal server error occurred")
	// Ensure the cgerr helper is used correctly
	return cgerr.ErrorWithDetails(wrappedErr, cgerr.CategoryRPC, cgerr.CodeInternalError, properties)
}
