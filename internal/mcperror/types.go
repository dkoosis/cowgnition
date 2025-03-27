// Package mcperror defines error types, codes, and utilities for MCP and JSON-RPC.
// file: internal/mcperror/types.go
package mcperror

import (
	"github.com/cockroachdb/errors"
)

// Base sentinel errors used throughout the application
var (
	// Resource errors
	ErrResourceNotFound = errors.WithProperty(
		errors.New("resource not found"),
		"category", CategoryResource,
		"code", CodeResourceNotFound,
	)

	// Tool errors
	ErrToolNotFound = errors.WithProperty(
		errors.New("tool not found"),
		"category", CategoryTool,
		"code", CodeToolNotFound,
	)

	// Argument errors
	ErrInvalidArguments = errors.WithProperty(
		errors.New("invalid arguments"),
		"category", CategoryRPC,
		"code", CodeInvalidParams,
	)

	// Timeout errors
	ErrTimeout = errors.WithProperty(
		errors.New("operation timed out"),
		"category", CategoryRPC,
		"code", CodeTimeoutError,
	)
)

// NewResourceError creates a new resource-related error with context
// Example usage:
//
//	properties := map[string]interface{}{"resource_uri": "auth://rtm"}
//	return mcperror.NewResourceError("Failed to load auth resource", err, properties)
func NewResourceError(message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", CategoryResource,
		"code", CodeResourceNotFound,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewToolError creates a new tool-related error with context
// Example usage:
//
//	properties := map[string]interface{}{"tool_name": "get_tasks"}
//	return mcperror.NewToolError("Failed to execute tool", err, properties)
func NewToolError(message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", CategoryTool,
		"code", CodeToolNotFound,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewInvalidArgumentsError creates a new invalid arguments error with context
// Example usage:
//
//	properties := map[string]interface{}{"argument": "frob", "expected": "string"}
//	return mcperror.NewInvalidArgumentsError("Invalid frob format", properties)
func NewInvalidArgumentsError(message string, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Newf("%s", message),
		"category", CategoryRPC,
		"code", CodeInvalidParams,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewMethodNotFoundError creates a new method not found error with context
// Example usage:
//
//	properties := map[string]interface{}{"available_methods": []string{"list_resources", "read_resource"}}
//	return mcperror.NewMethodNotFoundError("get_resources", properties)
func NewMethodNotFoundError(method string, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Newf("method '%s' not found", method),
		"category", CategoryRPC,
		"code", CodeMethodNotFound,
		"method", method,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewTimeoutError creates a new timeout error with context
// Example usage:
//
//	properties := map[string]interface{}{"operation": "read_resource", "timeout_sec": 30}
//	return mcperror.NewTimeoutError("Resource read operation timed out", properties)
func NewTimeoutError(message string, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Newf("%s", message),
		"category", CategoryRPC,
		"code", CodeTimeoutError,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewAuthError creates a new authentication error with context
// Example usage:
//
//	properties := map[string]interface{}{"auth_method": "token"}
//	return mcperror.NewAuthError("Invalid authentication token", err, properties)
func NewAuthError(message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", CategoryAuth,
		"code", CodeAuthError,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}

// NewRTMError creates a new Remember The Milk API error with context
// Example usage:
//
//	properties := map[string]interface{}{"method": "rtm.auth.getFrob"}
//	return mcperror.NewRTMError(101, "API key invalid", err, properties)
func NewRTMError(code int, message string, cause error, properties map[string]interface{}) error {
	err := errors.WithProperty(
		errors.Wrapf(cause, "%s", message),
		"category", CategoryRTM,
		"code", CodeRTMError,
		"rtm_code", code,
	)

	// Add additional properties if provided
	for key, value := range properties {
		err = errors.WithProperty(err, key, value)
	}

	return err
}
