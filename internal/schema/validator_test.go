// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	// Import the config package to use config.SchemaConfig.
	"github.com/dkoosis/cowgnition/internal/config"
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
const invalidJSONSyntaxMessage = `{"jsonrpc": "2.0", "method": "ping"` // Missing closing brace.

// --- Helper Function to Get Schema Path ---.
// Attempts to find the schema relative to the test file. Adjust if needed.
func getFullSchemaPath(t *testing.T) string {
	t.Helper()
	// Assumes schema.json is in the same directory as validator.go/validator_test.go.
	// If your structure is different, you might need ../schema.json or similar.
	path := "schema.json"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Try one level up if not found locally (common if tests run from package dir).
		path = "../schema/schema.json"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatalf("Could not find schema.json. Make sure it's in the 'internal/schema' directory. CWD: %s.", getwd(t))
		}
	}
	absPath, err := filepath.Abs(path)
	require.NoError(t, err, "Failed to get absolute path for schema.json.")
	return absPath
}

func getwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Logf("Warning: Could not get working directory: %v.", err)
		return "[unknown]"
	}
	return wd
}

// --- Test Cases ---.

// Test NewValidator creation.
func TestNewValidator(t *testing.T) { // Corrected test name for clarity.
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	cfg := config.SchemaConfig{
		SchemaOverrideURI: "file://" + schemaPath,
	}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	assert.NotNil(t, validator, "Validator should not be nil.")
	assert.NotNil(t, validator.compiler, "Compiler should not be nil.")
	assert.NotNil(t, validator.schemas, "Schemas map should not be nil.")
	assert.NotNil(t, validator.httpClient, "HTTP client should not be nil.")
	assert.NotNil(t, validator.logger, "Logger should not be nil.")
}

// Test Validator Initialization Success (File).
func TestValidator_Initialize_Success_File(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + schemaPath}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialize should succeed with valid schema file.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetLoadDuration(), "Load duration should be recorded.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	validator.mu.RLock()
	_, okBase := validator.schemas["base"]
	_, okDef := validator.schemas["JSONRPCRequest"] // Check for a known definition.
	validator.mu.RUnlock()
	assert.True(t, okBase, "Base schema should be compiled and stored.")
	assert.True(t, okDef, "Known definition 'JSONRPCRequest' should be compiled.")
}

// Test Validator Initialization Success (Embedded Fallback).
func TestValidator_Initialize_Success_Embedded(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: ""}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	require.NotEmpty(t, embeddedSchemaContent, "Internal embedded schema content should not be empty.")

	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialize should succeed using the embedded schema fallback.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	validator.mu.RLock()
	_, hasBase := validator.schemas["base"]
	_, hasRequest := validator.schemas["JSONRPCRequest"]
	validator.mu.RUnlock()

	assert.True(t, hasBase, "Base schema should be compiled and stored from embedded content.")
	assert.True(t, hasRequest, "JSONRPCRequest definition should be compiled from embedded content.")
}

// Test Validator Initialization Failure (Invalid JSON in File).
func TestValidator_Initialize_Failure_InvalidFileContent(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	schemaPath := createTempSchemaFile(t, invalidSchemaSyntax)
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + schemaPath}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.Error(t, err, "Initialize should fail with invalid schema file content.")
	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized on failure.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be of type *ValidationError.")
	assert.Equal(t, ErrSchemaLoadFailed, validationErr.Code, "Error code should indicate schema load failure.")
	assert.Contains(t, err.Error(), "JSON", "Error message should indicate JSON syntax error.")
	assert.Contains(t, err.Error(), "unmarshal", "Error message should indicate JSON syntax error.")
}

// Test Validator Initialization Failure (File Not Found).
func TestValidator_Initialize_Failure_FileNotFound(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file:///non/existent/path/schema.json"}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)
	require.Error(t, err, "Initialize should fail if file not found and no URL.")
	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found.")
}

// Test Validator Validate Success.
func TestValidator_Validate_Success(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(validMessage))
	assert.NoError(t, err, "Validation should succeed for a valid message against JSONRPCRequest.")
}

// Test Validator Validate Failure (Invalid Message - Missing Required).
func TestValidator_Validate_Failure_InvalidMessage_Missing(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidMessageMissingMethod))
	require.Error(t, err, "Validation should fail for invalid message (missing required).")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")
	assert.Contains(t, validationErr.Error(), "method", "Error details should mention the 'method' property.")
	assert.Contains(t, validationErr.Message, "required", "Error message should indicate a required field is missing.")
}

// Test Validator Validate Failure (Invalid Message - Wrong Type).
func TestValidator_Validate_Failure_InvalidMessage_Type(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidMessageWrongType))
	require.Error(t, err, "Validation should fail for invalid message (wrong type).")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")
	assert.Contains(t, validationErr.Error(), "method", "Error details should mention the 'method' property.")
	assert.Contains(t, validationErr.Error(), "string", "Error details should mention expected type 'string'.")
	assert.Contains(t, validationErr.Error(), "number", "Error details should mention actual type 'number'.")
	assert.Equal(t, "/method", validationErr.InstancePath, "Error instance path should point to '/method'.")
}

// Test Validator Validate Failure (Invalid JSON Syntax).
func TestValidator_Validate_Failure_InvalidJSON(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidJSONSyntaxMessage))
	require.Error(t, err, "Validation should fail for invalid JSON syntax.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrInvalidJSONFormat, validationErr.Code, "Error code should be ErrInvalidJSONFormat.")
	assert.Contains(t, validationErr.Message, `Invalid JSON format`, "Error message should indicate invalid JSON.")
}

// Test Validator Validate Before Initialization.
func TestValidator_Validate_NotInitialized(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger) // Not initialized.
	ctx := context.Background()

	err := validator.Validate(ctx, "JSONRPCRequest", []byte(validMessage))
	require.Error(t, err, "Validation should fail if validator is not initialized.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a ValidationError.")
	assert.Equal(t, ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found/uninitialized.")
	assert.Contains(t, validationErr.Message, "validator not initialized", "Error message should indicate not initialized.")
}

// Test Validator Shutdown method.
func TestValidator_Shutdown(t *testing.T) { // Corrected test name.
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	// Corrected: Use NewValidator.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed.")
	assert.True(t, validator.IsInitialized(), "Should be initialized.")

	err = validator.Shutdown()
	assert.NoError(t, err, "Shutdown should not return an error.")
	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized after shutdown.")

	validator.mu.RLock()
	assert.Nil(t, validator.schemas, "Schemas map should be cleared after shutdown.")
	validator.mu.RUnlock()

	err = validator.Shutdown()
	assert.NoError(t, err, "Calling Shutdown again should not return an error.")
}
