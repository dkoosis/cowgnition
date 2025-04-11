// File: internal/schema/validator_test.go.
package schema

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	// Added missing time import used by mock validator GetLoad/CompileDuration.
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

// Invalid JSON Schema (syntax error).
const invalidSchemaSyntax = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "InvalidSchema",
  "type": "object",
  "properties": {
    "jsonrpc": { "const": "2.0" },
` // Missing closing brace.

// Valid JSON message potentially conforming to parts of the full schema (e.g., base request).
const validMessage = `{"jsonrpc": "2.0", "method": "ping", "id": 1}`

// Invalid JSON message (missing required 'method').
const invalidMessageMissingMethod = `{"jsonrpc": "2.0", "id": 1}`

// Invalid JSON message (wrong type for 'method').
const invalidMessageWrongType = `{"jsonrpc": "2.0", "method": 123, "id": 1}`

// Invalid JSON syntax message.
const invalidJsonSyntaxMessage = `{"jsonrpc": "2.0", "method": "ping"` // Missing closing brace.

// --- Helper Function to Get Schema Path ---
// Attempts to find the schema relative to the test file. Adjust if needed.
func getFullSchemaPath(t *testing.T) string {
	t.Helper()
	// Assumes schema.json is in the same directory as validator.go/validator_test.go
	// If your structure is different, you might need ../schema.json or similar.
	path := "schema.json"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Try one level up if not found locally (common if tests run from package dir)
		path = "../schema/schema.json"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatalf("Could not find schema.json. Make sure it's in the 'internal/schema' directory. CWD: %s", getwd(t))
		}
	}
	absPath, err := filepath.Abs(path)
	require.NoError(t, err, "Failed to get absolute path for schema.json")
	// t.Logf("Using schema path: %s", absPath) // Optional: uncomment for debugging path issues
	return absPath
}

func getwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Logf("Warning: Could not get working directory: %v", err)
		return "[unknown]"
	}
	return wd
}

// --- Test Cases ---

// Test NewSchemaValidator creation.
func TestNewSchemaValidator(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger)
	assert.NotNil(t, validator, "Validator should not be nil.")
	assert.NotNil(t, validator.compiler, "Compiler should not be nil.")
	assert.NotNil(t, validator.schemas, "Schemas map should not be nil.")
	assert.NotNil(t, validator.httpClient, "HTTP client should not be nil.")
	assert.NotNil(t, validator.logger, "Logger should not be nil.")
}

// Test SchemaValidator Initialization Success (File).
func TestSchemaValidator_Initialize_Success_File(t *testing.T) {
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	source := SchemaSource{FilePath: schemaPath}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialize should succeed with valid schema file.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetLoadDuration(), "Load duration should be recorded.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	validator.mu.RLock()
	_, okBase := validator.schemas["base"]
	_, okDef := validator.schemas["JSONRPCRequest"] // Check for a known definition
	validator.mu.RUnlock()
	assert.True(t, okBase, "Base schema should be compiled and stored.")
	assert.True(t, okDef, "Known definition 'JSONRPCRequest' should be compiled.")
}

// Test SchemaValidator Initialization Success (Embedded - Keep for basic embedded test).
func TestSchemaValidator_Initialize_Success_Embedded(t *testing.T) {
	logger := logging.GetNoopLogger()
	// Use a small, valid schema just for the embedded test case
	localMinValidSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"definitions": { "ping": {"type": "object"} }
	}`
	source := SchemaSource{Embedded: []byte(localMinValidSchema)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialize should succeed with valid embedded schema.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetLoadDuration(), "Load duration should be recorded.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")
	validator.mu.RLock()
	_, okBase := validator.schemas["base"]
	_, okPing := validator.schemas["ping"]
	validator.mu.RUnlock()
	assert.True(t, okBase, "Base schema should be compiled and stored.")
	assert.True(t, okPing, "Specific definition 'ping' should be compiled.")
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

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be of type *ValidationError.")
	assert.Equal(t, ErrSchemaLoadFailed, validationErr.Code, "Error code should indicate schema load failure.")
	assert.Contains(t, validationErr.Error(), "invalid character", "Error message should indicate JSON syntax error")
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

// Test SchemaValidator Validate Success.
func TestSchemaValidator_Validate_Success(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	// Validate against a specific definition within the loaded schema
	err = validator.Validate(ctx, "JSONRPCRequest", []byte(validMessage))
	assert.NoError(t, err, "Validation should succeed for a valid message against JSONRPCRequest.")
}

// Test SchemaValidator Validate Failure (Invalid Message - Missing Required).
func TestSchemaValidator_Validate_Failure_InvalidMessage_Missing(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	// Validate against a specific definition expected to fail
	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidMessageMissingMethod))
	require.Error(t, err, "Validation should fail for invalid message (missing required).")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")
	// Adjust expected error substring based on actual jsonschema output for missing 'method'
	assert.Contains(t, validationErr.Error(), `missing properties: "method"`, "Error message should indicate missing 'method' property.")
}

// Test SchemaValidator Validate Failure (Invalid Message - Wrong Type).
func TestSchemaValidator_Validate_Failure_InvalidMessage_Type(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	// Validate against a specific definition expected to fail
	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidMessageWrongType))
	require.Error(t, err, "Validation should fail for invalid message (wrong type).")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")
	// Adjust expected error substring based on actual jsonschema output for wrong 'method' type
	assert.Contains(t, validationErr.Error(), `expected string, but got number`, "Error message should indicate 'method' type mismatch.")
	assert.Contains(t, validationErr.InstancePath, "/method", "Error instance path should point to '/method'")
}

// Test SchemaValidator Validate Failure (Invalid JSON Syntax).
func TestSchemaValidator_Validate_Failure_InvalidJSON(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidJsonSyntaxMessage))
	require.Error(t, err, "Validation should fail for invalid JSON syntax.")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrInvalidJSONFormat, validationErr.Code, "Error code should be ErrInvalidJSONFormat.")
	assert.Contains(t, validationErr.Message, `Invalid JSON format`, "Error message should indicate invalid JSON.")
}

// Test SchemaValidator Validate Before Initialization.
func TestSchemaValidator_Validate_NotInitialized(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger) // Not initialized.
	ctx := context.Background()

	err := validator.Validate(ctx, "JSONRPCRequest", []byte(validMessage))
	require.Error(t, err, "Validation should fail if validator is not initialized.")

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found/uninitialized.")
	assert.Contains(t, validationErr.Message, "Schema validator not initialized", "Error message should indicate not initialized.")
}

// Test Shutdown method.
func TestSchemaValidator_Shutdown(t *testing.T) {
	logger := logging.GetNoopLogger()
	source := SchemaSource{FilePath: getFullSchemaPath(t)}
	validator := NewSchemaValidator(source, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")
	assert.True(t, validator.IsInitialized(), "Should be initialized.")

	err = validator.Shutdown()
	assert.NoError(t, err, "Shutdown should not return an error.")
	assert.False(t, validator.IsInitialized(), "Validator should be marked as not initialized after shutdown.")

	validator.mu.RLock()
	assert.Nil(t, validator.schemas, "Schemas map should be cleared after shutdown.")
	validator.mu.RUnlock()

	err = validator.Shutdown()
	assert.NoError(t, err, "Calling Shutdown again should not return an error.")
}
