// Package mcp defines common error types and variables used within the MCP server.
// file: internal/mcp/errors.go
package mcp

import (
	"github.com/cockroachdb/errors"
)

// Error categories for grouping similar errors
const (
	ErrCategoryResource = "resource" // Resource-related errors
	ErrCategoryTool     = "tool"     // Tool-related errors
	ErrCategoryAuth     = "auth"     // Authentication-related errors
	ErrCategoryConfig   = "config"   // Configuration-related errors
	ErrCategoryRPC      = "rpc"      // JSON-RPC-related errors
	ErrCategoryRTM      = "rtm"      // Remember The Milk API-related errors
)

// Error codes aligned with JSON-RPC 2.0 specification
// Standard codes: -32700 to -32603
// Custom codes: -32000 to -32099
const (
	// Standard JSON-RPC 2.0 error codes
	CodeParseError     = -32700 // Invalid JSON received
	CodeInvalidRequest = -32600 // Invalid request object
	CodeMethodNotFound = -32601 // Method not found
	CodeInvalidParams  = -32602 // Invalid method parameters
	CodeInternalError  = -32603 // Internal JSON-RPC error

	// Custom application error codes
	CodeResourceNotFound = -32000 // Requested resource not found
	CodeToolNotFound     = -32001 // Requested tool not found
	CodeInvalidArguments = -32002 // Invalid arguments provided
	CodeAuthError        = -32003 // Authentication error
	CodeRTMError         = -32004 // Remember The Milk API error
	CodeTimeoutError     = -32005 // Operation timed out
)

// Base sentinel errors
var (
	// Resource errors
	ErrResourceNotFound = errors.WithProperty(
		errors.New("resource not found"),
		"category", ErrCategoryResource,
		"code", CodeResourceNotFound,
	)

	// Tool errors
	ErrToolNotFound = errors.WithProperty(
		errors.New("tool not found"),
		"category", ErrCategoryTool,
		"code", CodeToolNotFound,
	)

	// Argument errors
	ErrInvalidArguments = errors.WithProperty(
		errors.New("invalid arguments"),
		"category", ErrCategoryRPC,
		"code", CodeInvalidArguments,
	)

	// Timeout errors
	ErrTimeout = errors.WithProperty(
		errors.New("operation timed out"),
		"category", ErrCategoryRPC,
		"code", CodeTimeoutError,
	)
)

// NewResourceError creates a new resource-related error with context
func NewResourceError(message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", ErrCategoryResource,
		"code", CodeResourceNotFound,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewToolError creates a new tool-related error with context
func NewToolError(message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", ErrCategoryTool,
		"code", CodeToolNotFound,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewInvalidArgumentsError creates a new invalid arguments error with context
func NewInvalidArgumentsError(message string, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Newf("%s", message),
		"category", ErrCategoryRPC,
		"code", CodeInvalidArguments,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewAuthError creates a new authentication error with context
func NewAuthError(message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", ErrCategoryAuth,
		"code", CodeAuthError,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewRTMError creates a new Remember The Milk API error with context
func NewRTMError(code int, message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", ErrCategoryRTM,
		"code", CodeRTMError,
		"rtm_code", code,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// IsResourceNotFoundError checks if the error is a resource not found error
func IsResourceNotFoundError(err error) bool {
	return errors.Is(err, ErrResourceNotFound)
}

// IsToolNotFoundError checks if the error is a tool not found error
func IsToolNotFoundError(err error) bool {
	return errors.Is(err, ErrToolNotFound)
}

// IsInvalidArgumentsError checks if the error is an invalid arguments error
func IsInvalidArgumentsError(err error) bool {
	return errors.Is(err, ErrInvalidArguments)
}

// GetErrorCategory gets the error category from an error
func GetErrorCategory(err error) string {
	if category, ok := errors.TryGetProperty(err, "category"); ok {
		if cat, ok := category.(string); ok {
			return cat
		}
	}
	return ""
}

// GetErrorCode gets the JSON-RPC error code from an error
func GetErrorCode(err error) int {
	if code, ok := errors.TryGetProperty(err, "code"); ok {
		if c, ok := code.(int); ok {
			return c
		}
	}
	return CodeInternalError // Default to internal error
}

// GetErrorProperties extracts all properties from an error
func GetErrorProperties(err error) map[string]interface{} {
	properties := make(map[string]interface{})

	// This walks the chain of wrapped errors and collects all properties
	errors.WalkErrors(err, func(e error) bool {
		if ps, ok := errors.TryGetProperties(e); ok {
			for k, v := range ps {
				// Don't overwrite existing properties (give precedence to outer errors)
				if _, exists := properties[k]; !exists {
					properties[k] = v
				}
			}
		}
		return true // Continue walking the error chain
	})

	return properties
}

// ErrorToMap converts an error to a map suitable for JSON-RPC error responses
func ErrorToMap(err error) map[string]interface{} {
	if err == nil {
		return nil
	}

	code := GetErrorCode(err)
	properties := GetErrorProperties(err)

	// Create base error map
	errorMap := map[string]interface{}{
		"code":    code,
		"message": err.Error(),
	}

	// Add data field if we have properties to include
	// Filter out internal properties that shouldn't be exposed
	dataProps := make(map[string]interface{})
	for k, v := range properties {
		// Skip internal properties
		if k != "category" && k != "code" && k != "stack" {
			dataProps[k] = v
		}
	}

	if len(dataProps) > 0 {
		errorMap["data"] = dataProps
	}

	return errorMap
}
