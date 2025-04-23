// Package errors defines domain-specific error types and codes for the MCP (Model Context Protocol) layer.
// These errors provide more context than standard Go errors and help in mapping internal issues
// to appropriate JSON-RPC error responses or handling them specifically within the application.
// file: internal/mcp/mcp_errors/errors.go.
package errors

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

// Domain-specific error codes for the MCP layer.
const (
	// --- Auth Errors (1000-1999) ---.

	// ErrAuthFailure indicates a general authentication or authorization failure.
	ErrAuthFailure = iota + 1000
	// ErrAuthExpired indicates the provided credentials or token have expired.
	ErrAuthExpired
	// ErrAuthInvalid indicates the provided credentials or token are invalid or malformed.
	ErrAuthInvalid
	// ErrAuthMissing indicates required credentials or token were not provided.
	ErrAuthMissing

	// --- RTM API Errors (2000-2999) ---.

	// ErrRTMAPIFailure indicates a general failure during interaction with the RTM API.
	// This could be due to network issues, unexpected responses, or specific RTM error codes not handled individually.
	ErrRTMAPIFailure = iota + 1000 // Start RTM codes from 2000.
	// ErrRTMInvalidResponse indicates the RTM API returned a response that could not be parsed or was unexpected.
	ErrRTMInvalidResponse
	// ErrRTMServiceUnavailable indicates the RTM API service is temporarily unavailable.
	ErrRTMServiceUnavailable
	// ErrRTMPermissionDenied indicates the authenticated RTM user does not have permission for the requested action.
	ErrRTMPermissionDenied

	// --- Resource Errors (3000-3999) ---.

	// ErrResourceNotFound indicates a requested MCP resource (identified by URI) could not be found.
	ErrResourceNotFound = iota + 1000 // Start Resource codes from 3000.
	// ErrResourceForbidden indicates access to the requested MCP resource is denied.
	ErrResourceForbidden
	// ErrResourceInvalid indicates the requested MCP resource URI or associated data is invalid.
	ErrResourceInvalid

	// --- Protocol Errors (4000-4999) ---.

	// ErrProtocolInvalid indicates a violation of the MCP protocol rules (e.g., invalid sequence, unexpected message).
	ErrProtocolInvalid = iota + 1000 // Start Protocol codes from 4000.
	// ErrProtocolUnsupported indicates a requested feature or capability is not supported by this server implementation.
	ErrProtocolUnsupported
)

// BaseError is the common base for custom MCP error types.
// It embeds the standard error interface and adds structured context like codes and key-value details.
type BaseError struct {
	// Code is a numeric error code for categorization (using constants defined above).
	Code int
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

// AuthError represents an authentication or authorization error within the MCP layer or related services.
type AuthError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// NewAuthError creates a new authentication error, ensuring the cause is wrapped for stack trace.
// Use constants like ErrAuthFailure, ErrAuthExpired, ErrAuthInvalid, ErrAuthMissing for the code.
func NewAuthError(code int, message string, cause error, context map[string]interface{}) error {
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

// IsAuthError checks if an error is specifically an AuthError with a matching message.
// Deprecated: Prefer using errors.As and checking the Code field for more robust type checking.
func IsAuthError(err error, message string) bool {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		// Note: Matching solely on message string is brittle. Checking Code is better.
		return authErr.Message == message
	}
	return false
}

// RTMError represents an error specifically related to interactions with the Remember The Milk API.
type RTMError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// NewRTMError creates a new RTM API error, ensuring the cause is wrapped for stack trace.
// Use constants like ErrRTMAPIFailure, ErrRTMInvalidResponse etc. for the code.
func NewRTMError(code int, message string, cause error, context map[string]interface{}) error {
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

// ResourceError represents an error related to accessing or manipulating an MCP resource (identified by URI).
type ResourceError struct {
	BaseError // Embeds Code, Message, Cause, Context.
}

// NewResourceError creates a new resource error, ensuring the cause is wrapped for stack trace.
// Use constants like ErrResourceNotFound, ErrResourceForbidden, ErrResourceInvalid for the code.
func NewResourceError(code int, message string, cause error, context map[string]interface{}) error {
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

// Note: ProtocolError type could be added similarly if needed, using codes ErrProtocolInvalid, ErrProtocolUnsupported.
