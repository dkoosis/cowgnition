// Package mcperror defines error types, codes, and utilities for MCP and JSON-RPC.
// file: internal/mcperror/types.go
package mcperror

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

// Base sentinel errors used throughout the application
var (
	// Resource errors
	ErrResourceNotFound = errors.New("resource not found")

	// Tool errors
	ErrToolNotFound = errors.New("tool not found")

	// Argument errors
	ErrInvalidArguments = errors.New("invalid arguments")

	// Timeout errors
	ErrTimeout = errors.New("operation timed out")
)

// ErrorWithDetails adds structured details to an error while also
// attaching category and code information
func ErrorWithDetails(err error, category string, code int, details map[string]interface{}) error {
	// Mark the error with category and code using detail strings
	err = errors.WithDetail(err, fmt.Sprintf("category:%s", category))
	err = errors.WithDetail(err, fmt.Sprintf("code:%d", code))

	// Add all other details as separate detail strings
	for key, value := range details {
		err = errors.WithDetail(err, fmt.Sprintf("%s:%v", key, value))
	}

	return err
}

// NewResourceError creates a new resource-related error with context
// Example usage:
//
//	properties := map[string]interface{}{"resource_uri": "auth://rtm"}
//	return mcperror.NewResourceError("Failed to load auth resource", err, properties)
func NewResourceError(message string, cause error, properties map[string]interface{}) error {
	if cause == nil {
		err := errors.Newf("%s", message)
		err = errors.Mark(err, ErrResourceNotFound)
		return ErrorWithDetails(err, CategoryResource, CodeResourceNotFound, properties)
	}

	err := errors.Wrapf(cause, "%s", message)
	err = errors.Mark(err, ErrResourceNotFound)
	return ErrorWithDetails(err, CategoryResource, CodeResourceNotFound, properties)
}

// NewToolError creates a new tool-related error with context
// Example usage:
//
//	properties := map[string]interface{}{"tool_name": "get_tasks"}
//	return mcperror.NewToolError("Failed to execute tool", err, properties)
func NewToolError(message string, cause error, properties map[string]interface{}) error {
	if cause == nil {
		err := errors.Newf("%s", message)
		err = errors.Mark(err, ErrToolNotFound)
		return ErrorWithDetails(err, CategoryTool, CodeToolNotFound, properties)
	}

	err := errors.Wrapf(cause, "%s", message)
	err = errors.Mark(err, ErrToolNotFound)
	return ErrorWithDetails(err, CategoryTool, CodeToolNotFound, properties)
}

// NewInvalidArgumentsError creates a new invalid arguments error with context
// Example usage:
//
//	properties := map[string]interface{}{"argument": "frob", "expected": "string"}
//	return mcperror.NewInvalidArgumentsError("Invalid frob format", properties)
func NewInvalidArgumentsError(message string, properties map[string]interface{}) error {
	err := errors.Newf("%s", message)
	err = errors.Mark(err, ErrInvalidArguments)
	return ErrorWithDetails(err, CategoryRPC, CodeInvalidParams, properties)
}

// NewMethodNotFoundError creates a new method not found error with context
// Example usage:
//
//	properties := map[string]interface{}{"available_methods": []string{"list_resources", "read_resource"}}
//	return mcperror.NewMethodNotFoundError("get_resources", properties)
func NewMethodNotFoundError(method string, properties map[string]interface{}) error {
	err := errors.Newf("method '%s' not found", method)
	details := map[string]interface{}{
		"method": method,
	}
	// Merge additional properties
	for k, v := range properties {
		details[k] = v
	}
	return ErrorWithDetails(err, CategoryRPC, CodeMethodNotFound, details)
}

// NewTimeoutError creates a new timeout error with context
// Example usage:
//
//	properties := map[string]interface{}{"operation": "read_resource", "timeout_sec": 30}
//	return mcperror.NewTimeoutError("Resource read operation timed out", properties)
func NewTimeoutError(message string, properties map[string]interface{}) error {
	err := errors.Newf("%s", message)
	err = errors.Mark(err, ErrTimeout)
	return ErrorWithDetails(err, CategoryRPC, CodeTimeoutError, properties)
}

// NewAuthError creates a new authentication error with context
// Example usage:
//
//	properties := map[string]interface{}{"auth_method": "token"}
//	return mcperror.NewAuthError("Invalid authentication token", err, properties)
func NewAuthError(message string, cause error, properties map[string]interface{}) error {
	if cause == nil {
		err := errors.Newf("%s", message)
		return ErrorWithDetails(err, CategoryAuth, CodeAuthError, properties)
	}

	err := errors.Wrapf(cause, "%s", message)
	return ErrorWithDetails(err, CategoryAuth, CodeAuthError, properties)
}

// NewRTMError creates a new Remember The Milk API error with context
// Example usage:
//
//	properties := map[string]interface{}{"method": "rtm.auth.getFrob"}
//	return mcperror.NewRTMError(101, "API key invalid", err, properties)
func NewRTMError(code int, message string, cause error, properties map[string]interface{}) error {
	details := map[string]interface{}{
		"rtm_code": code,
	}
	// Merge additional properties
	for k, v := range properties {
		details[k] = v
	}

	if cause == nil {
		err := errors.Newf("%s", message)
		return ErrorWithDetails(err, CategoryRTM, CodeRTMError, details)
	}

	err := errors.Wrapf(cause, "%s", message)
	return ErrorWithDetails(err, CategoryRTM, CodeRTMError, details)
}
