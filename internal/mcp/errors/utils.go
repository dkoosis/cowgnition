// Package mcp/errors defines error types, codes, and utilities for MCP and JSON-RPC.
// file: internal/mcp/errors/utils.go
package errors

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// IsResourceNotFoundError checks if the error is a resource not found error
// Example usage:
//
//	if mcp/errors.IsResourceNotFoundError(err) {
//	    // Handle resource not found case
//	}
func IsResourceNotFoundError(err error) bool {
	return errors.Is(err, ErrResourceNotFound)
}

// IsToolNotFoundError checks if the error is a tool not found error
// Example usage:
//
//	if mcp/errors.IsToolNotFoundError(err) {
//	    // Handle tool not found case
//	}
func IsToolNotFoundError(err error) bool {
	return errors.Is(err, ErrToolNotFound)
}

// IsInvalidArgumentsError checks if the error is an invalid arguments error
// Example usage:
//
//	if mcp/errors.IsInvalidArgumentsError(err) {
//	    // Handle invalid arguments case
//	}
func IsInvalidArgumentsError(err error) bool {
	return errors.Is(err, ErrInvalidArguments)
}

// GetErrorCategory gets the error category from an error
// Example usage:
//
//	category := mcp/errors.GetErrorCategory(err)
//	if category == mcp/errors.CategoryRPC {
//	    // Handle RPC errors differently
//	}
func GetErrorCategory(err error) string {
	details := errors.GetAllDetails(err)
	for _, detail := range details {
		if strings.HasPrefix(detail, "category:") {
			return strings.TrimPrefix(detail, "category:")
		}
	}
	return ""
}

// GetErrorCode gets the JSON-RPC error code from an error
// Example usage:
//
//	code := mcp/errors.GetErrorCode(err)
//	if code == mcp/errors.CodeResourceNotFound {
//	    // Handle resource not found case
//	}
func GetErrorCode(err error) int {
	details := errors.GetAllDetails(err)
	for _, detail := range details {
		if strings.HasPrefix(detail, "code:") {
			codeStr := strings.TrimPrefix(detail, "code:")
			code, parseErr := strconv.Atoi(codeStr)
			if parseErr == nil {
				return code
			}
		}
	}
	return CodeInternalError // Default to internal error
}

// GetErrorProperties extracts all properties from an error
// Example usage:
//
//	props := mcp/errors.GetErrorProperties(err)
//	resourceID, ok := props["resource_id"].(string)
func GetErrorProperties(err error) map[string]interface{} {
	properties := make(map[string]interface{})
	details := errors.GetAllDetails(err)

	// Regular expression to match "key:value" details
	re := regexp.MustCompile(`^([^:]+):(.+)$`)

	for _, detail := range details {
		matches := re.FindStringSubmatch(detail)
		if len(matches) == 3 {
			key := matches[1]
			value := matches[2]

			// Skip internal properties
			if key != "category" && key != "code" {
				// Try to convert to appropriate type
				if intVal, err := strconv.Atoi(value); err == nil {
					properties[key] = intVal
				} else if boolVal, err := strconv.ParseBool(value); err == nil {
					properties[key] = boolVal
				} else {
					properties[key] = value
				}
			}
		}
	}

	return properties
}

// ErrorToMap converts an error to a map suitable for JSON-RPC error responses.
// Example usage:
//
//	errorMap := mcp/errors.ErrorToMap(err)
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

	// Add data field if we have properties to include.
	// Filter out internal properties that shouldn't be exposed.
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

// ToJSONRPCError converts an application error to a jsonrpc2.Error.
// This is used when sending error responses via the jsonrpc2 library.
// Example usage:
//
//	respErr := cgerr.ToJSONRPCError(err)
//	conn.ReplyWithError(ctx, req.ID, respErr)
func ToJSONRPCError(err error) *jsonrpc2.Error {
	if err == nil {
		return nil
	}

	// Get error code and message from our error
	code := GetErrorCode(err)
	message := UserFacingMessage(code)

	// Create the jsonrpc2 error with basic fields
	rpcErr := &jsonrpc2.Error{
		Code:    int64(code),
		Message: message,
	}

	// Add any additional properties as data
	properties := GetErrorProperties(err)
	if len(properties) > 0 {
		// Filter out sensitive information
		safeProps := make(map[string]interface{})
		for k, v := range properties {
			if k != "category" && k != "code" && k != "stack" &&
				!containsSensitiveKeyword(k) {
				safeProps[k] = v
			}
		}

		if len(safeProps) > 0 {
			// Marshal the map to JSON
			dataJSON, marshalErr := json.Marshal(safeProps)
			if marshalErr == nil {
				// Convert to json.RawMessage and use its address
				rawMsg := json.RawMessage(dataJSON)
				rpcErr.Data = &rawMsg
			}
		}
	}

	return rpcErr
}

// containsSensitiveKeyword checks if a key might contain sensitive information.
func containsSensitiveKeyword(key string) bool {
	sensitiveKeywords := []string{"token", "password", "secret", "key", "auth", "credential"}
	for _, keyword := range sensitiveKeywords {
		if key == keyword {
			return true
		}
	}
	return false
}

// internal/mcp/errors/utils.go
// Add this function to the file:

// NewInternalError creates a new internal error with context
// Example usage:
//
//	properties := map[string]interface{}{"resource_uri": "auth://rtm"}
//	return cgerr.NewInternalError("Internal server error", err, properties)
func NewInternalError(message string, cause error, properties map[string]interface{}) error {
	if cause == nil {
		err := errors.Newf("%s", message)
		return ErrorWithDetails(err, CategoryRPC, CodeInternalError, properties)
	}

	err := errors.Wrapf(cause, "%s", message)
	return ErrorWithDetails(err, CategoryRPC, CodeInternalError, properties)
}

// internal/mcp/errors/utils.go
// Add this function to the file:

// IsAuthError checks if the error is an auth error with a specific message
// Example usage:
//
//	if cgerr.IsAuthError(err, "No valid token found") {
//	    // Handle no valid token case
//	}
func IsAuthError(err error, msgSubstr string) bool {
	category := GetErrorCategory(err)
	if category != CategoryAuth {
		return false
	}

	if msgSubstr == "" {
		return true
	}

	// Check if the error message contains the substring
	return strings.Contains(err.Error(), msgSubstr)
}
