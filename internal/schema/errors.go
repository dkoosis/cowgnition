// file: internal/schema/errors.go
package schema

import (
	"fmt"
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
	// Cause is the underlying error, if any.
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
	if e.Cause != nil {
		base += fmt.Sprintf(": %v", e.Cause)
	}
	return base
}

// Unwrap returns the underlying error.
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
	return &ValidationError{
		Code:    code,
		Message: message,
		Cause:   errors.WithStack(cause), // Preserve stack trace.
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(),
		},
	}
}

// convertValidationError converts a jsonschema.ValidationError to our custom ValidationError.
func convertValidationError(valErr *jsonschema.ValidationError, messageType string, data []byte) *ValidationError {
	// Extract error details using BasicOutput format.
	basicOutput := valErr.BasicOutput()

	var primaryError jsonschema.BasicError
	if len(basicOutput.Errors) > 0 {
		// The 'BasicOutput' structure puts the most specific error often last,
		// but the overall error message (valErr.Message) usually summarizes the root issue.
		// Let's use the first error for path details as it might be the higher-level one.
		primaryError = basicOutput.Errors[0]
	}

	// Create our custom error with the extracted paths.
	customErr := NewValidationError(
		ErrValidationFailed,
		valErr.Message, // Use the primary message from the top-level validation error.
		valErr,         // Include the original error as cause.
	)

	// Assign paths if found from the primary basic error.
	if primaryError.KeywordLocation != "" {
		customErr.SchemaPath = primaryError.KeywordLocation // Points to schema keyword/path.
	}
	if primaryError.InstanceLocation != "" {
		customErr.InstancePath = primaryError.InstanceLocation // Points to data location.
	}

	// Fix for errcheck: Assign result back to customErr
	customErr = customErr.WithContext("messageType", messageType)
	customErr = customErr.WithContext("dataPreview", calculatePreview(data)) // Use helper here.

	// Add details about the validation error causes if available in BasicOutput.
	if len(basicOutput.Errors) > 0 {
		causes := make([]map[string]string, 0, len(basicOutput.Errors))
		for _, cause := range basicOutput.Errors {
			causes = append(causes, map[string]string{
				"instanceLocation": cause.InstanceLocation,
				"keywordLocation":  cause.KeywordLocation,
				"error":            cause.Error,
			})
		}
		// Fix for errcheck: Assign result back to customErr
		customErr = customErr.WithContext("validationErrors", causes)
	}

	return customErr
}
