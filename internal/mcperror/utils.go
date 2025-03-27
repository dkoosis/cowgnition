// Package mcperror defines error types, codes, and utilities for MCP and JSON-RPC.
// file: internal/mcperror/utils.go
package mcperror

import (
	"github.com/cockroachdb/errors"
)

// IsResourceNotFoundError checks if the error is a resource not found error
// Example usage:
//
//	if mcperror.IsResourceNotFoundError(err) {
//	    // Handle resource not found case
//	}
func IsResourceNotFoundError(err error) bool {
	return errors.Is(err, ErrResourceNotFound)
}

// IsToolNotFoundError checks if the error is a tool not found error
// Example usage:
//
//	if mcperror.IsToolNotFoundError(err) {
//	    // Handle tool not found case
//	}
func IsToolNotFoundError(err error) bool {
	return errors.Is(err, ErrToolNotFound)
}

// IsInvalidArgumentsError checks if the error is an invalid arguments error
// Example usage:
//
//	if mcperror.IsInvalidArgumentsError(err) {
//	    // Handle invalid arguments case
//	}
func IsInvalidArgumentsError(err error) bool {
	return errors.Is(err, ErrInvalidArguments)
}

// GetErrorCategory gets the error category from an error
// Example usage:
//
//	category := mcperror.GetErrorCategory(err)
//	if category == mcperror.CategoryRPC {
//	    // Handle RPC errors differently
//	}
func GetErrorCategory(err error) string {
	if category, ok := errors.TryGetProperty(err, "category"); ok {
		if cat, ok := category.(string); ok {
			return cat
		}
	}
	return ""
}

// GetErrorCode gets the JSON-RPC error code from an error
// Example usage:
//
//	code := mcperror.GetErrorCode(err)
//	if code == mcperror.CodeResourceNotFound {
//	    // Handle resource not found case
//	}
func GetErrorCode(err error) int {
	if code, ok := errors.TryGetProperty(err, "code"); ok {
		if c, ok := code.(int); ok {
			return c
		}
	}
	return CodeInternalError // Default to internal error
}

// GetErrorProperties extracts all properties from an error
// Example usage:
//
//	props := mcperror.GetErrorProperties(err)
//	resourceID, ok := props["resource_id"].(string)
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
// Example usage:
//
//	errorMap := mcperror.ErrorToMap(err)
//	jsonBytes, _ := json.Marshal(errorMap)
func ErrorToMap(err error) map[string]interface{} {
	if err == nil {
		return nil
	}

	code := GetErrorCode(err)
	properties := GetErrorProperties(err)

	// Create base error map
	errorMap := map[string]interface{}{
		"code":    code,
		"message": UserFacingMessage(code),
	}

	// Add data field if we have properties to include
	// Filter out internal properties that shouldn't be exposed
	dataProps := make(map[string]interface{})
	for k, v := range properties {
		// Skip internal properties
		if k != "category" && k != "code" && k != "stack" &&
			!containsSensitiveKeyword(k) {
			dataProps[k] = v
		}
	}

	if len(dataProps) > 0 {
		errorMap["data"] = dataProps
	}

	return errorMap
}

// containsSensitiveKeyword checks if a key might contain sensitive information
func containsSensitiveKeyword(key string) bool {
	sensitiveKeywords := []string{"token", "password", "secret", "key", "auth", "credential"}
	for _, keyword := range sensitiveKeywords {
		if key == keyword {
			return true
		}
	}
	return false
}
