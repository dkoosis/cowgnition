// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

// file: internal/schema/errors.go.

import (
	"fmt"
	"strings" // Ensure strings is imported.
	"time"

	"github.com/cockroachdb/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// ErrorCode defines validation error codes.
type ErrorCode int

// Defined validation error codes.
const (
	ErrSchemaNotFound ErrorCode = iota + 1000
	ErrSchemaLoadFailed
	ErrSchemaCompileFailed
	ErrValidationFailed
	ErrInvalidJSONFormat
)

// ValidationError represents a schema validation error.
type ValidationError struct {
	// Code is the numeric error code.
	Code ErrorCode
	// Message is a human-readable error message.
	Message string
	// Cause is the underlying error, if any. Should be *jsonschema.ValidationError when Code is ErrValidationFailed.
	Cause error
	// SchemaPath identifies the specific part of the schema that was violated.
	SchemaPath string
	// InstancePath identifies the specific part of the validated instance that violated the schema.
	InstancePath string
	// Context contains additional error context.
	Context map[string]interface{}
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	base := fmt.Sprintf("[%d] %s", e.Code, e.Message)
	if e.SchemaPath != "" {
		base += fmt.Sprintf(" (schema path: %s)", e.SchemaPath)
	}
	if e.InstancePath != "" {
		base += fmt.Sprintf(" (instance path: %s)", e.InstancePath)
	}
	// Use %+v for detailed cause formatting from cockroachdb/errors.
	if e.Cause != nil {
		base += fmt.Sprintf(": %+v", e.Cause)
	}
	return base
}

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// WithContext adds context information to the validation error.
func (e *ValidationError) WithContext(key string, value interface{}) *ValidationError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewValidationError creates a new ValidationError.
func NewValidationError(code ErrorCode, message string, cause error) *ValidationError {
	// Ensure cause is wrapped with stack trace if it's not nil and not already wrapped.
	// cockroaches/errors.WithStack is idempotent, so calling it on an already wrapped error is safe.
	wrappedCause := cause
	if cause != nil {
		wrappedCause = errors.WithStack(cause)
	}

	return &ValidationError{
		Code:    code,
		Message: message,
		Cause:   wrappedCause, // Use the potentially wrapped cause.
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(), // Use UTC for consistency.
		},
		// InstancePath and SchemaPath are typically set later from the underlying error.
	}
}

// convertValidationError converts a jsonschema.ValidationError to our custom ValidationError.
// It wraps the original error and extracts key path information.
func convertValidationError(valErr *jsonschema.ValidationError, messageType string, data []byte) *ValidationError {
	wrapperMessage := "Schema validation failed."
	// Create our custom error, wrapping the original jsonschema error.
	customErr := NewValidationError(
		ErrValidationFailed,
		wrapperMessage, // Use the generic wrapper message.
		valErr,         // Pass the original jsonschema error as the cause.
	)

	// Assign InstanceLocation and KeywordLocation from the top-level jsonschema error.
	// These might be empty for some error types (like type mismatch), as observed in tests.
	customErr.InstancePath = valErr.InstanceLocation
	customErr.SchemaPath = valErr.KeywordLocation

	// Add basic context.
	customErr = customErr.WithContext("messageType", messageType)
	customErr = customErr.WithContext("dataPreview", calculatePreview(data)) // Assumes calculatePreview exists.
	// Store the original top-level message from jsonschema in context for reference.
	customErr = customErr.WithContext("originalMessage", valErr.Message)

	// Add user-friendly suggestion based on the primary error message and instance path.
	// Generate suggestion based on the original jsonschema error message.
	suggestion := generateErrorSuggestion(valErr.Message, valErr.InstanceLocation)
	if suggestion != "" {
		customErr = customErr.WithContext("suggestion", suggestion)
	}

	// Optionally, add nested causes to context if needed for detailed debugging,
	// but the primary message check should now target the unwrapped cause directly in tests.
	if len(valErr.Causes) > 0 {
		causes := extractValidationCauses(valErr) // Use existing helper.
		if len(causes) > 0 {
			customErr = customErr.WithContext("validationCausesDetail", causes)
		}
	}

	return customErr
}

// extractValidationCauses extracts nested error causes from a ValidationError.
func extractValidationCauses(valErr *jsonschema.ValidationError) []map[string]string {
	// Base case: If there are no causes, return nil.
	if len(valErr.Causes) == 0 {
		return nil
	}

	// Pre-allocate slice with capacity.
	causes := make([]map[string]string, 0, len(valErr.Causes))

	// Iterate through the nested causes.
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
	}

	// Return nil if no details were extracted (e.g., all nested causes were empty).
	if len(causes) == 0 {
		return nil
	}
	return causes
}

// generateErrorSuggestion creates a user-friendly suggestion based on error details.
//
//nolint:gocyclo // Suppressing complexity warning; function generates suggestions based on many error patterns.
func generateErrorSuggestion(errorMsg, instancePath string) string {
	pathDisplay := instancePath
	if pathDisplay == "/" || pathDisplay == "" {
		pathDisplay = "the message root"
	} else if !strings.HasPrefix(pathDisplay, "/") {
		pathDisplay = "/" + pathDisplay // Ensure instance paths look like paths.
	}

	// Use keywords from the error message to provide targeted suggestions.
	// Property missing error.
	if strings.Contains(errorMsg, "required property") || strings.Contains(errorMsg, "missing properties") {
		prop := extractPropertyName(errorMsg, "required property", "missing properties")
		// If prop is found, be specific, otherwise use the general path.
		if prop != "[unknown]" {
			return fmt.Sprintf("Ensure the required field '%s' is provided in %s.", prop, pathDisplay)
		}
		return fmt.Sprintf("Ensure all required fields are provided in %s.", pathDisplay)
	}

	// Type mismatch error.
	if strings.Contains(errorMsg, "invalid type") || (strings.Contains(errorMsg, "expected") && strings.Contains(errorMsg, "but got")) {
		expected, actual := extractTypeInfo(errorMsg) // Assuming extractTypeInfo exists.
		if expected != "" && actual != "" {
			return fmt.Sprintf("Incorrect data type for %s. Expected '%s' but received '%s'.", pathDisplay, expected, actual)
		}
		return fmt.Sprintf("Check that the data type for %s matches the schema specification.", pathDisplay)
	}

	// Pattern match error.
	if strings.Contains(errorMsg, "does not match pattern") {
		pattern := extractPattern(errorMsg) // Assuming extractPattern exists.
		if pattern != "" {
			return fmt.Sprintf("The value for %s must match the required pattern: %s.", pathDisplay, pattern)
		}
		return fmt.Sprintf("The value for %s does not match the required format.", pathDisplay)
	}

	// Additional properties error.
	if strings.Contains(errorMsg, "additionalProperties") {
		offendingProp := extractOffendingProperty(errorMsg) // Assuming extractOffendingProperty exists.
		if offendingProp != "" {
			return fmt.Sprintf("Remove unrecognized property '%s' from the object at %s.", offendingProp, pathDisplay)
		}
		return fmt.Sprintf("Remove unrecognized properties from the object at %s.", pathDisplay)
	}

	// Enum error.
	if strings.Contains(errorMsg, "enum") || strings.Contains(errorMsg, "value must be one of") {
		values := extractEnumValues(errorMsg) // Assuming extractEnumValues exists.
		if values != "" {
			return fmt.Sprintf("The value for %s must be one of the allowed values: %s.", pathDisplay, values)
		}
		return fmt.Sprintf("The value for %s is not one of the allowed options.", pathDisplay)
	}

	// Format error (e.g., date-time, email).
	if strings.Contains(errorMsg, "invalid format") || strings.Contains(errorMsg, "must be in format") {
		format := extractFormat(errorMsg) // Assuming extractFormat exists.
		if format != "" {
			return fmt.Sprintf("The value for %s must be a valid '%s' format.", pathDisplay, format)
		}
		return fmt.Sprintf("The value for %s does not match the required format.", pathDisplay)
	}

	// Default suggestion if no specific pattern matched.
	return fmt.Sprintf("Review the value at %s against the MCP schema specification for correctness.", pathDisplay)
}

// extractPropertyName extracts property name from error messages.
func extractPropertyName(msg string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if idx := strings.Index(msg, prefix); idx != -1 {
			remainder := msg[idx+len(prefix):]
			// Look for quoted property name first.
			startQuote := strings.IndexAny(remainder, `"'`)
			if startQuote != -1 {
				quoteChar := remainder[startQuote]
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
				if prop != "" && !strings.ContainsAny(prop, " []{}()<>=") {
					return prop
				}
			}
		}
	}
	return "[unknown]"
}

// --- Stubs or implementations for other extraction helpers used in generateErrorSuggestion ---.
// (Ensure these helpers are implemented robustly).

func extractTypeInfo(msg string) (expected, actual string) {
	// Example simple implementation.
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
		// Clean up actual type representation if needed.
		actual = strings.TrimSpace(strings.TrimSuffix(typeParts[1], "."))
		return expected, actual
	}
	return "", ""
}

func extractPattern(msg string) string {
	// Example simple implementation.
	if idx := strings.Index(msg, "pattern "); idx != -1 {
		patternPart := strings.TrimPrefix(msg[idx:], "pattern ")
		// Remove potential surrounding quotes.
		patternPart = strings.Trim(patternPart, `'"`)
		// Remove potential trailing punctuation.
		patternPart = strings.TrimRight(patternPart, ".,;:!?")
		return patternPart
	}
	return ""
}

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
			// Trim potential trailing period.
			return strings.TrimSpace(strings.TrimSuffix(msg[idx+oneOfIdx+len("one of:"):], "."))
		}
	}
	return ""
}

func extractFormat(msg string) string {
	// Example simple implementation.
	formats := []string{"date-time", "date", "time", "email", "uri", "uri-reference", "hostname", "ipv4", "ipv6", "uuid", "json-pointer", "relative-json-pointer", "regex"}
	for _, format := range formats {
		if strings.Contains(msg, "'"+format+"'") || strings.Contains(msg, `"`+format+`"`) {
			return format
		}
	}
	if strings.Contains(msg, "invalid format") {
		// Try to extract format name if quoted after 'format'.
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
