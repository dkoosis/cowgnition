// Package jsonrpc implements the JSON-RPC 2.0, a simple protocol for remote procedure calls.
// file: internal/jsonrpc/types.go
package jsonrpc

import (
	"encoding/json"
	"fmt"
)

const (
	// Version is the JSON-RPC version string.
	Version = "2.0"
)

// Error represents a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Message represents a JSON-RPC message.
// It can be either a Request, Response, or Notification.
type Message struct {
	// Common fields for all message types
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Request represents a JSON-RPC request message.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Notification represents a JSON-RPC notification message.
// Notifications do not expect a response.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RequestHandler is a function that handles a JSON-RPC request.
type RequestHandler func(req *Request) (interface{}, error)

// NotificationHandler is a function that handles a JSON-RPC notification.
type NotificationHandler func(notif *Notification) error

// IsRequest returns true if the message is a request.
func (m *Message) IsRequest() bool {
	return m.Method != "" && m.ID != nil && m.Result == nil && m.Error == nil
}

// IsResponse returns true if the message is a response.
func (m *Message) IsResponse() bool {
	return m.Method == "" && m.ID != nil && (m.Result != nil || m.Error != nil)
}

// IsNotification returns true if the message is a notification.
func (m *Message) IsNotification() bool {
	return m.Method != "" && m.ID == nil && m.Result == nil && m.Error == nil
}

// ToRequest converts the message to a Request if it is a request, otherwise returns an error.
func (m *Message) ToRequest() (*Request, error) {
	if !m.IsRequest() {
		return nil, fmt.Errorf("message is not a request")
	}
	return &Request{
		JSONRPC: m.JSONRPC,
		ID:      m.ID,
		Method:  m.Method,
		Params:  m.Params,
	}, nil
}

// ToResponse converts the message to a Response if it is a response, otherwise returns an error.
func (m *Message) ToResponse() (*Response, error) {
	if !m.IsResponse() {
		return nil, fmt.Errorf("message is not a response")
	}
	return &Response{
		JSONRPC: m.JSONRPC,
		ID:      m.ID,
		Result:  m.Result,
		Error:   m.Error,
	}, nil
}

// ToNotification converts the message to a Notification if it is a notification, otherwise returns an error.
func (m *Message) ToNotification() (*Notification, error) {
	if !m.IsNotification() {
		return nil, fmt.Errorf("message is not a notification")
	}
	return &Notification{
		JSONRPC: m.JSONRPC,
		Method:  m.Method,
		Params:  m.Params,
	}, nil
}

// NewRequest creates a new JSON-RPC request.
func NewRequest(id interface{}, method string, params interface{}) (*Request, error) {
	var idJSON, paramsJSON json.RawMessage
	var err error

	if id != nil {
		idJSON, err = json.Marshal(id)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal ID: %w", err)
		}
	}

	if params != nil {
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	return &Request{
		JSONRPC: Version,
		ID:      idJSON,
		Method:  method,
		Params:  paramsJSON,
	}, nil
}

// NewResponse creates a new JSON-RPC response.
func NewResponse(id json.RawMessage, result interface{}, err *Error) (*Response, error) {
	var resultJSON json.RawMessage
	var marshalErr error

	if result != nil && err == nil {
		resultJSON, marshalErr = json.Marshal(result)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", marshalErr)
		}
	}

	return &Response{
		JSONRPC: Version,
		ID:      id,
		Result:  resultJSON,
		Error:   err,
	}, nil
}

// NewNotification creates a new JSON-RPC notification.
func NewNotification(method string, params interface{}) (*Notification, error) {
	var paramsJSON json.RawMessage
	var err error

	if params != nil {
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	return &Notification{
		JSONRPC: Version,
		Method:  method,
		Params:  paramsJSON,
	}, nil
}

// ParseParams parses the params of a request or notification into the specified struct.
func (r *Request) ParseParams(dst interface{}) error {
	if r.Params == nil {
		return nil
	}
	if err := json.Unmarshal(r.Params, dst); err != nil {
		return fmt.Errorf("failed to unmarshal params: %w", err)
	}
	return nil
}

// ParseParams parses the params of a notification into the specified struct.
func (n *Notification) ParseParams(dst interface{}) error {
	if n.Params == nil {
		return nil
	}
	if err := json.Unmarshal(n.Params, dst); err != nil {
		return fmt.Errorf("failed to unmarshal params: %w", err)
	}
	return nil
}

// GetID returns the ID as an interface{} (string or number).
func (r *Request) GetID() (interface{}, error) {
	var id interface{}
	err := json.Unmarshal(r.ID, &id)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal ID: %w", err)
	}
	return id, nil
}
