// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

// file: internal/schema/validator_test.go.

import (
	"context"
	"encoding/json" // Needed for json.SyntaxError check.
	"os"
	"path/filepath"
	"strings" // Import strings.
	"testing"

	// Needed by setupTestMiddleware indirectly.
	"github.com/cockroachdb/errors"
	// Import the config package to use config.SchemaConfig.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging" // Needed for jsonschema.ValidationError check.
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

// TestValidator_ReturnsInstance_When_CreatedWithValidConfig tests NewValidator creation.
func TestValidator_ReturnsInstance_When_CreatedWithValidConfig(t *testing.T) {
	t.Log("Testing Validator Creation: Should return a valid instance.")
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	cfg := config.SchemaConfig{
		SchemaOverrideURI: "file://" + schemaPath,
	}
	validator := NewValidator(cfg, logger)

	assert.NotNil(t, validator, "Validator instance should not be nil.")
	assert.NotNil(t, validator.compiler, "Compiler should be initialized.")
	assert.NotNil(t, validator.schemas, "Schemas map should be initialized (empty).")
	assert.NotNil(t, validator.httpClient, "HTTP client should be initialized.")
	assert.NotNil(t, validator.logger, "Logger should be initialized.")
}

// TestValidator_SucceedsInitialization_When_UsingValidSchemaFile tests initialization success from a file.
func TestValidator_SucceedsInitialization_When_UsingValidSchemaFile(t *testing.T) {
	t.Log("Testing Initialize Success: Using a valid schema file.")
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + schemaPath}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)

	require.NoError(t, err, "Initialize should succeed with a valid schema file.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetLoadDuration(), "Load duration should be recorded.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	validator.mu.RLock()
	schemaCount := len(validator.schemas)
	hasBase := validator.HasSchema("base")
	hasRequest := validator.HasSchema("JSONRPCRequest") // Check for a known definition.
	validator.mu.RUnlock()

	assert.True(t, hasBase, "Base schema should be compiled and stored.")
	assert.True(t, hasRequest, "Known definition 'JSONRPCRequest' should be compiled.")
	t.Logf("Successfully initialized from file: %s (%d schemas compiled).", schemaPath, schemaCount)
}

// TestValidator_SucceedsInitialization_When_UsingEmbeddedSchema tests initialization success using embedded content.
func TestValidator_SucceedsInitialization_When_UsingEmbeddedSchema(t *testing.T) {
	t.Log("Testing Initialize Success: Using embedded schema content as fallback.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: ""} // No override, should use embedded.
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	require.NotEmpty(t, embeddedSchemaContent, "Internal embedded schema content should not be empty for this test to be valid.")

	err := validator.Initialize(ctx)

	require.NoError(t, err, "Initialize should succeed using the embedded schema.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	validator.mu.RLock()
	schemaCount := len(validator.schemas)
	hasBase := validator.HasSchema("base")
	hasRequest := validator.HasSchema("JSONRPCRequest")
	validator.mu.RUnlock()

	assert.True(t, hasBase, "Base schema should be compiled and stored from embedded content.")
	assert.True(t, hasRequest, "JSONRPCRequest definition should be compiled from embedded content.")
	t.Logf("Successfully initialized with embedded schema (%d schemas compiled).", schemaCount)
}

// TestValidator_FailsInitialization_When_SchemaFileIsInvalidJSON tests initialization failure with invalid JSON content.
func TestValidator_FailsInitialization_When_SchemaFileIsInvalidJSON(t *testing.T) {
	t.Log("Testing Initialize Failure: Schema file contains invalid JSON syntax.")
	logger := logging.GetNoopLogger()
	schemaPath := createTempSchemaFile(t, invalidSchemaSyntax)
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + schemaPath}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)

	require.Error(t, err, "Initialize should fail when schema file content is invalid JSON.")
	assert.False(t, validator.IsInitialized(), "Validator should not be marked as initialized.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be of type *ValidationError.")
	assert.Equal(t, ErrSchemaLoadFailed, validationErr.Code, "Error code should be ErrSchemaLoadFailed.") // Expect LoadFailed because parsing happens during load.

	var syntaxError *json.SyntaxError
	isSyntaxError := errors.As(validationErr.Cause, &syntaxError) // Check the *cause* for the specific syntax error.
	assert.True(t, isSyntaxError, "Underlying cause should be a json.SyntaxError.")
	if isSyntaxError {
		t.Logf("Correctly identified underlying json.SyntaxError: %v.", syntaxError)
	}
}

// TestValidator_FailsInitialization_When_SchemaFileNotFound tests initialization failure when the override file doesn't exist.
func TestValidator_FailsInitialization_When_SchemaFileNotFound(t *testing.T) {
	t.Log("Testing Initialize Failure: Schema override file not found.")
	logger := logging.GetNoopLogger()
	nonExistentPath := "/non/existent/path/schema.json"
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + nonExistentPath}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()

	err := validator.Initialize(ctx)

	// NOTE: With the current loader logic, if the override fails, it falls back
	// to embedded. So, Initialize *should succeed* if the embedded schema is present.
	// We test the *loader's* error return separately if needed, or adjust this test
	// if the fallback behavior changes.
	// Let's assume embedded schema exists for this test.
	require.NoError(t, err, "Initialize should SUCCEED by falling back to embedded schema when override file is not found.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized via fallback.")
	t.Logf("Initialize correctly fell back to embedded schema when override file '%s' was not found.", nonExistentPath)

	// If you wanted to test the error returned *specifically* by loadSchemaFromURI when the file is not found,
	// you'd need a test that calls that function directly or mocks the embedded fallback.
}

// TestValidator_SucceedsValidation_When_MessageIsValid tests successful validation of a conforming message.
func TestValidator_SucceedsValidation_When_MessageIsValid(t *testing.T) {
	t.Log("Testing Validate Success: Message conforms to JSONRPCRequest schema.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed, cannot proceed with validation test.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(validMessage))

	assert.NoError(t, err, "Validation should succeed for a valid message against JSONRPCRequest.")
}

// TestValidator_FailsValidation_When_RequiredFieldMissing tests validation failure due to missing required field.
func TestValidator_FailsValidation_When_RequiredFieldMissing(t *testing.T) {
	t.Log("Testing Validate Failure: Message is missing required 'method' field for JSONRPCRequest.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed, cannot proceed with validation test.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidMessageMissingMethod))

	require.Error(t, err, "Validation should fail when a required field ('method') is missing.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a *ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")

	// Check context for detailed causes added by convertValidationError.
	causesRaw, ok := validationErr.Context["validationCausesDetail"]
	require.True(t, ok, "Validation causes detail should be present in context.")
	causes, ok := causesRaw.([]map[string]string)
	require.True(t, ok, "Validation causes detail should be []map[string]string.")
	require.NotEmpty(t, causes, "There should be at least one validation cause.")

	foundMissingMethod := false
	for _, causeMap := range causes {
		if msg, exists := causeMap["message"]; exists && strings.Contains(msg, "missing properties: 'method'") {
			foundMissingMethod = true
			t.Logf("Found expected validation cause: %v.", causeMap)
			break
		}
	}
	assert.True(t, foundMissingMethod, "Expected validation cause message indicating missing 'method' property.")
}

// TestValidator_FailsValidation_When_FieldHasWrongType tests validation failure due to incorrect field type.
func TestValidator_FailsValidation_When_FieldHasWrongType(t *testing.T) {
	t.Log("Testing Validate Failure: Message has incorrect type for 'method' field.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed, cannot proceed with validation test.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidMessageWrongType))

	require.Error(t, err, "Validation should fail when 'method' field has wrong type.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a *ValidationError.")
	assert.Equal(t, ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.")

	// Check context for detailed causes.
	causesRaw, ok := validationErr.Context["validationCausesDetail"]
	require.True(t, ok, "Validation causes detail should be present in context.")
	causes, ok := causesRaw.([]map[string]string)
	require.True(t, ok, "Validation causes detail should be []map[string]string.")
	require.NotEmpty(t, causes, "There should be at least one validation cause.")

	foundTypeMismatch := false
	expectedLocation := "/method" // Expected path for the type error.
	for _, causeMap := range causes {
		// Check both the message and the location within the *specific cause*.
		if msg, exists := causeMap["message"]; exists && strings.Contains(msg, "expected string, but got number") {
			if loc, locExists := causeMap["instanceLocation"]; locExists && loc == expectedLocation {
				foundTypeMismatch = true
				t.Logf("Found expected validation cause for type mismatch: %v.", causeMap)
				break
			}
		}
	}
	assert.True(t, foundTypeMismatch, "Expected validation cause message for type mismatch at instance path '%s'.", expectedLocation)
}

// TestValidator_FailsValidation_When_MessageIsInvalidJSON tests validation failure with invalid JSON syntax.
func TestValidator_FailsValidation_When_MessageIsInvalidJSON(t *testing.T) {
	t.Log("Testing Validate Failure: Input message has invalid JSON syntax.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed, cannot proceed with validation test.")

	err = validator.Validate(ctx, "JSONRPCRequest", []byte(invalidJSONSyntaxMessage))

	require.Error(t, err, "Validation should fail when the input message is invalid JSON.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a *ValidationError.")
	assert.Equal(t, ErrInvalidJSONFormat, validationErr.Code, "Error code should be ErrInvalidJSONFormat.")
	assert.Contains(t, validationErr.Message, `Invalid JSON format`, "Error message should indicate invalid JSON.") // Check wrapper message.

	var syntaxError *json.SyntaxError
	isSyntaxError := errors.As(validationErr.Cause, &syntaxError) // Check the *cause* for the specific syntax error.
	assert.True(t, isSyntaxError, "Underlying cause should be a json.SyntaxError.")
	if isSyntaxError {
		t.Logf("Correctly identified underlying json.SyntaxError: %v.", syntaxError)
	}
}

// TestValidator_FailsValidation_When_ValidatorNotInitialized tests calling Validate before Initialize.
func TestValidator_FailsValidation_When_ValidatorNotInitialized(t *testing.T) {
	t.Log("Testing Validate Failure: Calling Validate before Initialize.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: ""} // Config doesn't matter here.
	validator := NewValidator(cfg, logger)            // Not initialized.
	ctx := context.Background()

	err := validator.Validate(ctx, "JSONRPCRequest", []byte(validMessage))

	require.Error(t, err, "Validation should fail if validator is not initialized.")

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr), "Error should be a *ValidationError.")
	assert.Equal(t, ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found/uninitialized.")
	assert.Contains(t, validationErr.Message, "validator not initialized", "Error message should indicate not initialized.")
}

// TestValidator_Shutdown_When_Called_Then_MarksAsUninitialized tests the Shutdown method.
func TestValidator_Shutdown_When_Called_Then_MarksAsUninitialized(t *testing.T) {
	t.Log("Testing Shutdown: Ensures validator is marked uninitialized and resources cleared.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + getFullSchemaPath(t)}
	validator := NewValidator(cfg, logger)
	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Initialization failed, cannot proceed with shutdown test.")
	require.True(t, validator.IsInitialized(), "Validator should be initialized before shutdown.")

	// Call Shutdown the first time.
	err = validator.Shutdown()
	assert.NoError(t, err, "First call to Shutdown should not return an error.")
	assert.False(t, validator.IsInitialized(), "Validator should be marked as uninitialized after shutdown.")

	// Check internal state (use lock for safety).
	validator.mu.RLock()
	assert.Nil(t, validator.schemas, "Schemas map should be nil after shutdown.")
	assert.Nil(t, validator.schemaDoc, "SchemaDoc map should be nil after shutdown.")
	validator.mu.RUnlock()

	// Call Shutdown again (should be idempotent).
	err = validator.Shutdown()
	assert.NoError(t, err, "Calling Shutdown again should not return an error.")
	assert.False(t, validator.IsInitialized(), "Validator should remain uninitialized after second shutdown call.")
}
