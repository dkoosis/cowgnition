// file: internal/transport/errors.go
package transport

import (
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
)

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

// ErrorType identifies the specific category of transport error.
type ErrorType int

const (
	ErrorTypeGeneric ErrorType = iota
	ErrorTypeMessageSize
	ErrorTypeParse
	ErrorTypeTimeout
	ErrorTypeClosed
)

// Error represents a transport-level error with contextual information.
type Error struct {
	Type    ErrorType              // Discriminator field
	Code    ErrorCode              // Numeric error code
	Message string                 // Human-readable error message
	Cause   error                  // Underlying error, if any
	Context map[string]interface{} // Contextual information for debugging

	// Fields for specific error types, used based on Type value
	Size     int    // For MessageSize errors
	MaxSize  int    // For MessageSize errors
	Fragment []byte // For MessageSize errors
}

// Error implements the error interface.
func (e *Error) Error() string {
	base := fmt.Sprintf("[%d] %s", e.Code, e.Message)
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", base, e.Cause)
	}
	return base
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Cause
}

// WithContext adds or updates context information to the error.
func (e *Error) WithContext(key string, value interface{}) *Error {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// Is implements error matching for the errors.Is function.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}

	// Match by error type and code
	return e.Type == t.Type && e.Code == t.Code
}

// NewError creates a basic transport error.
func NewError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Type:    ErrorTypeGeneric,
		Code:    code,
		Message: message,
		Cause:   errors.WithStack(cause), // Preserve stack trace
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(),
		},
	}
}

// NewMessageSizeError creates a message size error.
func NewMessageSizeError(size, maxSize int, fragment []byte) *Error {
	err := NewError(
		ErrMessageTooLarge,
		fmt.Sprintf("message size %d exceeds maximum allowed size %d", size, maxSize),
		nil,
	)
	err.Type = ErrorTypeMessageSize
	err.Size = size
	err.MaxSize = maxSize
	err.Fragment = fragment

	if len(fragment) > 0 {
		err.Context["messagePreview"] = string(fragment)
	}

	return err
}

// NewParseError creates an error for JSON parsing failures.
func NewParseError(message []byte, cause error) *Error {
	preview := message
	if len(preview) > 100 {
		preview = preview[:100]
	}

	err := NewError(
		ErrJSONParseFailed,
		"failed to parse JSON-RPC message",
		cause,
	)
	err.Type = ErrorTypeParse
	err.WithContext("messagePreview", string(preview))
	err.WithContext("messageLength", len(message))

	return err
}

// NewTimeoutError creates an error for read/write timeouts.
func NewTimeoutError(operation string, cause error) *Error {
	err := NewError(
		ErrReadTimeout, // Will be updated below if it's a write operation
		fmt.Sprintf("%s operation timed out", operation),
		cause,
	)
	err.Type = ErrorTypeTimeout

	// Determine if it's a read or write timeout
	if operation == "write" {
		err.Code = ErrWriteTimeout
	}

	return err
}

// NewClosedError creates an error for operations on a closed transport.
func NewClosedError(operation string) *Error {
	err := NewError(
		ErrTransportClosed,
		fmt.Sprintf("cannot perform %s on closed transport", operation),
		nil,
	)
	err.Type = ErrorTypeClosed

	return err
}

// JSON-RPC 2.0 error codes as defined in the specification.
const (
	// Standard JSON-RPC 2.0 error codes.
	JSONRPCParseError     = -32700
	JSONRPCInvalidRequest = -32600
	JSONRPCMethodNotFound = -32601
	JSONRPCInvalidParams  = -32602
	JSONRPCInternalError  = -32603

	// Server error codes (reserved range).
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
	var transportErr *Error
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
