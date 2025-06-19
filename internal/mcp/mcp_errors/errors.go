// Package errors defines domain-specific error types and codes for the MCP (Model Context Protocol) layer.
// These errors provide more context than standard Go errors and help in mapping internal issues.
// to appropriate JSON-RPC error responses or handling them specifically within the application.
package errors

// file: internal/errors.go

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/transport" // Import transport for JSON-RPC codes.
)

// ErrorCode defines domain-specific error codes for the MCP layer.
type ErrorCode int

// Domain-specific error codes for the MCP layer.
const (
	// --- Auth Errors (1000-1999) ---.
	ErrAuthFailure ErrorCode = 1000 + iota
	ErrAuthExpired
	ErrAuthInvalid
	ErrAuthMissing

	// --- RTM API Errors (2000-2999) ---.
	ErrRTMAPIFailure ErrorCode = 2000 + iota
	ErrRTMInvalidResponse
	ErrRTMServiceUnavailable
	ErrRTMPermissionDenied

	// --- Resource Errors (3000-3999) ---.
	ErrResourceNotFound ErrorCode = 3000 + iota
	ErrResourceForbidden
	ErrResourceInvalid

	// --- Protocol Errors (4000-4999 AND JSON-RPC range) ---.
	// Note: Mixing iota with specific assignments requires restarting iota if needed,
	// or carefully managing the sequence. Here, we assign specific values
	// for JSON-RPC equivalents and custom protocol errors.
	ErrProtocolInvalid     ErrorCode = 4000 + iota // iota = 0 here -> 4000
	ErrProtocolUnsupported                         // iota = 1 -> 4001 // <<< THIS VALUE IS LIKELY UNUSED/OBSOLETE

	// Map specific internal errors to JSON-RPC standard codes where applicable.
	ErrParseError     ErrorCode = -32700 // JSONRPCParseError
	ErrInvalidRequest ErrorCode = -32600 // JSONRPCInvalidRequest
	ErrMethodNotFound ErrorCode = -32601 // JSONRPCMethodNotFound
	ErrInvalidParams  ErrorCode = -32602 // JSONRPCInvalidParams
	ErrInternalError  ErrorCode = -32603 // JSONRPCInternalError

	// Custom server-defined protocol errors within the recommended range (-32000 to -32099).
	ErrRequestSequence ErrorCode = -32001 // Invalid message sequence for current state
	ErrServiceNotFound ErrorCode = -32002 // Specific internal error when routing fails

	// You can add more custom -320xx codes here if needed...
)

// BaseError is the common base for custom MCP error types.
// It embeds the standard error interface and adds structured context like codes and key-value details.
type BaseError struct {
	// Code is a numeric error code for categorization (using constants defined above).
	Code ErrorCode
	// Message is a human-readable error message intended primarily for logging and debugging.
	Message string
	// Cause is the underlying error that led to this error, allowing error chain traversal.
	Cause error
	// Context contains additional key-value details relevant to the error (e.g., resource URI, user ID).
	Context map[string]interface{}
}

// Error implements the standard Go error interface.
// It formats the error message including the code and the underlying cause if present.
func (e *BaseError) Error() string {
	if e.Cause != nil {
		// Use %v for standard cause formatting. Use %+v if stack trace from cause is desired.
		return fmt.Sprintf("MCPError (Code: %d): %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("MCPError (Code: %d): %s", e.Code, e.Message)
}

// Unwrap returns the underlying error (Cause), enabling errors.Is and errors.As.
func (e *BaseError) Unwrap() error {
	return e.Cause
}

// WithContext adds a key-value pair to the error's context map.
// It initializes the map if necessary and returns the modified error pointer for chaining.
func (e *BaseError) WithContext(key string, value interface{}) *BaseError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// --- Specific Error Type Structs ---.

// AuthError represents an authentication or authorization error within the MCP layer or related services.
type AuthError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// RTMError represents an error specifically related to interactions with the Remember The Milk API.
type RTMError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// ResourceError represents an error related to accessing or manipulating an MCP resource (identified by URI).
type ResourceError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// ProtocolError represents a violation of the MCP protocol rules or structure.
type ProtocolError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// InvalidParamsError represents an error due to invalid method parameters.
type InvalidParamsError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// MethodNotFoundError represents an error when a requested method is not found.
type MethodNotFoundError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// ServiceNotFoundError represents an error when a requested service cannot be found in the registry.
type ServiceNotFoundError struct {
	BaseError
}

// InternalError represents a generic internal server error.
type InternalError struct {
	BaseError
}

// ParseError represents a JSON parsing error.
type ParseError struct {
	BaseError
}

// InvalidRequestError represents an invalid JSON-RPC request structure error.
type InvalidRequestError struct {
	BaseError
}

// --- Constructor Functions ---.

// NewAuthError creates a new authentication error, ensuring the cause is wrapped for stack trace.
// Use constants like ErrAuthFailure, ErrAuthExpired, ErrAuthInvalid, ErrAuthMissing for the code.
func NewAuthError(code ErrorCode, message string, cause error, context map[string]interface{}) error {
	// Default to general auth failure if code is invalid or not provided.
	if code < 1000 || code > 1999 {
		code = ErrAuthFailure
	}
	err := &AuthError{
		BaseError: BaseError{
			Code:    code,
			Message: message,
			Cause:   errors.WithStack(cause), // Wrap cause for stack trace.
			Context: context,
		},
	}
	return err
}

// NewRTMError creates a new RTM API error, ensuring the cause is wrapped for stack trace.
// Use constants like ErrRTMAPIFailure, ErrRTMInvalidResponse etc. for the code.
func NewRTMError(code ErrorCode, message string, cause error, context map[string]interface{}) error {
	// Default to general RTM API failure if code is invalid or not provided.
	if code < 2000 || code > 2999 {
		code = ErrRTMAPIFailure
	}
	err := &RTMError{
		BaseError: BaseError{
			Code:    code,
			Message: message,
			Cause:   errors.WithStack(cause), // Wrap cause for stack trace.
			Context: context,
		},
	}
	return err
}

// NewResourceError creates a new resource error, ensuring the cause is wrapped for stack trace.
// Use constants like ErrResourceNotFound, ErrResourceForbidden, ErrResourceInvalid for the code.
func NewResourceError(code ErrorCode, message string, cause error, context map[string]interface{}) error {
	// Default to resource not found if code is invalid or not provided.
	if code < 3000 || code > 3999 {
		code = ErrResourceNotFound
	}
	err := &ResourceError{
		BaseError: BaseError{
			Code:    code,
			Message: message,
			Cause:   errors.WithStack(cause), // Wrap cause for stack trace.
			Context: context,
		},
	}
	return err
}

// NewProtocolError creates a new protocol error.
// --- MODIFICATION START ---
// Removed the default code assignment logic that was causing the bug.
func NewProtocolError(code ErrorCode, message string, cause error, context map[string]interface{}) error {
	// Rely on the caller to provide a valid ErrorCode constant.
	// if code < 4000 || code > 4999 { // <<< REMOVED THIS INCORRECT CHECK
	// 	code = ErrProtocolInvalid
	// }
	err := &ProtocolError{
		BaseError: BaseError{
			Code:    code, // Use the provided code directly.
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// --- MODIFICATION END ---

// NewInvalidParamsError creates an error for invalid parameters (maps to -32602).
func NewInvalidParamsError(message string, cause error, context map[string]interface{}) error {
	err := &InvalidParamsError{
		BaseError: BaseError{
			Code:    ErrInvalidParams,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// NewMethodNotFoundError creates an error for method not found (maps to -32601).
func NewMethodNotFoundError(message string, cause error, context map[string]interface{}) error {
	err := &MethodNotFoundError{
		BaseError: BaseError{
			Code:    ErrMethodNotFound,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// NewServiceNotFoundError creates an error when a service lookup fails.
func NewServiceNotFoundError(message string, cause error, context map[string]interface{}) error {
	err := &ServiceNotFoundError{
		BaseError: BaseError{
			Code:    ErrServiceNotFound,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// NewInternalError creates a generic internal server error (maps to -32603).
func NewInternalError(message string, cause error, context map[string]interface{}) error {
	err := &InternalError{
		BaseError: BaseError{
			Code:    ErrInternalError,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// NewParseError creates a JSON parse error (maps to -32700).
func NewParseError(message string, cause error, context map[string]interface{}) error {
	err := &ParseError{
		BaseError: BaseError{
			Code:    ErrParseError,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// NewInvalidRequestError creates an invalid request structure error (maps to -32600).
func NewInvalidRequestError(message string, cause error, context map[string]interface{}) error {
	err := &InvalidRequestError{
		BaseError: BaseError{
			Code:    ErrInvalidRequest,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// --- JSON-RPC Error Mapping ---.

// MapMCPErrorToJSONRPC translates an MCP error (or any error) into JSON-RPC components.
func MapMCPErrorToJSONRPC(err error) (code int, message string, data map[string]interface{}) {
	// Initialize data map.
	data = make(map[string]interface{})

	var baseErr *BaseError
	if !errors.As(err, &baseErr) {
		// Not one of our specific MCP errors, treat as generic internal.
		code = transport.JSONRPCInternalError // -32603.
		message = "An internal server error occurred."
		data["goErrorType"] = fmt.Sprintf("%T", err)
		data["detail"] = err.Error() // Include original error message in data.
		return code, message, data
	}

	// Map based on the MCP error code.
	switch baseErr.Code {
	// JSON-RPC Standard Codes.
	case ErrParseError:
		code = transport.JSONRPCParseError // -32700.
		message = "Parse error."
		data["detail"] = baseErr.Message
	case ErrInvalidRequest:
		code = transport.JSONRPCInvalidRequest // -32600.
		message = "Invalid Request."
		data["detail"] = baseErr.Message
	case ErrMethodNotFound:
		code = transport.JSONRPCMethodNotFound // -32601.
		message = "Method not found."
		data["detail"] = baseErr.Message
	case ErrInvalidParams:
		code = transport.JSONRPCInvalidParams // -32602.
		message = "Invalid params."
		data["detail"] = baseErr.Message
	case ErrInternalError:
		code = transport.JSONRPCInternalError // -32603.
		message = "Internal error."
		data["detail"] = baseErr.Message

	// Implementation-Defined Server Errors (-32000 to -32099).
	// Assign specific codes within this range for application errors.
	case ErrServiceNotFound:
		code = -32000
		message = "Service unavailable." // User-friendly message.
		data["detail"] = baseErr.Message // Internal detail.
	case ErrRequestSequence: // Added case for ErrRequestSequence
		code = -32001 // Assigning a specific code
		message = "Invalid Request Sequence."
		data["detail"] = baseErr.Message
	case ErrResourceNotFound:
		code = -32002 // Renumbered slightly
		message = "Resource not found."
		data["detail"] = baseErr.Message
	case ErrResourceInvalid:
		code = -32003 // Renumbered slightly
		message = "Invalid resource identifier."
		data["detail"] = baseErr.Message
	case ErrAuthFailure, ErrAuthInvalid, ErrAuthExpired, ErrAuthMissing:
		code = -32010
		message = "Authentication required or failed."
		data["detail"] = baseErr.Message
		// Avoid leaking specific auth failure reasons unless intended.
	case ErrRTMAPIFailure, ErrRTMInvalidResponse, ErrRTMServiceUnavailable:
		code = -32020
		message = "Could not communicate with external service (RTM)."
		data["detail"] = baseErr.Message
	case ErrRTMPermissionDenied:
		code = -32021
		message = "Permission denied by external service (RTM)."
		data["detail"] = baseErr.Message
	case ErrProtocolInvalid: // Code 4000
		code = transport.JSONRPCInvalidRequest // Map general protocol issues to -32600 for client
		message = "Invalid Request (Protocol)."
		data["detail"] = baseErr.Message
		data["internalCode"] = baseErr.Code // Include original internal code
	case ErrProtocolUnsupported: // Code 4001
		code = transport.JSONRPCMethodNotFound // Map unsupported protocol features to -32601
		message = "Unsupported Operation (Protocol)."
		data["detail"] = baseErr.Message
		data["internalCode"] = baseErr.Code // Include original internal code

	// Fallback for any other MCP error codes (e.g., 1xxx, 2xxx, 3xxx).
	default:
		code = transport.JSONRPCInternalError // -32603.
		message = "An unspecified internal error occurred."
		data["detail"] = baseErr.Message
		data["internalCode"] = baseErr.Code
	}

	// Merge context from the BaseError, avoiding sensitive info.
	if baseErr.Context != nil {
		for k, v := range baseErr.Context {
			// Only include context fields deemed safe for client exposure.
			// Example: Allow 'uri', 'toolName', 'method' but not internal details.
			switch k {
			case "uri", "toolName", "method", "service", "state": // Add more safe keys as needed.
				if _, exists := data[k]; !exists { // Avoid overwriting standard fields like 'method'.
					data[k] = v
				}
			default:
				// Potentially log unsafe context keys server-side but don't add to client response data.
			}
		}
	}

	// Remove data field if empty after filtering.
	if len(data) == 0 {
		data = nil
	}

	return code, message, data
}
