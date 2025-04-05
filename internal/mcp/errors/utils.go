// Package errors defines error types, codes, and utilities for MCP and JSON-RPC.
// file: internal/mcp/errors/utils.go.
package errors

import (
	"encoding/json"
	// Removed fmt import as it was only used in the removed ErrorWithDetails placeholder.
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/sourcegraph/jsonrpc2"
	// The necessary sentinel errors (ErrResourceNotFound etc.) and constants (CategoryRPC, CodeInternalError etc.)
	// are assumed to be defined in other files within this package (like types.go and codes.go) and are accessible here.
)

// --- Generic Error Creation Helpers ---.

// New creates a new error with a stack trace using cockroachdb/errors.
// Use this for errors where specific codes/categories aren't immediately needed,
// or when you plan to add details later.
func New(message string) error {
	return errors.New(message)
}

// Newf creates a new formatted error with a stack trace using cockroachdb/errors.
func Newf(format string, args ...interface{}) error {
	return errors.Newf(format, args...)
}

// Wrap wraps an existing error with a message and stack trace using cockroachdb/errors.
// Preserves the original error cause.
func Wrap(cause error, message string) error {
	return errors.Wrap(cause, message)
}

// Wrapf wraps an existing error with a formatted message and stack trace using cockroachdb/errors.
// Preserves the original error cause.
func Wrapf(cause error, format string, args ...interface{}) error {
	return errors.Wrapf(cause, format, args...)
}

// --- Error Checking Helpers ---.

// IsResourceNotFoundError checks if the error is a resource not found error.
// Example usage:.
//
//	if errors.IsResourceNotFoundError(err) {
//	    // Handle resource not found case.
//	}
func IsResourceNotFoundError(err error) bool {
	// Assumes ErrResourceNotFound is defined in this package (likely types.go).
	return errors.Is(err, ErrResourceNotFound)
}

// IsToolNotFoundError checks if the error is a tool not found error.
// Example usage:.
//
//	if errors.IsToolNotFoundError(err) {
//	    // Handle tool not found case.
//	}
func IsToolNotFoundError(err error) bool {
	// Assumes ErrToolNotFound is defined in this package (likely types.go).
	return errors.Is(err, ErrToolNotFound)
}

// IsInvalidArgumentsError checks if the error is an invalid arguments error.
// Example usage:.
//
//	if errors.IsInvalidArgumentsError(err) {
//	    // Handle invalid arguments case.
//	}
func IsInvalidArgumentsError(err error) bool {
	// Assumes ErrInvalidArguments is defined in this package (likely types.go).
	return errors.Is(err, ErrInvalidArguments)
}

// IsAuthError checks if the error is an auth error with a specific message.
// Example usage:.
//
//	if errors.IsAuthError(err, "No valid token found") {
//	    // Handle no valid token case.
//	}
func IsAuthError(err error, msgSubstr string) bool {
	category := GetErrorCategory(err)
	// Assumes CategoryAuth is defined in this package (likely codes.go).
	if category != CategoryAuth {
		return false
	}

	if msgSubstr == "" {
		return true
	}

	// Check if the error message contains the substring.
	return strings.Contains(err.Error(), msgSubstr)
}

// --- Context Extraction Helpers ---.

// GetErrorCategory gets the error category from an error.
// It expects the category to be stored as a detail string "category:VALUE".
// Example usage:.
//
//	category := errors.GetErrorCategory(err)
//	if category == errors.CategoryRPC { // Assumes CategoryRPC is defined (likely codes.go).
//	    // Handle RPC errors differently.
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

// GetErrorCode gets the JSON-RPC error code from an error.
// It expects the code to be stored as a detail string "code:VALUE".
// Example usage:.
//
//	code := errors.GetErrorCode(err)
//	if code == errors.CodeResourceNotFound { // Assumes CodeResourceNotFound is defined (likely codes.go).
//	    // Handle resource not found case.
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
	// Assumes CodeInternalError is defined in this package (likely codes.go).
	return CodeInternalError // Default to internal error.
}

// GetErrorProperties extracts all properties from an error.
// It expects properties to be stored as detail strings "key:value".
// Example usage:.
//
//	props := errors.GetErrorProperties(err)
//	resourceID, ok := props["resource_id"].(string)
func GetErrorProperties(err error) map[string]interface{} {
	properties := make(map[string]interface{})
	details := errors.GetAllDetails(err)

	// Regular expression to match "key:value" details.
	re := regexp.MustCompile(`^([^:]+):(.+)$`)

	for _, detail := range details {
		matches := re.FindStringSubmatch(detail)
		if len(matches) == 3 {
			key := matches[1]
			value := matches[2]

			// Skip internal properties used for code/category extraction.
			if key != "category" && key != "code" {
				// Try to convert to appropriate type.
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

// --- Conversion Helpers ---.

// ErrorToMap converts an error to a map suitable for JSON-RPC error responses.
// Example usage:.
//
//	errorMap := errors.ErrorToMap(err)
//	jsonBytes, _ := json.Marshal(errorMap)
func ErrorToMap(err error) map[string]interface{} {
	if err == nil {
		return nil
	}

	code := GetErrorCode(err)
	properties := GetErrorProperties(err)

	// Create base error map.
	// Assumes UserFacingMessage is defined elsewhere in this package.
	errorMap := map[string]interface{}{
		"code":    code,
		"message": UserFacingMessage(code),
	}

	// Add data field if we have properties to include.
	// Filter out internal properties that shouldn't be exposed.
	dataProps := make(map[string]interface{})
	for k, v := range properties {
		// Skip internal properties and potentially sensitive ones.
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
// Example usage:.
//
//	respErr := errors.ToJSONRPCError(err)
//	conn.ReplyWithError(ctx, req.ID, respErr)
func ToJSONRPCError(err error) *jsonrpc2.Error {
	if err == nil {
		return nil
	}

	// Get error code and message from our error.
	code := GetErrorCode(err)
	// Assumes UserFacingMessage is defined elsewhere in this package.
	message := UserFacingMessage(code)

	// Create the jsonrpc2 error with basic fields.
	rpcErr := &jsonrpc2.Error{
		Code:    int64(code),
		Message: message,
	}

	// Add any additional properties as data.
	properties := GetErrorProperties(err)
	if len(properties) > 0 {
		// Filter out sensitive information.
		safeProps := make(map[string]interface{})
		for k, v := range properties {
			if k != "category" && k != "code" && k != "stack" &&
				!containsSensitiveKeyword(k) {
				safeProps[k] = v
			}
		}

		if len(safeProps) > 0 {
			// Marshal the map to JSON.
			dataJSON, marshalErr := json.Marshal(safeProps)
			if marshalErr == nil {
				// Convert to json.RawMessage and use its address.
				rawMsg := json.RawMessage(dataJSON)
				rpcErr.Data = &rawMsg
			}
			// Note: Consider logging marshalErr if it occurs.
		}
	}

	return rpcErr
}

// containsSensitiveKeyword checks if a key might contain sensitive information.
// Adapt this list based on your application's specific keys.
func containsSensitiveKeyword(key string) bool {
	// Use lower case for case-insensitive comparison.
	lowerKey := strings.ToLower(key)
	sensitiveKeywords := []string{"token", "password", "secret", "key", "auth", "credential", "session", "cookie"}
	for _, keyword := range sensitiveKeywords {
		if strings.Contains(lowerKey, keyword) {
			return true
		}
	}
	return false
}

// --- Specific Error Creation Helpers ---.

// NOTE: The ErrorWithDetails function is NOT defined here.
// It is assumed to be defined in types.go as indicated by the compiler error.
// This NewInternalError function relies on that external definition.

// NewInternalError creates a new internal error with context.
// It automatically assigns CategoryRPC and CodeInternalError using the
// ErrorWithDetails function defined elsewhere in the package (likely types.go).
// Example usage:.
//
//	properties := map[string]interface{}{"operation": "read_resource"}
//	return errors.NewInternalError("Internal server error", err, properties)
func NewInternalError(message string, cause error, properties map[string]interface{}) error {
	var baseErr error
	if cause == nil {
		// Use the generic helper for base creation.
		baseErr = New(message)
	} else {
		// Use the generic helper for wrapping.
		baseErr = Wrap(cause, message)
	}

	// Assumes CategoryRPC and CodeInternalError are defined (likely codes.go).
	// Assumes ErrorWithDetails is defined (likely types.go) and attaches details correctly.
	return ErrorWithDetails(baseErr, CategoryRPC, CodeInternalError, properties)
}

// NO placeholder definitions below this line anymore.
