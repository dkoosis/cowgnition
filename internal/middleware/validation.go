// file: internal/schema/validator.go
package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// SchemaSource defines where to load the schema from.
type SchemaSource struct {
	// URL is the remote location of the schema, if applicable.
	URL string
	// FilePath is the local file path of the schema, if applicable.
	FilePath string
	// Embedded is the embedded schema content, if applicable.
	Embedded []byte
}

// SchemaValidator handles loading, compiling, and validating against JSON schemas.
// It is designed to validate JSON-RPC messages against the MCP schema specification.
type SchemaValidator struct {
	// source contains the configuration for where to load the schema from.
	source SchemaSource
	// compiler is the JSONSchema compiler used to process schemas.
	compiler *jsonschema.Compiler
	// schemas maps message types to their compiled schema.
	schemas map[string]*jsonschema.Schema
	// mu protects concurrent access to the schemas map.
	mu sync.RWMutex
	// httpClient is used for remote schema fetching.
	httpClient *http.Client
}

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
		Cause:   errors.WithStack(cause), // Preserve stack trace
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(),
		},
	}
}

// NewSchemaValidator creates a new SchemaValidator with the given schema source.
func NewSchemaValidator(source SchemaSource) *SchemaValidator {
	compiler := jsonschema.NewCompiler()

	// Set up draft-2020-12 dialect
	compiler.Draft = jsonschema.Draft2020

	// Provide schemas for metaschema (required for draft-2020-12)
	compiler.AssertFormat = true
	compiler.AssertContent = true

	return &SchemaValidator{
		source:     source,
		compiler:   compiler,
		schemas:    make(map[string]*jsonschema.Schema),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Initialize loads and compiles the MCP schema definitions.
// This should be called during application startup before any validation occurs.
func (v *SchemaValidator) Initialize(ctx context.Context) error {
	schemaData, err := v.loadSchemaData(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to load schema data")
	}

	// Add the schema to the compiler
	if err := v.compiler.AddResource("mcp-schema.json", bytes.NewReader(schemaData)); err != nil {
		return NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to add schema resource to compiler",
			errors.Wrap(err, "compiler.AddResource failed"),
		).WithContext("schemaSize", len(schemaData))
	}

	// Compile the base schema
	baseSchema, err := v.compiler.Compile("mcp-schema.json")
	if err != nil {
		return NewValidationError(
			ErrSchemaCompileFailed,
			"Failed to compile base schema",
			errors.Wrap(err, "compiler.Compile failed"),
		)
	}

	// Store the schema
	v.mu.Lock()
	defer v.mu.Unlock()
	v.schemas["base"] = baseSchema

	// Compile specific message type schemas
	// For example: ClientRequest, ServerRequest, ClientNotification, etc.
	// We'll derive these from the definitions in the base schema

	// Add more specific schemas here as needed
	// This would involve extracting and compiling specific parts of the schema
	// For example:
	/*
		if err := v.compileSubSchema("ClientRequest", "#/definitions/ClientRequest"); err != nil {
			return err
		}
	*/

	return nil
}

// compileSubSchema compiles a sub-schema from the base schema.
// This uses the main schema but with a specific reference pointer.
func (v *SchemaValidator) compileSubSchema(name, pointer string) error {
	// In the santhosh-tekuri/jsonschema/v5 library, CompileWithID doesn't exist
	// Instead, we need to manually add the schema with the pointer as the ID
	subSchema, err := v.compiler.Compile(pointer)
	if err != nil {
		return NewValidationError(
			ErrSchemaCompileFailed,
			fmt.Sprintf("Failed to compile %s schema", name),
			errors.Wrap(err, fmt.Sprintf("compiler.Compile failed for %s", name)),
		).WithContext("schemaPointer", pointer)
	}

	v.schemas[name] = subSchema
	return nil
}

// loadSchemaData loads the schema data from the configured source.
func (v *SchemaValidator) loadSchemaData(ctx context.Context) ([]byte, error) {
	// Try to load from each source in order of preference

	// 1. Try embedded schema if provided
	if len(v.source.Embedded) > 0 {
		return v.source.Embedded, nil
	}

	// 2. Try local file if path is provided
	if v.source.FilePath != "" {
		data, err := os.ReadFile(v.source.FilePath)
		if err == nil {
			return data, nil
		}
		// If file read failed, log and continue to next source
		// We don't return error yet, we'll try URL next
	}

	// 3. Try URL if provided
	if v.source.URL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.source.URL, nil)
		if err != nil {
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to create HTTP request for schema URL",
				errors.Wrap(err, "http.NewRequestWithContext failed"),
			).WithContext("url", v.source.URL)
		}

		resp, err := v.httpClient.Do(req)
		if err != nil {
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to fetch schema from URL",
				errors.Wrap(err, "httpClient.Do failed"),
			).WithContext("url", v.source.URL)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				fmt.Sprintf("Failed to fetch schema: HTTP status %d", resp.StatusCode),
				nil,
			).WithContext("url", v.source.URL).
				WithContext("statusCode", resp.StatusCode)
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to read schema from HTTP response",
				errors.Wrap(err, "io.ReadAll failed"),
			).WithContext("url", v.source.URL)
		}

		return data, nil
	}

	// 4. If we get here, all sources failed
	return nil, NewValidationError(
		ErrSchemaNotFound,
		"No valid schema source configured",
		nil,
	).WithContext("sources", map[string]interface{}{
		"embedded": len(v.source.Embedded) > 0,
		"filePath": v.source.FilePath,
		"url":      v.source.URL,
	})
}

// Validate validates the given JSON data against the schema for the specified message type.
// The messageType parameter should identify which schema to use (e.g., "ClientRequest").
func (v *SchemaValidator) Validate(ctx context.Context, messageType string, data []byte) error {
	// First, ensure the data is valid JSON
	var instance interface{}
	if err := json.Unmarshal(data, &instance); err != nil {
		return NewValidationError(
			ErrInvalidJSONFormat,
			"Invalid JSON format",
			errors.Wrap(err, "json.Unmarshal failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", string(data[:min(len(data), 100)]))
	}

	// Get the schema for the message type
	v.mu.RLock()
	schema, ok := v.schemas[messageType]
	v.mu.RUnlock()

	if !ok {
		// If we don't have a specific schema for this message type, use the base schema
		v.mu.RLock()
		schema, ok = v.schemas["base"]
		v.mu.RUnlock()

		if !ok {
			return NewValidationError(
				ErrSchemaNotFound,
				fmt.Sprintf("No schema found for message type: %s", messageType),
				nil,
			).WithContext("messageType", messageType).
				WithContext("availableSchemas", getSchemaKeys(v.schemas))
		}
	}

	// Validate the instance against the schema
	err := schema.Validate(instance)
	if err != nil {
		// Convert jsonschema validation error to our custom error type
		var valErr *jsonschema.ValidationError
		if errors.As(err, &valErr) {
			return convertValidationError(valErr, messageType, data)
		}

		// For other types of errors
		return NewValidationError(
			ErrValidationFailed,
			"Schema validation failed",
			errors.Wrap(err, "schema.Validate failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", string(data[:min(len(data), 100)]))
	}

	return nil
}

// convertValidationError converts a jsonschema.ValidationError to our custom ValidationError.
func convertValidationError(valErr *jsonschema.ValidationError, messageType string, data []byte) *ValidationError {
	// Extract error details
	// In this library, the error details are in the Basic Output format described in JSON Schema spec

	// Extract schema path and instance path from the error
	var schemaPath string
	var instancePath string

	// Get basic path from the error message
	errorMsg := valErr.Error()
	if strings.Contains(errorMsg, "schema path") {
		// Try to extract schema path from the error message
		parts := strings.Split(errorMsg, "schema path:")
		if len(parts) > 1 {
			schemaPathPart := strings.TrimSpace(parts[1])
			endIdx := strings.Index(schemaPathPart, ":")
			if endIdx != -1 {
				schemaPath = schemaPathPart[:endIdx]
			} else {
				schemaPath = schemaPathPart
			}
		}
	}

	// Try to extract instance path using BasicOutput() if available
	basicOutput := valErr.BasicOutput()
	if len(basicOutput.Errors) > 0 {
		for _, errorDetail := range basicOutput.Errors {
			if errorDetail.InstanceLocation != "" {
				instancePath = errorDetail.InstanceLocation
				break
			}
		}
	}

	// Create our custom error with the extracted paths
	customErr := NewValidationError(
		ErrValidationFailed,
		valErr.Message,
		valErr,
	).WithContext("messageType", messageType).
		WithContext("dataPreview", string(data[:min(len(data), 100)]))

	customErr.SchemaPath = schemaPath
	customErr.InstancePath = instancePath

	// Add basic info about the validation error causes
	if len(valErr.Causes) > 0 {
		causes := make([]string, 0, len(valErr.Causes))
		for _, cause := range valErr.Causes {
			causes = append(causes, cause.Error())
		}
		customErr.WithContext("causes", causes)
	}

	return customErr
}

// getSchemaKeys returns the keys of the schemas map for debugging purposes.
func getSchemaKeys(schemas map[string]*jsonschema.Schema) []string {
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	return keys
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
