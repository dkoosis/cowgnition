// internal/jsonrpc/adapter.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcperror"
	"github.com/sourcegraph/jsonrpc2"
)

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
		a.requestTimeout = timeout
	}
}

// NewAdapter creates a new JSON-RPC adapter with the provided options.
func NewAdapter(opts ...AdapterOption) *Adapter {
	a := &Adapter{
		handlers:       make(map[string]Handler),
		requestTimeout: DefaultTimeout,
	}

	// Apply options
	for _, opt := range opts {
		opt(a)
	}

	return a
}

// RegisterHandler registers a handler function for a specific method.
func (a *Adapter) RegisterHandler(method string, handler Handler) {
	a.handlers[method] = handler
}

// Handle implements the jsonrpc2.Handler interface.
func (a *Adapter) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	// Skip notifications if the method isn't found
	if !req.Notif {
		handler, ok := a.getHandler(ctx, conn, req)
		if !ok {
			return
		}

		// Execute the handler in a goroutine with timeout
		a.executeHandler(ctx, conn, req, handler)
	} else if handler, ok := a.handlers[req.Method]; ok {
		// Handle notification if the method exists
		a.handleNotification(ctx, handler, req)
	}
}

// getHandler returns the handler for the requested method or sends a method not found error.
func (a *Adapter) getHandler(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (Handler, bool) {
	handler, ok := a.handlers[req.Method]
	if !ok {
		// Create a method not found error with the method name as a property
		properties := map[string]interface{}{
			"request_id": req.ID,
		}
		methodError := mcperror.NewMethodNotFoundError(req.Method, properties)

		if err := a.sendErrorResponse(ctx, conn, req, methodError); err != nil {
			// Log the full error with stack trace for server-side debugging
			log.Printf("jsonrpc.Adapter: error sending MethodNotFound error: %+v", err)
		}
		return nil, false
	}
	return handler, true
}

// executeHandler handles a regular request with a timeout.
func (a *Adapter) executeHandler(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, handler Handler) {
	// Create a timeout context for the request
	timeoutCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel()

	// Create a channel for the result
	resultCh := make(chan struct {
		result interface{}
		err    error
	}, 1)

	// Execute handler in a goroutine
	go a.runHandler(timeoutCtx, req, handler, resultCh)

	// Wait for result or timeout
	select {
	case <-timeoutCtx.Done():
		a.handleTimeout(ctx, conn, req, timeoutCtx)
	case res := <-resultCh:
		a.processResult(ctx, conn, req, res.result, res.err)
	}
}

// handleNotification processes a notification without a response.
func (a *Adapter) handleNotification(ctx context.Context, handler Handler, req *jsonrpc2.Request) {
	var params json.RawMessage
	if req.Params != nil {
		params = *req.Params
	}

	// Execute handler without waiting for result
	go func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
		defer cancel()

		if _, err := handler(timeoutCtx, params); err != nil {
			// Log the full error with stack trace for server-side debugging
			log.Printf("jsonrpc.Adapter: error handling notification: %+v", err)
		}
	}()
}

// runHandler executes the handler and sends the result to the provided channel.
func (a *Adapter) runHandler(ctx context.Context, req *jsonrpc2.Request, handler Handler, resultCh chan<- struct {
	result interface{}
	err    error
}) {
	var params json.RawMessage
	if req.Params != nil {
		params = *req.Params
	}

	result, err := handler(ctx, params)
	resultCh <- struct {
		result interface{}
		err    error
	}{result, err}
}

// handleTimeout processes a timeout event.
func (a *Adapter) handleTimeout(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, timeoutCtx context.Context) {
	// Only handle timeout for request, not notification
	if req.Notif {
		return
	}

	// Check if the context was canceled due to timeout
	if timeoutCtx.Err() == context.DeadlineExceeded {
		properties := map[string]interface{}{
			"method":      req.Method,
			"timeout_sec": a.requestTimeout.Seconds(),
			"request_id":  req.ID,
		}

		timeoutErr := mcperror.NewTimeoutError(
			"Request timed out while executing handler",
			properties,
		)

		if err := a.sendErrorResponse(ctx, conn, req, timeoutErr); err != nil {
			log.Printf("jsonrpc.Adapter: error sending timeout error: %+v", err)
		}
	}
}

// processResult handles the result from a handler execution.
func (a *Adapter) processResult(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, result interface{}, err error) {
	if err != nil {
		if err := a.sendErrorResponse(ctx, conn, req, err); err != nil {
			log.Printf("jsonrpc.Adapter: error sending error response: %+v", err)
		}
		return
	}

	// Send the result for requests (not notifications)
	if !req.Notif {
		if err := conn.Reply(ctx, req.ID, result); err != nil {
			// Wrap the error with additional context
			wrappedErr := errors.Wrapf(err, "failed to send response for method %s", req.Method)
			wrappedErr = mcperror.ErrorWithDetails(wrappedErr, mcperror.CategoryRPC, mcperror.CodeInternalError,
				map[string]interface{}{
					"method":     req.Method,
					"request_id": req.ID,
				})
			log.Printf("jsonrpc.Adapter: error sending response: %+v", wrappedErr)
		}
	}
}

// sendErrorResponse converts an internal error to a JSON-RPC error and sends it.
func (a *Adapter) sendErrorResponse(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, err error) error {
	// Only respond to requests, not notifications
	if req.Notif {
		return nil
	}

	// Get error code and prepare the client-safe message
	code := mcperror.GetErrorCode(err)
	message := mcperror.UserFacingMessage(code)

	// Extract properties that are safe to expose to clients
	properties := mcperror.GetErrorProperties(err)
	safeProps := make(map[string]interface{})

	// Only include safe properties in the error data
	for k, v := range properties {
		// Exclude internal properties and possibly sensitive data
		if k != "category" && k != "code" && k != "stack" &&
			!containsSensitiveKeyword(k) {
			safeProps[k] = v
		}
	}

	// Create JSON-RPC error object
	rpcErr := &jsonrpc2.Error{
		Code:    int64(code),
		Message: message,
	}

	// Add data field if we have safe properties to include
	if len(safeProps) > 0 {
		dataJSON, marshalErr := json.Marshal(safeProps)
		if marshalErr == nil {
			rpcErr.Data = (*json.RawMessage)(&dataJSON)
		}
	}

	// Log the full error with all details for server-side debugging
	// The %+v format includes stack traces provided by cockroachdb/errors
	log.Printf("JSON-RPC error: %+v", err)

	// Send the sanitized error response to the client
	return conn.ReplyWithError(ctx, req.ID, rpcErr)
}

// Helper function to check if a string might contain sensitive information
func containsSensitiveKeyword(key string) bool {
	sensitiveKeywords := []string{"token", "password", "secret", "key", "auth", "credential"}
	for _, keyword := range sensitiveKeywords {
		if key == keyword {
			return true
		}
	}
	return false
}

// NewInvalidParamsError creates a new InvalidParams error with properties.
func NewInvalidParamsError(details string, properties map[string]interface{}) error {
	return mcperror.NewInvalidArgumentsError(details, properties)
}

// NewInternalError creates a new InternalError with properties.
func NewInternalError(err error, properties map[string]interface{}) error {
	wrappedErr := errors.Wrapf(err, "internal server error")
	return mcperror.ErrorWithDetails(wrappedErr, mcperror.CategoryRPC, mcperror.CodeInternalError, properties)
}
