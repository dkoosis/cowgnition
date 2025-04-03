// file: internal/mcp/errors/utils.go
package errors

import (
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

// ToJSONRPCError converts an internal error to a *jsonrpc2.Error that can be used
// with the jsonrpc2 library for error responses.
// Example usage:
//
//	respErr := mcp/errors.ToJSONRPCError(err)
//	conn.ReplyWithError(ctx, req.ID, respErr)
func ToJSONRPCError(err error) *jsonrpc2.Error {
	if err == nil {
		return nil
	}

	// Extract error code and user-facing message
	code := GetErrorCode(err)
	message := UserFacingMessage(code)

	// Extract properties for the data field
	properties := GetErrorProperties(err)

	// Filter out internal properties and sensitive information
	dataProps := make(map[string]interface{})
	for k, v := range properties {
		if k != "category" && k != "code" && k != "stack" &&
			!containsSensitiveKeyword(k) {
			dataProps[k] = v
		}
	}

	// Create the JSON-RPC error object
	rpcErr := &jsonrpc2.Error{
		Code:    int64(code),
		Message: message,
	}

	// Add data if we have properties to include
	if len(dataProps) > 0 {
		rpcErr.Data = dataProps
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
