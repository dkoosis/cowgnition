// internal/jsonrpc/adapter.go
package jsonrpc

import (
	"context"
	"encoding/json"
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
	handler, ok := a.handlers[req.Method]
	if !ok {
		// Only respond to requests, not notifications
		if !req.Notif {
			err := &jsonrpc2.Error{
				Code:    jsonrpc2.CodeMethodNotFound,
				Message: fmt.Sprintf("method %q not found", req.Method),
			}
			if err := conn.ReplyWithError(ctx, req.ID, err); err != nil {
				log.Printf("jsonrpc.Adapter: error sending MethodNotFound error: %v", err)
			}
		}
		return
	}

	// Create a timeout context for the request
	timeoutCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel()

	// Create a channel for the result
	resultCh := make(chan struct {
		result interface{}
		err    error
	}, 1)

	// Handle the request in a goroutine
	go func() {
		var params json.RawMessage
		if req.Params != nil {
			params = *req.Params
		}

		result, err := handler(timeoutCtx, params)
		resultCh <- struct {
			result interface{}
			err    error
		}{result, err}
	}()

	// Wait for result or timeout
	select {
	case <-timeoutCtx.Done():
		// Check if the context was canceled due to timeout
		if timeoutCtx.Err() == context.DeadlineExceeded {
			if !req.Notif {
				timeoutErr := &jsonrpc2.Error{
					Code:    jsonrpc2.CodeInternalError,
					Message: "request timed out",
				}
				if err := conn.ReplyWithError(ctx, req.ID, timeoutErr); err != nil {
					log.Printf("jsonrpc.Adapter: error sending timeout error: %v", err)
				}
			}
		}
	case res := <-resultCh:
		// Handle the result as before
		if res.err != nil {
			// Convert error to JSON-RPC error
			var rpcErr *jsonrpc2.Error
			if jsonErr, ok := res.err.(*jsonrpc2.Error); ok {
				rpcErr = jsonErr
			} else {
				rpcErr = &jsonrpc2.Error{
					Code:    jsonrpc2.CodeInternalError,
					Message: res.err.Error(),
				}
			}

			// Only respond to requests, not notifications
			if !req.Notif {
				if replyErr := conn.ReplyWithError(ctx, req.ID, rpcErr); replyErr != nil {
					log.Printf("jsonrpc.Adapter: error sending error response: %v", replyErr)
				}
			}
			return
		}

		// Send the result for requests (not notifications)
		if !req.Notif {
			if err := conn.Reply(ctx, req.ID, res.result); err != nil {
				log.Printf("jsonrpc.Adapter: error sending response: %v", err)
			}
		}
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
