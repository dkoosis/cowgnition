// file: internal/mcp/errors/errors.go
package errors

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

// Domain-specific error codes.
const (
	// Auth-related error codes.
	ErrAuthFailure = iota + 1000
	ErrAuthExpired
	ErrAuthInvalid
	ErrAuthMissing

	// RTM API error codes.
	ErrRTMAPIFailure = iota + 2000
	ErrRTMInvalidResponse
	ErrRTMServiceUnavailable
	ErrRTMPermissionDenied

	// Resource-related error codes.
	ErrResourceNotFound = iota + 3000
	ErrResourceForbidden
	ErrResourceInvalid

	// Protocol-related error codes.
	ErrProtocolInvalid = iota + 4000
	ErrProtocolUnsupported
)

// BaseError is the common base for all custom error types.
// It implements the error interface and provides context-rich information.
type BaseError struct {
	// Code is the numeric error code for categorization.
	Code int

	// Message is a human-readable error message.
	Message string

	// Cause is the underlying error, if any.
	Cause error

	// Context contains additional error details.
	Context map[string]interface{}
}

// Error implements the error interface.
func (e *BaseError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *BaseError) Unwrap() error {
	return e.Cause
}

// WithContext adds a key-value pair to the error context.
func (e *BaseError) WithContext(key string, value interface{}) *BaseError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// AuthError represents an authentication or authorization error.
type AuthError struct {
	BaseError
}

// NewAuthError creates a new authentication error.
func NewAuthError(message string, cause error, context map[string]interface{}) error {
	err := &AuthError{
		BaseError: BaseError{
			Code:    ErrAuthFailure,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// IsAuthError checks if an error is an authentication error with the specified message.
func IsAuthError(err error, message string) bool {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return authErr.Message == message
	}
	return false
}

// RTMError represents an error related to the Remember The Milk API.
type RTMError struct {
	BaseError
}

// NewRTMError creates a new RTM API error.
func NewRTMError(code int, message string, cause error, context map[string]interface{}) error {
	// If no specific code is provided, use the general RTM failure code
	if code <= 0 {
		code = ErrRTMAPIFailure
	}

	err := &RTMError{
		BaseError: BaseError{
			Code:    code,
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}

// ResourceError represents an error related to a resource operation.
type ResourceError struct {
	BaseError
}

// NewResourceError creates a new resource error.
func NewResourceError(message string, cause error, context map[string]interface{}) error {
	err := &ResourceError{
		BaseError: BaseError{
			Code:    ErrResourceNotFound, // Default code
			Message: message,
			Cause:   errors.WithStack(cause),
			Context: context,
		},
	}
	return err
}
