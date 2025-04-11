// File: internal/schema/validator_test.go.
package schema

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a temporary schema file for testing.
func createTempSchemaFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test_schema.json")
	err := os.WriteFile(path, []byte(content), 0600)
	require.NoError(t, err, "Failed to create temporary schema file.")
	return path
}

// Minimal valid JSON Schema for basic tests.
const minValidSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "TestSchema",
  "type": "object",
  "properties": {
    "jsonrpc": { "const": "2.0" },
    "method": { "type": "string" },
    "id": { "type": ["string", "integer", "null"] }
  },
  "required": ["jsonrpc", "method"]
}`

// Minimal invalid JSON Schema (syntax error).
const invalidSchemaSyntax = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "InvalidSchema",
  "type": "object",
  "properties": {
    "jsonrpc": { "const": "2.0" },
` // Missing closing brace.

// Valid JSON message conforming to minValidSchema.
const validMessage = `{"jsonrpc": "2.0", "method": "ping", "id": 1}`

// Invalid JSON message (missing required 'method').
const invalidMessageMissingMethod = `{"jsonrpc": "2.0", "id": 1}`

// Invalid JSON message (wrong type for 'method').
const invalidMessageWrongType = `{"jsonrpc": "2.0", "method": 123, "id": 1}`

// Invalid JSON syntax message.
const invalidJsonSyntaxMessage = `{"jsonrpc": "2.0", "method": "ping"` // Missing closing brace.

// Test NewSchemaValidator creation.
func TestNewSchemaValidator(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	assert.NotNil(t, validator, "Validator should not be nil.")
	assert.NotNil(t, validator.compiler, "Compiler should not be nil.")
	assert.NotNil(t, validator.schemas, "Schemas map should not be nil.")
	assert.NotNil(t, validator.httpClient, "HTTP client should not be nil.")
	assert.NotNil(t, validator.logger, "Logger should not be nil.")
	assert.Equal(t, source, validator.source, "Source should be stored.")
}

// Test SchemaValidator Initialization Success (Embedded).
func TestSchemaValidator_Initialize_Success_Embedded(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialize should succeed with valid embedded schema.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetLoadDuration(), "Load duration should be recorded.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	// Check if the base schema was compiled.
	validator.mu.RLock()
	_, ok := validator.schemas["base"]
	validator.mu.RUnlock()
	assert.True(t, ok, "Base schema should be compiled and stored.")
}

// Test SchemaValidator Initialization Success (File).
func TestSchemaValidator_Initialize_Success_File(t *testing.T) {
	logger := logging.GetNoopLogger()
	schemaPath := createTempSchemaFile(t, minValidSchema)
	source := SchemaSource{FilePath: schemaPath}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialize should succeed with valid schema file.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
}

// Test SchemaValidator Initialization Failure (Invalid JSON in File).
func TestSchemaValidator_Initialize_Failure_InvalidFileContent(t *testing.T) {
	logger := logging.GetNoopLogger()
	schemaPath := createTempSchemaFile(t, invalidSchemaSyntax)
	source := SchemaSource{FilePath: schemaPath}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.Error(t, err, "Initialize should fail with invalid schema file content.")
	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized on failure.")

	// Check if the error is a validation error.
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be of type *ValidationError.")
	assert.Contains(t, err.Error(), "compiler.AddResource failed", "Error message should indicate AddResource failure.") // This might vary depending on the library's error.
}

// Test SchemaValidator Initialization Failure (File Not Found).
func TestSchemaValidator_Initialize_Failure_FileNotFound(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: "/non/existent/path/schema.json"} // No URL fallback.
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.Error(t, err, "Initialize should fail if file not found and no URL.")
	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized.")
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be of type *ValidationError.")
	assert.Equal(t, ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found.")
}

// Test SchemaValidator Initialization Failure (Invalid URL - depends on network).
// func TestSchemaValidator_Initialize_Failure_InvalidURL(t *testing.T) {
// 	logger := logging.GetNoopLogger()
// 	// Assuming no file path and invalid URL.
// 	source := SchemaSource{URL: "http://invalid-url-that-does-not-exist-zzzzzz/schema.json"}
// 	validator := NewSchemaValidator(source, logger)
// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Timeout for network request.
// 	defer cancel()

// 	err := validator.Initialize(ctx)
// 	require.Error(t, err, "Initialize should fail with invalid URL.")
// 	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized.")
// 	var validationErr *ValidationError
// 	assert.True(t, errors.As(err, &validationErr), "Error should be of type *ValidationError.")
// 	assert.Equal(t, ErrSchemaLoadFailed, validationErr.Code, "Error code should indicate load failure.")
// }

// Test SchemaValidator Validate Success.
func TestSchemaValidator_Validate_Success(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "base", []byte(validMessage))
	assert.NoError(t, err, "Validation should succeed for a valid message.")
}

// Test SchemaValidator Validate Failure (Invalid Message - Missing Required).
func TestSchemaValidator_Validate_Failure_InvalidMessage_Missing(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "base", []byte(invalidMessageMissingMethod))
	require.Error(t, err, "Validation should fail for invalid message (missing required).")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")
	assert.Contains(t, validationErr.Message, `"method" is required`, "Error message should indicate missing field.")
}

// Test SchemaValidator Validate Failure (Invalid Message - Wrong Type).
func TestSchemaValidator_Validate_Failure_InvalidMessage_Type(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "base", []byte(invalidMessageWrongType))
	require.Error(t, err, "Validation should fail for invalid message (wrong type).")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")
	assert.Contains(t, validationErr.Message, `expected string, but got number`, "Error message should indicate type mismatch.")
}

// Test SchemaValidator Validate Failure (Invalid JSON Syntax).
func TestSchemaValidator_Validate_Failure_InvalidJSON(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "base", []byte(invalidJsonSyntaxMessage))
	require.Error(t, err, "Validation should fail for invalid JSON syntax.")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrInvalidJSONFormat, validationErr.Code, "Error code should be ErrInvalidJSONFormat.")
	assert.Contains(t, validationErr.Message, `Invalid JSON format`, "Error message should indicate invalid JSON.")
}

// Test SchemaValidator Validate Before Initialization.
func TestSchemaValidator_Validate_NotInitialized(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger) // Not initialized.
	ctx := context.Background()

	err := validator.Validate(ctx, "base", []byte(validMessage))
	require.Error(t, err, "Validation should fail if validator is not initialized.")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found/uninitialized.")
	assert.Contains(t, validationErr.Message, "Schema validator not initialized", "Error message should indicate not initialized.")
}

// Test Shutdown method.
func TestSchemaValidator_Shutdown(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{Embedded: []byte(minValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")
	assert.True(t, validator.IsInitialized(), "Should be initialized.")

	err = validator.Shutdown()
	assert.NoError(t, err, "Shutdown should not return an error.")
	assert.False(t, validator.IsInitialized(), "Validator should be marked as not initialized after shutdown.")

	// Verify schemas map is cleared (or check internal state if possible).
	validator.mu.RLock()
	assert.Nil(t, validator.schemas, "Schemas map should be cleared after shutdown.")
	validator.mu.RUnlock()

	// Calling Shutdown again should be safe and do nothing.
	err = validator.Shutdown()
	assert.NoError(t, err, "Calling Shutdown again should not return an error.")
}
