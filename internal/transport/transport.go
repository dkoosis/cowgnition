// file: internal/transport/transport.go
FIXME
// file: internal/transport/transport.go
package transport

import (
	"context"
	"encoding/json"
	"io"
)

// MaxMessageSize defines the maximum allowed size for a single JSON-RPC message in bytes.
// This helps prevent memory exhaustion attacks.
const MaxMessageSize = 1024 * 1024 // 1MB

// Transport defines the interface for sending and receiving JSON-RPC messages.
// Implementations must be concurrency-safe.
type Transport interface {
	// ReadMessage reads a single JSON-RPC message from the transport.
	// It returns the raw message bytes, or an error if reading fails.
	// The context allows for cancellation of long-running reads.
	ReadMessage(ctx context.Context) ([]byte, error)

	// WriteMessage sends a single JSON-RPC message over the transport.
	// It takes raw message bytes and returns an error if writing fails.
	// The context allows for cancellation of long-running writes.
	WriteMessage(ctx context.Context, message []byte) error

	// Close shuts down the transport, closing any underlying connections.
	// Any blocked Read or Write operations will be unblocked and return errors.
	Close() error
}

// MessageHandler defines the signature for a function that processes JSON-RPC messages.
// It receives the raw message bytes and returns a response message or error.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// ErrorHandler defines the signature for functions that handle transport errors.
// It allows customized error handling strategies.
type ErrorHandler func(ctx context.Context, err error)

// DefaultErrorHandler provides a basic error handling implementation.
func DefaultErrorHandler(ctx context.Context, err error) {
	// Default implementation does nothing; implementations should replace with
	// appropriate logging, metrics, etc.
}

// ValidateMessage performs basic validation on a JSON-RPC message.
// It ensures the message has the required fields for a JSON-RPC 2.0 message.
func ValidateMessage(message []byte) error {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return NewParseError(message, err)
	}

	// Check for required JSON-RPC 2.0 fields
	version, ok := msg["jsonrpc"]
	if !ok {
		return NewError(
			ErrInvalidMessage,
			"missing 'jsonrpc' field",
			nil,
		).WithContext("messagePreview", string(message[:min(len(message), 100)]))
	}

	if version != "2.0" {
		return NewError(
			ErrInvalidMessage,
			"unsupported JSON-RPC version",
			nil,
		).WithContext("version", version).
			WithContext("messagePreview", string(message[:min(len(message), 100)]))
	}

	// Check if it's a request, notification, or response
	if _, hasMethod := msg["method"]; hasMethod {
		// It's a request or notification
		if id, hasID := msg["id"]; hasID {
			// It's a request - validate ID format
			switch id.(type) {
			case string, float64, nil, json.Number:
				// These are valid ID types
			default:
				return NewError(
					ErrInvalidMessage,
					"invalid request ID type",
					nil,
				).WithContext("idType", id).
					WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}
		}
		// If no ID, it's a notification which is valid
	} else {
		// Should be a response - must have either result or error
		if _, hasResult := msg["result"]; !hasResult {
			if _, hasError := msg["error"]; !hasError {
				return NewError(
					ErrInvalidMessage,
					"response message must contain either 'result' or 'error' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}
		}
		// Must have an ID
		if _, hasID := msg["id"]; !hasID {
			return NewError(
				ErrInvalidMessage,
				"response message must contain 'id' field",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}
	}

	return nil
}

// min returns the smaller of x or y.
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// BaseTransport provides common functionality for transport implementations.
// It can be embedded in concrete transport types to reduce code duplication.
type BaseTransport struct {
	// Fields and methods common to all transport implementations
}

// Specific transport implementations should be in subpackages:
// internal/transport/stdio
// internal/transport/http
// internal/transport/sse

###
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/cockroachdb/errors"
)

// MaxMessageSize defines the maximum allowed size for a single JSON-RPC message in bytes.
// This helps prevent memory exhaustion attacks.
const MaxMessageSize = 1024 * 1024 // 1MB

// ErrorCode defines known error codes for the transport layer.
type ErrorCode int

// Defined error codes for the transport layer.
const (
	ErrGeneric ErrorCode = iota + 1000
	ErrInvalidMessage
	ErrMessageTooLarge
	ErrTransportClosed
	ErrReadTimeout
	ErrWriteTimeout
	ErrJSONParseFailed
)

// Transport defines the interface for sending and receiving JSON-RPC messages.
// Implementations must be concurrency-safe.
type Transport interface {
	// ReadMessage reads a single JSON-RPC message from the transport.
	// It returns the raw message bytes, or an error if reading fails.
	// The context allows for cancellation of long-running reads.
	ReadMessage(ctx context.Context) ([]byte, error)

	// WriteMessage sends a single JSON-RPC message over the transport.
	// It takes raw message bytes and returns an error if writing fails.
	// The context allows for cancellation of long-running writes.
	WriteMessage(ctx context.Context, message []byte) error

	// Close shuts down the transport, closing any underlying connections.
	// Any blocked Read or Write operations will be unblocked and return errors.
	Close() error
}

// MessageHandler defines the signature for a function that processes JSON-RPC messages.
// It receives the raw message bytes and returns a response message or error.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// ErrorHandler defines the signature for functions that handle transport errors.
// It allows customized error handling strategies.
type ErrorHandler func(ctx context.Context, err error)

// DefaultErrorHandler provides a basic error handling implementation.
func DefaultErrorHandler(ctx context.Context, err error) {
	// Default implementation does nothing; implementations should replace with
	// appropriate logging, metrics, etc.
}

// TransportError is the base error type for all transport-related errors.
// It implements the error interface and includes context and error codes.
type TransportError struct {
	// Code is the numeric error code for this error.
	Code ErrorCode

	// Message is a human-readable description of the error.
	Message string

	// Cause is the underlying error that caused this one, if any.
	Cause error

	// Context contains additional key-value pairs that provide context for debugging.
	Context map[string]interface{}
}

// Error implements the error interface for TransportError.
func (e *TransportError) Error() string {
	base := fmt.Sprintf("[%d] %s", e.Code, e.Message)
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", base, e.Cause)
	}
	return base
}

// Unwrap returns the underlying error for TransportError.
func (e *TransportError) Unwrap() error {
	return e.Cause
}

// WithContext adds or updates context information to the error.
func (e *TransportError) WithContext(key string, value interface{}) *TransportError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewTransportError creates a new transport error with the given code and message.
func NewTransportError(code ErrorCode, message string, cause error) *TransportError {
	return &TransportError{
		Code:    code,
		Message: message,
		Cause:   errors.WithStack(cause), // Preserve stack trace
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(),
		},
	}
}

// MessageSizeError is returned when a message exceeds the maximum allowed size.
type MessageSizeError struct {
	*TransportError
	Size     int
	MaxSize  int
	Fragment []byte // First few bytes of the oversized message for debugging
}

// NewMessageSizeError creates a new MessageSizeError with the provided details.
func NewMessageSizeError(size, maxSize int, fragment []byte) *MessageSizeError {
	err := &MessageSizeError{
		TransportError: &TransportError{
			Code:    ErrMessageTooLarge,
			Message: fmt.Sprintf("message size %d exceeds maximum allowed size %d", size, maxSize),
			Context: map[string]interface{}{
				"messageSize": size,
				"maxSize":     maxSize,
				"timestamp":   time.Now().UTC(),
			},
		},
		Size:     size,
		MaxSize:  maxSize,
		Fragment: fragment,
	}

	if len(fragment) > 0 {
		err.Context["messagePreview"] = string(fragment)
	}

	return err
}

// ParseError is returned when a message cannot be parsed as valid JSON-RPC.
type ParseError struct {
	*TransportError
	RawMessage []byte
}

// NewParseError creates a new ParseError with the provided details.
func NewParseError(message []byte, cause error) *ParseError {
	preview := message
	if len(preview) > 100 {
		preview = preview[:100]
	}

	return &ParseError{
		TransportError: &TransportError{
			Code:    ErrJSONParseFailed,
			Message: "failed to parse JSON-RPC message",
			Cause:   errors.WithStack(cause), // Preserve stack trace
			Context: map[string]interface{}{
				"messagePreview": string(preview),
				"messageLength":  len(message),
				"timestamp":      time.Now().UTC(),
			},
		},
		RawMessage: message,
	}
}

// JSON-RPC 2.0 error codes as defined in the specification.
const (
	// Standard JSON-RPC 2.0 error codes
	JSONRPCParseError     = -32700
	JSONRPCInvalidRequest = -32600
	JSONRPCMethodNotFound = -32601
	JSONRPCInvalidParams  = -32602
	JSONRPCInternalError  = -32603

	// Server error codes (reserved range)
	JSONRPCServerErrorStart = -32099
	JSONRPCServerErrorEnd   = -32000
)

// MapErrorToJSONRPC maps internal transport errors to JSON-RPC 2.0 error codes and messages.
// This function helps maintain a consistent mapping between our application errors and JSON-RPC errors.
func MapErrorToJSONRPC(err error) (int, string, map[string]interface{}) {
	// Default values
	code := JSONRPCInternalError
	message := "Internal error"
	data := map[string]interface{}{}

	// Check for specific error types
	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		data["errorCode"] = transportErr.Code

		// Map transport error codes to JSON-RPC error codes
		switch transportErr.Code {
		case ErrJSONParseFailed:
			code = JSONRPCParseError
			message = "Parse error"
		case ErrInvalidMessage:
			code = JSONRPCInvalidRequest
			message = "Invalid Request"
		case ErrMessageTooLarge:
			code = JSONRPCInvalidRequest
			message = "Message too large"
		default:
			// Use the server-defined error range for other transport errors
			code = JSONRPCServerErrorStart + int(transportErr.Code)
			message = transportErr.Message
		}

		// Add safe context data (be careful not to expose sensitive information)
		for k, v := range transportErr.Context {
			// Filter out potentially sensitive context keys
			if k != "messagePreview" && k != "timestamp" && k != "messageSize" && k != "maxSize" {
				continue
			}
			data[k] = v
		}
	}

	return code, message, data
}

// ValidateMessage performs basic validation on a JSON-RPC message.
// It ensures the message has the required fields for a JSON-RPC 2.0 message.
func ValidateMessage(message []byte) error {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return NewParseError(message, err)
	}

	// Check for required JSON-RPC 2.0 fields
	version, ok := msg["jsonrpc"]
	if !ok {
		return NewTransportError(
			ErrInvalidMessage,
			"missing 'jsonrpc' field",
			nil,
		).WithContext("messagePreview", string(message[:min(len(message), 100)]))
	}

	if version != "2.0" {
		return NewTransportError(
			ErrInvalidMessage,
			fmt.Sprintf("unsupported JSON-RPC version: %v", version),
			nil,
		).WithContext("version", version).
			WithContext("messagePreview", string(message[:min(len(message), 100)]))
	}

	// Check if it's a request, notification, or response
	if _, hasMethod := msg["method"]; hasMethod {
		// It's a request or notification
		if id, hasID := msg["id"]; hasID {
			// It's a request - validate ID format
			switch id.(type) {
			case string, float64, nil, json.Number:
				// These are valid ID types
			default:
				return NewTransportError(
					ErrInvalidMessage,
					fmt.Sprintf("invalid request ID type: %T", id),
					nil,
				).WithContext("idType", fmt.Sprintf("%T", id)).
					WithContext("idValue", fmt.Sprintf("%v", id)).
					WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}
		}
		// If no ID, it's a notification which is valid
	} else {
		// Should be a response - must have either result or error
		if _, hasResult := msg["result"]; !hasResult {
			if _, hasError := msg["error"]; !hasError {
				return NewTransportError(
					ErrInvalidMessage,
					"response message must contain either 'result' or 'error' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}
		}
		// Must have an ID
		if _, hasID := msg["id"]; !hasID {
			return NewTransportError(
				ErrInvalidMessage,
				"response message must contain 'id' field",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}
	}

	return nil
}

// min returns the smaller of x or y.
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// NewIOTransport creates a transport from an io.Reader and io.Writer.
// This is a general-purpose constructor that works with any paired Reader/Writer,
// and can be used by more specific transport implementations like stdio or network.
func NewIOTransport(reader io.Reader, writer io.Writer, closer io.Closer) Transport {
	return &ioTransport{
		reader: reader,
		writer: writer,
		closer: closer,
	}
}

// ioTransport implements Transport using an io.Reader and io.Writer.
type ioTransport struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
}

// ReadMessage implements Transport.ReadMessage using an io.Reader.
func (t *ioTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	// TODO: Implement bounded buffer reading with context support
	// This should include:
	// 1. Protection against lines that exceed MaxMessageSize
	// 2. Context cancellation support
	// 3. Proper error handling for malformed NDJSON

	// For now, return a placeholder error with stack trace
	return nil, errors.WithStack(NewTransportError(
		ErrGeneric,
		"ioTransport.ReadMessage not implemented",
		nil,
	))
}

// WriteMessage implements Transport.WriteMessage using an io.Writer.
func (t *ioTransport) WriteMessage(ctx context.Context, message []byte) error {
	// TODO: Implement bounded buffer writing with context support
	// This should include:
	// 1. Adding newline termination for NDJSON format
	// 2. Context cancellation support
	// 3. Atomic writes to avoid message corruption

	// For now, return a placeholder error with stack trace
	return errors.WithStack(NewTransportError(
		ErrGeneric,
		"ioTransport.WriteMessage not implemented",
		nil,
	).WithContext("messageLength", len(message)))
}

// Close implements Transport.Close.
func (t *ioTransport) Close() error {
	if t.closer != nil {
		if err := t.closer.Close(); err != nil {
			return errors.WithStack(NewTransportError(
				ErrGeneric,
				"failed to close transport",
				err,
			))
		}
	}
	return nil
}
