// internal/jsonrpc/adapter.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"errors" // Added for errors.As
	"fmt"
	"log"
	"time"

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
		// Method not found error
		err := &jsonrpc2.Error{
			Code:    jsonrpc2.CodeMethodNotFound,
			Message: fmt.Sprintf("method %q not found", req.Method),
		}
		if err := conn.ReplyWithError(ctx, req.ID, err); err != nil {
			log.Printf("jsonrpc.Adapter: error sending MethodNotFound error: %v", err)
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
			log.Printf("jsonrpc.Adapter: error handling notification: %v", err)
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
		timeoutErr := &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInternalError,
			Message: "request timed out",
		}
		if err := conn.ReplyWithError(ctx, req.ID, timeoutErr); err != nil {
			log.Printf("jsonrpc.Adapter: error sending timeout error: %v", err)
		}
	}
}

// processResult handles the result from a handler execution.
func (a *Adapter) processResult(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, result interface{}, err error) {
	if err != nil {
		a.sendErrorResponse(ctx, conn, req, err)
		return
	}

	// Send the result for requests (not notifications)
	if !req.Notif {
		if err := conn.Reply(ctx, req.ID, result); err != nil {
			log.Printf("jsonrpc.Adapter: error sending response: %v", err)
		}
	}
}

// sendErrorResponse converts an error to a JSON-RPC error and sends it.
func (a *Adapter) sendErrorResponse(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, err error) {
	// Only respond to requests, not notifications
	if req.Notif {
		return
	}

	// Convert error to JSON-RPC error
	var rpcErr *jsonrpc2.Error
	if errors.As(err, &rpcErr) {
		// Already a JSON-RPC error, use it directly
	} else {
		rpcErr = &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInternalError,
			Message: err.Error(),
		}
	}

	if replyErr := conn.ReplyWithError(ctx, req.ID, rpcErr); replyErr != nil {
		log.Printf("jsonrpc.Adapter: error sending error response: %v", replyErr)
	}
}

// NewMethodNotFoundError creates a new MethodNotFound error.
func NewMethodNotFoundError(method string) *jsonrpc2.Error {
	return &jsonrpc2.Error{
		Code:    jsonrpc2.CodeMethodNotFound,
		Message: fmt.Sprintf("method %q not found", method),
	}
}

// NewInvalidParamsError creates a new InvalidParams error.
func NewInvalidParamsError(details string) *jsonrpc2.Error {
	var data json.RawMessage
	data, _ = json.Marshal(details)

	return &jsonrpc2.Error{
		Code:    jsonrpc2.CodeInvalidParams,
		Message: "invalid params",
		Data:    &data, // Use pointer to the data
	}
}

// NewInternalError creates a new InternalError.
func NewInternalError(err error) *jsonrpc2.Error {
	var data json.RawMessage
	data, _ = json.Marshal(err.Error())

	return &jsonrpc2.Error{
		Code:    jsonrpc2.CodeInternalError,
		Message: "internal error",
		Data:    &data, // Use pointer to the data
	}
}

// NewTimeoutError creates a new TimeoutError.
func NewTimeoutError() *jsonrpc2.Error {
	return &jsonrpc2.Error{
		Code:    jsonrpc2.CodeInternalError,
		Message: "request timed out",
	}
}
