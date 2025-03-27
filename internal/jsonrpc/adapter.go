// Package jsonrpc provides JSON-RPC 2.0 functionality for the MCP server.
// file: internal/jsonrpc/adapter.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/sourcegraph/jsonrpc2"
)

// Handler is a function that handles a JSON-RPC method call.
type Handler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// Adapter wraps the sourcegraph/jsonrpc2 library to provide JSON-RPC 2.0
// functionality for the MCP server.
type Adapter struct {
	handlers map[string]Handler
}

// NewAdapter creates a new JSON-RPC adapter.
func NewAdapter() *Adapter {
	return &Adapter{
		handlers: make(map[string]Handler),
	}
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

	// Handle the request
	var params json.RawMessage
	if req.Params != nil {
		params = *req.Params
	}

	result, err := handler(ctx, params)
	if err != nil {
		// Convert error to JSON-RPC error
		var rpcErr *jsonrpc2.Error
		if jsonErr, ok := err.(*jsonrpc2.Error); ok {
			rpcErr = jsonErr
		} else {
			rpcErr = &jsonrpc2.Error{
				Code:    jsonrpc2.CodeInternalError,
				Message: err.Error(),
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
		if err := conn.Reply(ctx, req.ID, result); err != nil {
			log.Printf("jsonrpc.Adapter: error sending response: %v", err)
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
