// Package transport defines interfaces and implementations for sending and receiving MCP messages.
// This file specifically defines custom error types and codes used within the transport layer,
// providing structured error information beyond standard Go errors.
package transport

// file: internal/transport/transport_errors.go

import (
	"fmt"
	"io"
	"time"

	"github.com/cockroachdb/errors"
)

// ErrorCode defines specific numeric codes for transport-layer errors.
// These help categorize errors beyond the standard Go error interface.
type ErrorCode int

// Defined error codes for the transport layer.
const (
	// ErrGeneric represents a general or unspecified transport error.
	ErrGeneric ErrorCode = iota + 1000 // Start custom codes above common HTTP/RPC ranges.
	// ErrInvalidMessage indicates a message violated framing or basic structural rules (not JSON-RPC spec rules, which have their own codes).
	ErrInvalidMessage
	// ErrMessageTooLarge signifies a message exceeded the defined MaxMessageSize.
	ErrMessageTooLarge
	// ErrTransportClosed indicates an operation was attempted on a closed transport.
	ErrTransportClosed
	// ErrReadTimeout signifies a timeout during a read operation.
	ErrReadTimeout
	// ErrWriteTimeout signifies a timeout during a write operation.
	ErrWriteTimeout
	// ErrJSONParseFailed indicates a failure during the initial JSON syntax parsing.
	ErrJSONParseFailed
)

// ErrorType categorizes transport errors for higher-level handling or filtering.
type ErrorType int

// Defined error types for transport errors.
const (
	// ErrorTypeGeneric represents a general or unspecified transport error.
	ErrorTypeGeneric ErrorType = iota
	// ErrorTypeMessageSize indicates an error due to excessive message size.
	ErrorTypeMessageSize
	// ErrorTypeParse indicates a JSON parsing error.
	ErrorTypeParse
	// ErrorTypeTimeout indicates a timeout during a read or write operation.
	ErrorTypeTimeout
	// ErrorTypeClosed indicates an operation was attempted on a closed transport.
	ErrorTypeClosed
)

// Error represents a transport-level error, providing structured details beyond the basic error message.
// It includes a type, code, underlying cause, and optional context.
type Error struct {
	// Type categorizes the error (e.g., Timeout, Closed).
	Type ErrorType
	// Code provides a specific numeric identifier for the error condition.
	Code ErrorCode
	// Message is a human-readable description of the error.
	Message string
	// Cause holds the underlying error that triggered this transport error, if any.
	Cause error
	// Context stores additional key-value pairs relevant to the error (e.g., message preview, size).
	Context map[string]interface{}

	// --- Fields specific to certain error types ---
	// Size holds the actual message size for MessageSize errors.
	Size int
	// MaxSize holds the configured maximum size for MessageSize errors.
	MaxSize int
	// Fragment holds a preview of the oversized message for MessageSize errors.
	Fragment []byte
}

// Error implements the standard Go error interface, providing a formatted error string.
func (e *Error) Error() string {
	base := fmt.Sprintf("TransportError [%d] %s", e.Code, e.Message)
	if e.Cause != nil {
		// Use %v for standard cause formatting. Use %+v if stack trace from cause is desired.
		return fmt.Sprintf("%s: %v", base, e.Cause)
	}
	return base
}

// Unwrap returns the underlying cause of the error, allowing for error inspection with errors.Is/As.
func (e *Error) Unwrap() error {
	return e.Cause
}

// WithContext adds or updates a key-value pair in the error's context map.
// Returns the modified error pointer for chaining.
func (e *Error) WithContext(key string, value interface{}) *Error {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// Is implements error comparison logic for use with errors.Is.
// It checks if the target error is a transport.Error with the same Type and Code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	// Primarily match by error type and code for categorization.
	return e.Type == t.Type && e.Code == t.Code
}

// NewError creates a basic transport error with a generic type.
// The cause error is wrapped using errors.WithStack to preserve stack trace information.
func NewError(code ErrorCode, message string, cause error) *Error {
	var wrappedCause error
	if cause != nil {
		wrappedCause = errors.WithStack(cause) // Ensure stack trace is attached.
	}
	return &Error{
		Type:    ErrorTypeGeneric,
		Code:    code,
		Message: message,
		Cause:   wrappedCause,
		Context: map[string]interface{}{ // Initialize context with a timestamp.
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
}

// NewMessageSizeError creates a specific transport error for messages exceeding MaxMessageSize.
// It includes the message size, maximum allowed size, and a fragment for context.
func NewMessageSizeError(size, maxSize int, fragment []byte) *Error {
	err := NewError(
		ErrMessageTooLarge,
		fmt.Sprintf("message size %d exceeds maximum allowed size %d", size, maxSize),
		nil, // No underlying Go error typically causes this.
	)
	err.Type = ErrorTypeMessageSize
	err.Size = size
	err.MaxSize = maxSize
	err.Fragment = fragment // Store fragment internally if needed elsewhere.

	if len(fragment) > 0 {
		// Add preview directly to context for easier logging/access.
		err = err.WithContext("messagePreview", string(fragment))
	}

	return err
}

// NewParseError creates a specific transport error for failures during initial JSON syntax parsing.
// Includes a preview of the invalid message.
func NewParseError(message []byte, cause error) *Error {
	preview := message
	maxPreview := 100
	if len(preview) > maxPreview {
		preview = preview[:maxPreview]
	}

	err := NewError(
		ErrJSONParseFailed,
		"failed to parse JSON message syntax",
		cause, // Include the underlying json.Unmarshal error.
	)
	err.Type = ErrorTypeParse
	err = err.WithContext("messagePreview", string(preview))
	err = err.WithContext("messageLength", len(message))

	return err
}

// NewTimeoutError creates a specific transport error for read or write timeouts.
// Distinguishes between read/write operations based on the provided string.
func NewTimeoutError(operation string, cause error) *Error {
	code := ErrReadTimeout
	if operation == "write" {
		code = ErrWriteTimeout
	}
	err := NewError(
		code,
		fmt.Sprintf("%s operation timed out", operation),
		cause, // Include the context.DeadlineExceeded or similar error.
	)
	err.Type = ErrorTypeTimeout
	err = err.WithContext("operation", operation) // Add operation type to context.

	return err
}

// NewClosedError creates a specific transport error for operations attempted on a closed transport.
func NewClosedError(operation string) *Error {
	err := NewError(
		ErrTransportClosed,
		fmt.Sprintf("cannot perform %s on closed transport", operation),
		nil, // Typically no underlying Go error, just state violation.
	)
	err.Type = ErrorTypeClosed
	err = err.WithContext("operation", operation) // Add operation type to context.

	return err
}

// --- JSON-RPC 2.0 Error Code Constants ---
// Standard codes used when mapping transport errors to JSON-RPC responses.
const (
	// JSONRPCParseError indicates invalid JSON was received by the server.
	// An error occurred on the server while parsing the JSON text.
	JSONRPCParseError = -32700
	// JSONRPCInvalidRequest indicates the JSON sent is not a valid Request object.
	JSONRPCInvalidRequest = -32600
	// JSONRPCMethodNotFound indicates the method does not exist / is not available.
	JSONRPCMethodNotFound = -32601
	// JSONRPCInvalidParams indicates invalid method parameter(s).
	JSONRPCInvalidParams = -32602
	// JSONRPCInternalError indicates an internal JSON-RPC error.
	JSONRPCInternalError = -32603

	// JSONRPCServerErrorStart defines the start of the reserved range for implementation-defined server-errors.
	JSONRPCServerErrorStart = -32099
	// JSONRPCServerErrorEnd defines the end of the reserved range for implementation-defined server-errors.
	JSONRPCServerErrorEnd = -32000
)

// MapErrorToJSONRPC maps internal transport errors to JSON-RPC 2.0 error codes,
// messages, and an optional data payload suitable for a JSON-RPC error response.
func MapErrorToJSONRPC(err error) (code int, message string, data map[string]interface{}) {
	// Initialize data map.
	data = make(map[string]interface{})

	var transportErr *Error
	if errors.As(err, &transportErr) {
		// Add internal transport code to data payload for correlation.
		data["internalCode"] = transportErr.Code
		data["errorType"] = fmt.Sprintf("%T", transportErr) // Add type info.

		// Map specific transport errors to standard JSON-RPC codes.
		switch transportErr.Code {
		case ErrJSONParseFailed:
			code = JSONRPCParseError
			message = "Parse error"
			data["detail"] = "Invalid JSON received."
		case ErrInvalidMessage:
			code = JSONRPCInvalidRequest
			message = "Invalid Request"
			data["detail"] = "Message format is invalid or does not conform to transport expectations."
		case ErrMessageTooLarge:
			code = JSONRPCInvalidRequest // Often treated as invalid request.
			message = "Invalid Request"
			data["detail"] = fmt.Sprintf("Message size (%d bytes) exceeds limit (%d bytes).", transportErr.Size, transportErr.MaxSize)
		case ErrTransportClosed, ErrReadTimeout, ErrWriteTimeout:
			code = JSONRPCInternalError // Treat transport operational issues as internal errors.
			message = "Internal error"
			data["detail"] = "Transport communication error occurred." // Avoid exposing too much detail.
		default: // Other generic transport errors.
			code = JSONRPCInternalError
			message = "Internal error"
			data["detail"] = "An unspecified transport error occurred."
		}

		// Selectively add context, ensuring no sensitive info is leaked.
		if ctx := transportErr.Context; ctx != nil {
			if preview, ok := ctx["messagePreview"].(string); ok {
				data["messagePreview"] = preview
			}
			if length, ok := ctx["messageLength"].(int); ok {
				data["messageLength"] = length
			}
			// Add other *safe* context fields as needed.
		}
	} else {
		// If it's not a transport.Error, treat as a generic internal server error.
		code = JSONRPCInternalError
		message = "Internal error"
		data["detail"] = "An unexpected internal server error occurred."
		// Optionally include the Go error type for server-side logging/debugging.
		data["goErrorType"] = fmt.Sprintf("%T", err)
	}

	return code, message, data
}

// IsClosedError checks if an error (or its cause chain) signifies a closed transport condition.
// It specifically looks for transport errors with the ErrorTypeClosed type.
func IsClosedError(err error) bool {
	var transportErr *Error
	// errors.As checks the entire error chain.
	if errors.As(err, &transportErr) {
		return transportErr.Type == ErrorTypeClosed
	}
	// Additionally check for standard io.EOF which often indicates closure.
	if errors.Is(err, io.EOF) {
		return true
	}
	// Consider adding checks for net.ErrClosed if using network transports directly.
	return false
}
