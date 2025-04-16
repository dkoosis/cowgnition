// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation_errors.go

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// --- JSON-RPC Error Response Creation Helpers ---
// createParseErrorResponse, createInvalidRequestErrorResponse,
// createValidationErrorResponse, createInternalErrorResponse,
// createGenericErrorResponseWithData remain the same as previous refactor.

// createParseErrorResponse creates a standard JSON-RPC -32700 error response.
func createParseErrorResponse(id interface{}, parseErr error) ([]byte, error) {
	data := map[string]interface{}{
		"details":    "The received message could not be parsed as valid JSON.",
		"suggestion": "Check JSON syntax, ensure quotes and brackets are balanced.",
	}
	if parseErr != nil {
		data["cause"] = parseErr.Error()
	}
	return createGenericErrorResponseWithData(id, transport.JSONRPCParseError, "Parse error", data)
}

// createInvalidRequestErrorResponse creates a standard JSON-RPC -32600 error response.
func createInvalidRequestErrorResponse(id interface{}, requestErr error) ([]byte, error) {
	data := map[string]interface{}{
		"details":    "The JSON is valid, but it's not a valid JSON-RPC Request object.",
		"suggestion": "Ensure the message includes 'jsonrpc': '2.0' and a valid 'method' field. If expecting a response, include a valid 'id'.",
	}
	if requestErr != nil {
		data["cause"] = requestErr.Error()
	}
	return createGenericErrorResponseWithData(id, transport.JSONRPCInvalidRequest, "Invalid Request", data)
}

// createValidationErrorResponse creates a JSON-RPC error response (-32600 or -32602) from a schema.ValidationError.
func createValidationErrorResponse(id interface{}, validationErr error) ([]byte, error) {
	var schemaValErr *schema.ValidationError
	if !errors.As(validationErr, &schemaValErr) {
		return createInternalErrorResponse(id)
	}

	code := transport.JSONRPCInvalidRequest // Default code (-32600)
	message := "Invalid Request"

	if schemaValErr.InstancePath != "" && (strings.HasPrefix(schemaValErr.InstancePath, "/params") ||
		strings.HasPrefix(schemaValErr.InstancePath, "params") ||
		strings.Contains(schemaValErr.InstancePath, "/params/") ||
		strings.Contains(schemaValErr.InstancePath, ".params.")) {
		code = transport.JSONRPCInvalidParams // -32602
		message = "Invalid params"
	}

	errorData := map[string]interface{}{
		"validationPath":  schemaValErr.InstancePath,
		"schemaPath":      schemaValErr.SchemaPath,
		"validationError": schemaValErr.Message,
	}

	if schemaValErr.Context != nil {
		for k, v := range schemaValErr.Context {
			if _, exists := errorData[k]; !exists {
				errorData["context_"+k] = v
			}
			if k == "suggestion" {
				errorData["suggestion"] = v
			}
		}
	}

	if _, exists := errorData["suggestion"]; !exists {
		errorData["suggestion"] = generateDefaultSuggestion(schemaValErr) // Call refactored suggestion generator
	}

	return createGenericErrorResponseWithData(id, code, message, errorData)
}

// createInternalErrorResponse creates a standard JSON-RPC -32603 error response.
func createInternalErrorResponse(id interface{}) ([]byte, error) {
	data := map[string]interface{}{
		"details":    "An unexpected internal server error occurred.",
		"suggestion": "Please report this error to the server administrator or developer.",
	}
	return createGenericErrorResponseWithData(id, transport.JSONRPCInternalError, "Internal error", data)
}

// createGenericErrorResponseWithData creates a standard JSON-RPC error response structure.
func createGenericErrorResponseWithData(id interface{}, code int, message string, data interface{}) ([]byte, error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	if data != nil {
		dataMap, isMap := data.(map[string]interface{})
		if (isMap && len(dataMap) > 0) || !isMap {
			response["error"].(map[string]interface{})["data"] = data
		}
	}
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Wrap(err, "CRITICAL: failed to marshal generic error response")
	}
	return responseBytes, nil
}

// --- Suggestion Generation (Refactored Again for Complexity) ---

// suggestionFunc defines the signature for helper functions that generate suggestions.
type suggestionFunc func(msg, path string) string

// generateDefaultSuggestion provides basic suggestions based on common validation error patterns.
// Reduced complexity by iterating through a list of suggestion functions.
func generateDefaultSuggestion(validationErr *schema.ValidationError) string {
	msg := validationErr.Message
	path := validationErr.InstancePath
	if path == "" {
		path = "root" // Use "root" if path is empty
	}

	// List of suggestion functions to try in order.
	suggestionGenerators := []suggestionFunc{
		suggestForRequired,
		suggestForTypeMismatch,
		suggestForPattern,
		suggestForNumericLimit,
		suggestForItemLimit,
		suggestForFormat,
		suggestForEnum,
		suggestForAdditionalProperties,
		// Add more specific suggestion functions here if needed
	}

	// Iterate and return the first non-empty suggestion.
	for _, suggester := range suggestionGenerators {
		if suggestion := suggester(msg, path); suggestion != "" {
			return suggestion
		}
	}

	// Generic fallback suggestion if no specific helper matched.
	return fmt.Sprintf("Review the value at '%s' against the MCP schema specification for correctness.", path)
}

// --- Suggestion Generation Detail Extractors (remain the same) ---
// extractPropertyName, extractTypeInfo, extractPattern, extractNumericLimit,
// extractFormat, extractEnumValues, extractOffendingProperty remain the same

func extractPropertyName(msg string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if idx := strings.Index(msg, prefix); idx != -1 {
			remainder := msg[idx+len(prefix):]
			startQuote := strings.IndexAny(remainder, `"'`)
			if startQuote != -1 {
				quoteChar := remainder[startQuote]
				endQuote := strings.Index(remainder[startQuote+1:], string(quoteChar))
				if endQuote != -1 {
					return remainder[startQuote+1 : startQuote+1+endQuote]
				}
			}
		}
	}
	return "[unknown]"
}

func extractTypeInfo(msg string) (expectedType, actualType string) {
	if strings.Contains(msg, "expected") && strings.Contains(msg, "but got") {
		parts := strings.SplitN(msg, "expected", 2)
		if len(parts) < 2 {
			return "", ""
		}
		typeParts := strings.SplitN(parts[1], "but got", 2)
		if len(typeParts) < 2 {
			return "", ""
		}
		expectedType = strings.TrimSpace(strings.TrimSuffix(typeParts[0], ","))
		actualType = strings.TrimSpace(strings.TrimSuffix(typeParts[1], "."))
		return expectedType, actualType
	}
	return "", ""
}

func extractPattern(msg string) string {
	if idx := strings.Index(msg, "pattern"); idx != -1 {
		remainder := msg[idx+len("pattern"):]
		startQuote := strings.IndexAny(remainder, `"'`)
		if startQuote != -1 {
			quoteChar := remainder[startQuote]
			endQuote := strings.Index(remainder[startQuote+1:], string(quoteChar))
			if endQuote != -1 {
				return remainder[startQuote+1 : startQuote+1+endQuote]
			}
		}
		if colonIdx := strings.Index(remainder, ":"); colonIdx != -1 {
			patternStr := strings.TrimSpace(remainder[colonIdx+1:])
			if strings.HasPrefix(patternStr, "^") || strings.HasSuffix(patternStr, "$") {
				return patternStr
			}
		}
	}
	return ""
}

func extractNumericLimit(msg string) string {
	keywords := []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems"}
	for _, keyword := range keywords {
		if idx := strings.Index(msg, keyword); idx != -1 {
			remainder := msg[idx+len(keyword):]
			numStr := ""
			foundDigit := false
			for _, r := range remainder {
				if r >= '0' && r <= '9' {
					numStr += string(r)
					foundDigit = true
				} else if foundDigit {
					break
				}
			}
			if numStr != "" {
				return numStr
			}
		}
	}
	return ""
}

func extractFormat(msg string) string {
	formats := []string{"date-time", "date", "time", "email", "uri", "uri-reference", "hostname", "ipv4", "ipv6", "uuid", "json-pointer", "relative-json-pointer", "regex"}
	for _, format := range formats {
		if strings.Contains(msg, "'"+format+"'") || strings.Contains(msg, `"`+format+`"`) {
			return format
		}
	}
	return ""
}

func extractEnumValues(msg string) string {
	if idx := strings.Index(msg, "["); idx != -1 {
		endIdx := strings.Index(msg[idx:], "]")
		if endIdx != -1 {
			return strings.TrimSpace(msg[idx+1 : idx+endIdx])
		}
	}
	if idx := strings.Index(msg, "one of:"); idx != -1 {
		return strings.TrimSpace(msg[idx+len("one of:"):])
	}
	return ""
}

func extractOffendingProperty(msg string) string {
	if strings.Contains(msg, "additionalProperties") && strings.Contains(msg, "not allowed") {
		startQuote := strings.IndexAny(msg, `"'`)
		if startQuote != -1 {
			quoteChar := msg[startQuote]
			endQuote := strings.Index(msg[startQuote+1:], string(quoteChar))
			if endQuote != -1 {
				return msg[startQuote+1 : startQuote+1+endQuote]
			}
		}
	}
	return ""
}

// --- Individual Suggestion Helper Functions (remain the same) ---

func suggestForRequired(msg, path string) string {
	if strings.Contains(msg, "required property") || strings.Contains(msg, "missing properties") {
		prop := extractPropertyName(msg, "required property", "missing properties")
		return fmt.Sprintf("Ensure the required field '%s' is provided at path '%s'.", prop, path)
	}
	return ""
}

func suggestForTypeMismatch(msg, path string) string {
	if strings.Contains(msg, "invalid type") || (strings.Contains(msg, "expected") && strings.Contains(msg, "but got")) {
		expectedType, actualType := extractTypeInfo(msg)
		if expectedType != "" && actualType != "" {
			return fmt.Sprintf("Field at '%s' has incorrect type. Expected '%s' but received '%s'.", path, expectedType, actualType)
		}
		return fmt.Sprintf("Check the data type for the field at '%s'. Review the schema for the expected type.", path)
	}
	return ""
}

func suggestForPattern(msg, path string) string {
	if strings.Contains(msg, "does not match pattern") || strings.Contains(msg, "pattern mismatch") {
		pattern := extractPattern(msg)
		if pattern != "" {
			return fmt.Sprintf("The value at '%s' does not match required pattern: %s.", path, pattern)
		}
		return fmt.Sprintf("The value at '%s' does not match the required pattern. Consult the schema or documentation.", path)
	}
	return ""
}

func suggestForNumericLimit(msg, path string) string {
	if strings.Contains(msg, "minimum") || strings.Contains(msg, "maximum") { // Covers min/max value and length
		limit := extractNumericLimit(msg)
		if limit != "" {
			if strings.Contains(msg, "minimum") || strings.Contains(msg, "minLength") {
				return fmt.Sprintf("The value/length at '%s' is below the minimum allowed value/length of %s.", path, limit)
			}
			return fmt.Sprintf("The value/length at '%s' exceeds the maximum allowed value/length of %s.", path, limit)
		}
		return fmt.Sprintf("The value or length at '%s' is outside the allowed range/limits.", path)
	}
	return ""
}

func suggestForItemLimit(msg, path string) string {
	if strings.Contains(msg, "minItems") || strings.Contains(msg, "maxItems") {
		limit := extractNumericLimit(msg)
		if limit != "" {
			if strings.Contains(msg, "minItems") {
				return fmt.Sprintf("The array at '%s' must contain at least %s items.", path, limit)
			}
			return fmt.Sprintf("The array at '%s' must contain no more than %s items.", path, limit)
		}
		return fmt.Sprintf("The array at '%s' has an incorrect number of items.", path)
	}
	return ""
}

func suggestForFormat(msg, path string) string {
	if strings.Contains(msg, "format") {
		format := extractFormat(msg)
		if format != "" {
			return fmt.Sprintf("The value at '%s' must be in '%s' format.", path, format)
		}
		return fmt.Sprintf("The value at '%s' does not match the expected format (e.g., date-time, email, uri).", path)
	}
	return ""
}

func suggestForEnum(msg, path string) string {
	if strings.Contains(msg, "enum") {
		values := extractEnumValues(msg)
		if values != "" {
			return fmt.Sprintf("The value at '%s' must be one of: %s.", path, values)
		}
		return fmt.Sprintf("The value at '%s' must be one of the allowed enumerated values.", path)
	}
	return ""
}

func suggestForAdditionalProperties(msg, path string) string {
	if strings.Contains(msg, "additionalProperties") {
		offendingProp := extractOffendingProperty(msg)
		if offendingProp != "" {
			return fmt.Sprintf("The object at '%s' contains an unexpected property '%s' which is not allowed by the schema.", path, offendingProp)
		}
		return fmt.Sprintf("The object at '%s' contains additional properties that are not allowed by the schema.", path)
	}
	return ""
}
