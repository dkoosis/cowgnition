// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// This file defines custom error types used for schema loading, compilation, and validation failures,
// providing more specific context than standard errors.
package schema

// file: internal/schema/errors.go.

import (
	"fmt"
	"strings" // Ensure strings is imported.
	"time"

	"github.com/cockroachdb/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// ErrorCode defines numeric codes for specific schema-related errors.
type ErrorCode int

// Defined validation error codes.
const (
	// ErrSchemaNotFound indicates the schema definition could not be located (e.g., file not found, definition missing).
	ErrSchemaNotFound ErrorCode = iota + 1000
	// ErrSchemaLoadFailed indicates an error occurred while reading or parsing the schema source (file, URL, embedded).
	ErrSchemaLoadFailed
	// ErrSchemaCompileFailed indicates an error occurred during the compilation of the schema by the underlying validator library.
	ErrSchemaCompileFailed
	// ErrValidationFailed indicates that the data provided did not conform to the compiled schema rules.
	ErrValidationFailed
	// ErrInvalidJSONFormat indicates the data provided for validation was not syntactically valid JSON.
	ErrInvalidJSONFormat
)

// ValidationError represents a schema validation error, providing structured details.
// It includes an error code, message, paths within the schema and instance, and the underlying cause.
type ValidationError struct {
	// Code categorizes the specific type of schema error (e.g., ErrSchemaNotFound, ErrValidationFailed).
	Code ErrorCode
	// Message provides a human-readable summary of the validation error.
	Message string
	// Cause holds the underlying error, often from the jsonschema library (like *jsonschema.ValidationError) or file I/O.
	Cause error
	// SchemaPath points to the location within the JSON schema that triggered the validation failure (e.g., "#/properties/name/type").
	SchemaPath string
	// InstancePath points to the location within the JSON data instance that failed validation (e.g., "/params/name").
	InstancePath string
	// Context contains additional key-value pairs relevant to the error (e.g., messageType, suggestion).
	Context map[string]interface{}
}

// Error implements the standard Go error interface, providing a detailed string representation.
func (e *ValidationError) Error() string {
	base := fmt.Sprintf("SchemaError [%d] %s", e.Code, e.Message)
	if e.SchemaPath != "" {
		base += fmt.Sprintf(" (schema: %s)", e.SchemaPath)
	}
	if e.InstancePath != "" {
		base += fmt.Sprintf(" (instance: %s)", e.InstancePath)
	}
	// Use %+v for detailed cause formatting (includes stack trace if available).
	if e.Cause != nil {
		base += fmt.Sprintf(": %+v", e.Cause)
	}
	return base
}

// Unwrap returns the underlying error (Cause) for compatibility with errors.Is/As.
func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// WithContext adds context information (key-value pair) to the validation error.
// Returns the modified error pointer for chaining.
func (e *ValidationError) WithContext(key string, value interface{}) *ValidationError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewValidationError creates a new ValidationError with the given code, message, and cause.
// It automatically wraps the cause error to preserve stack trace information and adds a timestamp to the context.
func NewValidationError(code ErrorCode, message string, cause error) *ValidationError {
	var wrappedCause error
	if cause != nil {
		// Ensure cause is wrapped with stack trace information using cockroachdb/errors.
		wrappedCause = errors.WithStack(cause)
	}

	return &ValidationError{
		Code:    code,
		Message: message,
		Cause:   wrappedCause,
		Context: map[string]interface{}{
			// Add timestamp for when the validation error was created.
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		},
		// InstancePath and SchemaPath are typically filled in later by convertValidationError.
	}
}

// convertValidationError converts a jsonschema.ValidationError to our custom ValidationError.
// It wraps the original error and extracts key path information and context.
func convertValidationError(valErr *jsonschema.ValidationError, messageType string, data []byte) *ValidationError {
	wrapperMessage := "Schema validation failed."
	// Create our custom error, wrapping the original jsonschema error.
	customErr := NewValidationError(
		ErrValidationFailed,
		wrapperMessage, // Use the generic wrapper message initially.
		valErr,         // Pass the original jsonschema error as the cause.
	)

	// Assign InstanceLocation and KeywordLocation from the top-level jsonschema error.
	customErr.InstancePath = valErr.InstanceLocation
	customErr.SchemaPath = valErr.KeywordLocation
	// Overwrite the generic message with the more specific one from the cause, if available.
	if valErr.Message != "" {
		customErr.Message = valErr.Message
	}

	// Add basic context.
	customErr = customErr.WithContext("messageType", messageType)
	customErr = customErr.WithContext("dataPreview", calculatePreview(data)) // Assumes calculatePreview exists.

	// Add user-friendly suggestion based on the primary error message and instance path.
	suggestion := generateErrorSuggestion(valErr.Message, valErr.InstanceLocation)
	if suggestion != "" {
		customErr = customErr.WithContext("suggestion", suggestion)
	}

	// Add nested causes to context for detailed debugging.
	if len(valErr.Causes) > 0 {
		causes := extractValidationCauses(valErr)
		if len(causes) > 0 {
			customErr = customErr.WithContext("validationCausesDetail", causes)
		}
	}

	return customErr
}

// extractValidationCauses recursively extracts relevant details from nested jsonschema validation errors.
func extractValidationCauses(valErr *jsonschema.ValidationError) []map[string]string {
	if len(valErr.Causes) == 0 {
		return nil
	}

	causes := make([]map[string]string, 0, len(valErr.Causes))

	for _, cause := range valErr.Causes {
		// Create a map for the current cause's details.
		causeMap := make(map[string]string)
		// Add details only if they are non-empty.
		if cause.InstanceLocation != "" {
			causeMap["instanceLocation"] = cause.InstanceLocation
		}
		if cause.KeywordLocation != "" {
			causeMap["keywordLocation"] = cause.KeywordLocation
		}
		if cause.Message != "" {
			causeMap["message"] = cause.Message
		}

		// Add the map to the results only if it contains details.
		if len(causeMap) > 0 {
			causes = append(causes, causeMap)
		}

		// Recursively add causes from the nested error.
		// Check needed as Cause can be nil or non-*ValidationError.
		nestedCauses := extractValidationCauses(cause)
		if len(nestedCauses) > 0 {
			causes = append(causes, nestedCauses...)
		}
	}

	if len(causes) == 0 {
		return nil // Return nil if no details were extracted.
	}
	return causes
}

// generateErrorSuggestion creates a user-friendly suggestion based on error message keywords and instance path.
// It aims to provide actionable advice beyond just stating the validation failure.
// Check common/high-impact errors first for potentially better suggestions.
// nolint:gocyclo // Complexity is inherent in handling multiple validation error patterns.
func generateErrorSuggestion(errorMsg, instancePath string) string {
	pathDisplay := instancePath
	if pathDisplay == "/" || pathDisplay == "" {
		pathDisplay = "the message root"
	} else if !strings.HasPrefix(pathDisplay, "/") {
		pathDisplay = "/" + pathDisplay // Ensure instance paths look like paths for consistency.
	}

	// --- Pattern Matching for Suggestions ---.

	// Property missing error.
	if strings.Contains(errorMsg, "required property") || strings.Contains(errorMsg, "missing properties") {
		prop := extractPropertyName(errorMsg, "required property", "missing properties")
		if prop != "[unknown]" {
			return fmt.Sprintf("Ensure the required field '%s' is provided in %s.", prop, pathDisplay)
		}
		return fmt.Sprintf("Ensure all required fields are provided in %s.", pathDisplay)
	}

	// Type mismatch error.
	if strings.Contains(errorMsg, "invalid type") || (strings.Contains(errorMsg, "expected") && strings.Contains(errorMsg, "but got")) {
		expected, actual := extractTypeInfo(errorMsg)
		if expected != "" && actual != "" {
			return fmt.Sprintf("Incorrect data type for field at %s. Expected type '%s' but received '%s'.", pathDisplay, expected, actual)
		}
		return fmt.Sprintf("Check the data type for the field at %s; it does not match the schema's expected type.", pathDisplay)
	}

	// Pattern match error.
	if strings.Contains(errorMsg, "does not match pattern") {
		pattern := extractPattern(errorMsg)
		if pattern != "" {
			return fmt.Sprintf("The value provided for %s must match the required pattern: %s.", pathDisplay, pattern)
		}
		return fmt.Sprintf("The value provided for %s does not match the required format/pattern.", pathDisplay)
	}

	// Additional properties error.
	if strings.Contains(errorMsg, "additionalProperties") {
		offendingProp := extractOffendingProperty(errorMsg)
		if offendingProp != "" {
			return fmt.Sprintf("Remove unrecognized property '%s' from the object at %s, as it's not allowed by the schema.", offendingProp, pathDisplay)
		}
		return fmt.Sprintf("Remove unrecognized properties from the object at %s.", pathDisplay)
	}

	// Enum error.
	if strings.Contains(errorMsg, "enum") || strings.Contains(errorMsg, "value must be one of") {
		values := extractEnumValues(errorMsg)
		if values != "" {
			return fmt.Sprintf("The value for %s must be one of the allowed options: %s.", pathDisplay, values)
		}
		return fmt.Sprintf("The value for %s is not one of the allowed enumerated options.", pathDisplay)
	}

	// Format error (e.g., date-time, email).
	if strings.Contains(errorMsg, "invalid format") || strings.Contains(errorMsg, "must be in format") {
		format := extractFormat(errorMsg)
		if format != "" {
			return fmt.Sprintf("The value for %s must be a valid '%s' format.", pathDisplay, format)
		}
		return fmt.Sprintf("The value for %s does not match the expected format (e.g., date-time, email, uri).", pathDisplay)
	}

	// Default suggestion if no specific pattern matched.
	return fmt.Sprintf("Review the value at %s against the MCP schema specification for correctness. Error detail: %s.", pathDisplay, errorMsg)
}

// --- Extraction Helper Functions (used by generateErrorSuggestion) ---.

// extractPropertyName attempts to extract a property name mentioned in specific error message patterns.
func extractPropertyName(msg string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if idx := strings.Index(msg, prefix); idx != -1 {
			remainder := msg[idx+len(prefix):]
			// Look for quoted property name first.
			startQuote := strings.IndexAny(remainder, `"'`)
			if startQuote != -1 {
				quoteChar := remainder[startQuote]
				// Find the closing quote of the same type.
				endQuote := strings.Index(remainder[startQuote+1:], string(quoteChar))
				if endQuote != -1 {
					return remainder[startQuote+1 : startQuote+1+endQuote]
				}
			}
			// Fallback: Look for property name after colon (simple heuristic).
			colonIdx := strings.Index(remainder, ":")
			if colonIdx != -1 {
				prop := strings.TrimSpace(strings.Split(remainder[colonIdx+1:], ",")[0]) // Get first part after colon.
				// Basic check it's not a complex structure or sentence fragment.
				if prop != "" && !strings.ContainsAny(prop, " []{}()<>=") && len(prop) < 50 { // Added length check.
					return prop
				}
			}
		}
	}
	return "[unknown]" // Return placeholder if extraction fails.
}

// extractTypeInfo extracts expected and actual types from common type mismatch error messages.
func extractTypeInfo(msg string) (expected, actual string) {
	// Example simple implementation, adaptable to specific error formats from jsonschema.
	if strings.Contains(msg, "expected") && strings.Contains(msg, "but got") {
		parts := strings.SplitN(msg, "expected", 2)
		if len(parts) < 2 {
			return "", ""
		}
		typeParts := strings.SplitN(parts[1], "but got", 2)
		if len(typeParts) < 2 {
			return "", ""
		}
		expected = strings.TrimSpace(strings.TrimSuffix(typeParts[0], ","))
		// Clean up actual type representation if needed (e.g., remove trailing punctuation).
		actual = strings.TrimSpace(strings.TrimSuffix(typeParts[1], "."))
		return expected, actual
	}
	return "", ""
}

// extractPattern attempts to extract the regex pattern from pattern mismatch error messages.
func extractPattern(msg string) string {
	// Example simple implementation.
	if idx := strings.Index(msg, "pattern "); idx != -1 {
		patternPart := strings.TrimPrefix(msg[idx:], "pattern ")
		patternPart = strings.Trim(patternPart, `'"`)     // Remove potential surrounding quotes.
		patternPart = strings.TrimRight(patternPart, ".") // Remove potential trailing period.
		return patternPart
	}
	return ""
}

// extractOffendingProperty attempts to extract the property name from additionalProperties errors.
func extractOffendingProperty(msg string) string {
	// Example simple implementation.
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

// extractEnumValues attempts to extract the allowed enum values from enum validation error messages.
func extractEnumValues(msg string) string {
	// Example simple implementation.
	if idx := strings.Index(msg, "enum"); idx != -1 {
		// Look for values within square brackets.
		if startBracket := strings.Index(msg[idx:], "["); startBracket != -1 {
			trueStart := idx + startBracket
			if endBracket := strings.Index(msg[trueStart:], "]"); endBracket != -1 {
				return strings.TrimSpace(msg[trueStart+1 : trueStart+endBracket])
			}
		}
		// Look for values after "one of:".
		if oneOfIdx := strings.Index(msg[idx:], "one of:"); oneOfIdx != -1 {
			return strings.TrimSpace(strings.TrimSuffix(msg[idx+oneOfIdx+len("one of:"):], "."))
		}
	}
	return ""
}

// extractFormat attempts to extract the required format name (e.g., date-time, email) from format validation errors.
func extractFormat(msg string) string {
	// Example simple implementation covering common formats.
	formats := []string{"date-time", "date", "time", "email", "uri", "uri-reference", "hostname", "ipv4", "ipv6", "uuid", "json-pointer", "relative-json-pointer", "regex"}
	for _, format := range formats {
		// Check for format name surrounded by single or double quotes.
		if strings.Contains(msg, "'"+format+"'") || strings.Contains(msg, `"`+format+`"`) {
			return format
		}
	}
	// Fallback check if message mentions format explicitly.
	if strings.Contains(msg, "invalid format") {
		if idx := strings.Index(msg, "format "); idx != -1 {
			remainder := msg[idx+len("format "):]
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
	return ""
}
