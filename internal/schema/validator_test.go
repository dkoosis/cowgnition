// file: internal/schema/validator_test.go
package schema_test // Changed package name.

import (
	"context"
	"encoding/json" // Needed for json.SyntaxError check.
	"os"
	"path/filepath"
	"strings" // Import strings.
	"testing"

	"github.com/cockroachdb/errors"
	// Import the config package to use config.SchemaConfig.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import mcptypes package.

	// Import the package being tested.
	sch "github.com/dkoosis/cowgnition/internal/schema"

	// Use mcptypes for ValidatorInterface reference.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Compile-time interface check ---
// This ensures the concrete *Validator type implements the mcptypes.ValidatorInterface.
// Placed at the package level for clarity.
// NOTE: We now reference the type from the imported package 'sch'.
var _ mcptypes.ValidatorInterface = (*sch.Validator)(nil) // Use mcptypes interface.

// --- End interface check ---

// Helper function to create a temporary schema file for testing.
// NOTE: Uses sch.invalidSchemaSyntax defined below, assuming it's exported or moved.
// For simplicity, we'll redefine consts locally in the test file.
const invalidSchemaSyntax = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "InvalidSchema",
  "type": "object",
  "properties": {
    "jsonrpc": { "const": "2.0" },
` // Missing closing brace.

func createTempSchemaFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test_schema.json")
	err := os.WriteFile(path, []byte(content), 0600)
	require.NoError(t, err, "Failed to create temporary schema file.")
	return path
}

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
	// Assumes schema.json is in the 'internal/schema' directory relative to project root.
	path := "../schema/schema.json" // Path relative to the test file's new location conceptually.

	// ---> ADD LOGGING <---
	cwd, _ := os.Getwd()
	t.Logf("Attempting to find schema. CWD: %s, Relative Path: %s", cwd, path)
	// ---> END LOGGING <---

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Could not find schema.json at expected path '%s'. CWD: %s.", path, getwd(t))
	}

	absPath, err := filepath.Abs(path)
	require.NoError(t, err, "Failed to get absolute path for schema.json.")

	// ---> ADD LOGGING <---
	t.Logf("Resolved absolute path: %s", absPath)
	// ---> END LOGGING <---
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

// --- Test Setup Helper ---
// setupTestValidator creates and initializes a validator using the embedded schema for testing.
// Returns mcptypes.ValidatorInterface to match usage in tests.
// NOTE: Now uses sch.NewValidator.
func setupTestValidator(t *testing.T) mcptypes.ValidatorInterface { // Return interface type.
	t.Helper()
	logger := logging.GetNoopLogger()
	// Use embedded schema for consistency in tests unless specific file content is needed.
	cfg := config.SchemaConfig{SchemaOverrideURI: ""}
	validator := sch.NewValidator(cfg, logger) // Use qualified call.
	require.NotNil(t, validator)

	ctx := context.Background()
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Test setup failed: Could not initialize validator with embedded schema.")
	require.True(t, validator.IsInitialized(), "Test setup failed: Validator not initialized.")
	return validator // Return the concrete type which satisfies the interface.
}

// --- END NEW HELPER ---

// --- Test Cases ---.

// TestValidator_NewValidator_ReturnsInstance_When_ConfigIsValid tests NewValidator creation.
// Renamed function to follow ADR-008 convention.
func TestValidator_NewValidator_ReturnsInstance_When_ConfigIsValid(t *testing.T) {
	t.Log("Testing Validator Creation: Should return a valid instance.")
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	cfg := config.SchemaConfig{
		SchemaOverrideURI: "file://" + schemaPath,
	}
	validator := sch.NewValidator(cfg, logger) // Use qualified call.

	assert.NotNil(t, validator, "Validator instance should not be nil.")
	// Cannot directly assert internal fields anymore as they are unexported.
	// Rely on Initialize tests.
}

// TestValidator_Initialize_Succeeds_When_UsingValidSchemaFile tests initialization success from a file.
// Renamed function to follow ADR-008 convention.
func TestValidator_Initialize_Succeeds_When_UsingValidSchemaFile(t *testing.T) {
	t.Log("Testing Initialize Success: Using a valid schema file.")
	logger := logging.GetNoopLogger()
	schemaPath := getFullSchemaPath(t)
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + schemaPath}
	validator := sch.NewValidator(cfg, logger) // Use qualified call.
	ctx := context.Background()

	err := validator.Initialize(ctx)

	require.NoError(t, err, "Initialize should succeed with a valid schema file.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetLoadDuration(), "Load duration should be recorded.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	// Check internal state via interface methods where possible.
	hasBase := validator.HasSchema("base")
	hasRequest := validator.HasSchema("JSONRPCRequest") // Check for a known definition.

	assert.True(t, hasBase, "Base schema should be compiled and stored.")
	assert.True(t, hasRequest, "Known definition 'JSONRPCRequest' should be compiled.")

	// Cannot check internal count easily anymore.
	t.Logf("Successfully initialized from file: %s.", schemaPath)
}

// TestValidator_Initialize_Succeeds_When_UsingEmbeddedSchema tests initialization success using embedded content.
// Renamed function to follow ADR-008 convention.
func TestValidator_Initialize_Succeeds_When_UsingEmbeddedSchema(t *testing.T) {
	t.Log("Testing Initialize Success: Using embedded schema content as fallback.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: ""} // No override, should use embedded.
	validator := sch.NewValidator(cfg, logger)        // Use qualified call.
	ctx := context.Background()

	// Need access to the embedded content, potentially expose via a helper in schema package?
	// Or skip this check if embeddedSchemaContent is not exported. Let's assume we can't check it here easily.
	// require.NotEmpty(t, sch.embeddedSchemaContent, "Internal embedded schema content should not be empty for this test to be valid.")

	err := validator.Initialize(ctx)

	require.NoError(t, err, "Initialize should succeed using the embedded schema.")
	assert.True(t, validator.IsInitialized(), "Validator should be marked as initialized.")
	assert.NotZero(t, validator.GetCompileDuration(), "Compile duration should be recorded.")

	hasBase := validator.HasSchema("base")
	hasRequest := validator.HasSchema("JSONRPCRequest")

	assert.True(t, hasBase, "Base schema should be compiled and stored from embedded content.")
	assert.True(t, hasRequest, "JSONRPCRequest definition should be compiled from embedded content.")

	t.Logf("Successfully initialized with embedded schema.")
}

// TestValidator_Initialize_Fails_When_SchemaFileIsInvalidJSON tests initialization failure with invalid JSON content.
// Renamed function to follow ADR-008 convention.
func TestValidator_Initialize_Fails_When_SchemaFileIsInvalidJSON(t *testing.T) {
	t.Log("Testing Initialize Failure: Schema file contains invalid JSON syntax.")
	logger := logging.GetNoopLogger()
	schemaPath := createTempSchemaFile(t, invalidSchemaSyntax)
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + schemaPath}
	v := sch.NewValidator(cfg, logger) // Use qualified call.
	ctx := context.Background()

	err := v.Initialize(ctx)
	require.Error(t, err, "Initialize should fail when schema file content is invalid JSON.")

	// Call method on the concrete type returned by NewValidator.
	assert.False(t, v.IsInitialized(), "Validator should not be marked as initialized.")

	var validationErr *sch.ValidationError // Use qualified type.
	require.True(t, errors.As(err, &validationErr), "Error should be of type *schema.ValidationError.")
	assert.Equal(t, sch.ErrSchemaLoadFailed, validationErr.Code, "Error code should be ErrSchemaLoadFailed.") // Use qualified constant.

	var syntaxError *json.SyntaxError
	isSyntaxError := errors.As(validationErr.Cause, &syntaxError) // Check the *cause* for the specific syntax error.
	assert.True(t, isSyntaxError, "Underlying cause should be a json.SyntaxError.")
	if isSyntaxError {
		t.Logf("Correctly identified underlying json.SyntaxError: %v.", syntaxError)
	}
}

// TestValidator_Initialize_SucceedsWithFallback_When_SchemaFileNotFound tests initialization success via fallback when the override file doesn't exist.
// Renamed function to follow ADR-008 convention and reflect fallback behavior.
func TestValidator_Initialize_SucceedsWithFallback_When_SchemaFileNotFound(t *testing.T) {
	t.Log("Testing Initialize Fallback: Schema override file not found.")
	logger := logging.GetNoopLogger()
	nonExistentPath := "/non/existent/path/schema.json"
	cfg := config.SchemaConfig{SchemaOverrideURI: "file://" + nonExistentPath}
	v := sch.NewValidator(cfg, logger) // Use qualified call.
	ctx := context.Background()

	// Assume embedded schema exists and is valid (cannot check sch.embeddedSchemaContent directly).

	err := v.Initialize(ctx)

	// NOTE: With the current loader logic, if the override fails, it falls back.
	// to embedded. So, Initialize *should succeed* if the embedded schema is present.
	require.NoError(t, err, "Initialize should SUCCEED by falling back to embedded schema when override file is not found.")

	// Call method on the concrete type.
	assert.True(t, v.IsInitialized(), "Validator should be marked as initialized via fallback.")
	t.Logf("Initialize correctly fell back to embedded schema when override file '%s' was not found.", nonExistentPath)
}

// TestValidator_Validate_Succeeds_When_MessageIsValid tests successful validation of a conforming message.
// Renamed function to follow ADR-008 convention.
func TestValidator_Validate_Succeeds_When_MessageIsValid(t *testing.T) {
	t.Log("Testing Validate Success: Message conforms to JSONRPCRequest schema.")
	// Use setup helper for initialized validator with embedded schema.
	validator := setupTestValidator(t) // Returns interface.
	ctx := context.Background()

	// Use generic request schema name from mapping.
	err := validator.Validate(ctx, "request", []byte(validMessage))

	assert.NoError(t, err, "Validation should succeed for a valid message against 'request' (mapped to JSONRPCRequest).")
}

// TestValidator_Validate_Fails_When_RequiredFieldMissing tests validation failure due to missing required field.
// Renamed function to follow ADR-008 convention.
func TestValidator_Validate_Fails_When_RequiredFieldMissing(t *testing.T) {
	t.Log("Testing Validate Failure: Message is missing required 'method' field for JSONRPCRequest.")
	validator := setupTestValidator(t) // Returns interface.
	ctx := context.Background()

	// Use generic request schema name from mapping.
	err := validator.Validate(ctx, "request", []byte(invalidMessageMissingMethod))

	require.Error(t, err, "Validation should fail when a required field ('method') is missing.")

	var validationErr *sch.ValidationError // Use qualified type.
	require.True(t, errors.As(err, &validationErr), "Error should be a *schema.ValidationError.")
	assert.Equal(t, sch.ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.") // Use qualified constant.

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

// TestValidator_Validate_Fails_When_FieldHasWrongType tests validation failure due to incorrect field type.
// Renamed function to follow ADR-008 convention.
func TestValidator_Validate_Fails_When_FieldHasWrongType(t *testing.T) {
	t.Log("Testing Validate Failure: Message has incorrect type for 'method' field.")
	validator := setupTestValidator(t) // Returns interface.
	ctx := context.Background()

	err := validator.Validate(ctx, "request", []byte(invalidMessageWrongType))

	require.Error(t, err, "Validation should fail when 'method' field has wrong type.")

	var validationErr *sch.ValidationError // Use qualified type.
	require.True(t, errors.As(err, &validationErr), "Error should be a *schema.ValidationError.")
	assert.Equal(t, sch.ErrValidationFailed, validationErr.Code, "Error code should be ErrValidationFailed.") // Use qualified constant.

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
	assert.True(t, foundTypeMismatch, "Expected validation cause for type mismatch at instance path '%s'.", expectedLocation)
}

// TestValidator_Validate_Fails_When_MessageIsInvalidJSON tests validation failure with invalid JSON syntax.
// Renamed function to follow ADR-008 convention.
func TestValidator_Validate_Fails_When_MessageIsInvalidJSON(t *testing.T) {
	t.Log("Testing Validate Failure: Input message has invalid JSON syntax.")
	validator := setupTestValidator(t) // Returns interface.
	ctx := context.Background()

	err := validator.Validate(ctx, "request", []byte(invalidJSONSyntaxMessage))

	require.Error(t, err, "Validation should fail when the input message is invalid JSON.")

	var validationErr *sch.ValidationError // Use qualified type.
	require.True(t, errors.As(err, &validationErr), "Error should be a *schema.ValidationError.")
	assert.Equal(t, sch.ErrInvalidJSONFormat, validationErr.Code, "Error code should be ErrInvalidJSONFormat.")     // Use qualified constant.
	assert.Contains(t, validationErr.Message, `Invalid JSON format`, "Error message should indicate invalid JSON.") // Check wrapper message.

	var syntaxError *json.SyntaxError
	isSyntaxError := errors.As(validationErr.Cause, &syntaxError) // Check the *cause* for the specific syntax error.
	assert.True(t, isSyntaxError, "Underlying cause should be a json.SyntaxError.")
	if isSyntaxError {
		t.Logf("Correctly identified underlying json.SyntaxError: %v.", syntaxError)
	}
}

// TestValidator_Validate_Fails_When_ValidatorNotInitialized tests calling Validate before Initialize.
// Renamed function to follow ADR-008 convention.
func TestValidator_Validate_Fails_When_ValidatorNotInitialized(t *testing.T) {
	t.Log("Testing Validate Failure: Calling Validate before Initialize.")
	logger := logging.GetNoopLogger()
	cfg := config.SchemaConfig{SchemaOverrideURI: ""} // Config doesn't matter here.
	validator := sch.NewValidator(cfg, logger)        // Use qualified call.
	// Not initialized.
	ctx := context.Background()

	err := validator.Validate(ctx, "request", []byte(validMessage))

	require.Error(t, err, "Validation should fail if validator is not initialized.")

	var validationErr *sch.ValidationError // Use qualified type.
	require.True(t, errors.As(err, &validationErr), "Error should be a *schema.ValidationError.")
	assert.Equal(t, sch.ErrSchemaNotFound, validationErr.Code, "Error code should indicate schema not found/uninitialized.") // Use qualified constant.
	assert.Contains(t, validationErr.Message, "validator not initialized", "Error message should indicate not initialized.")
}

// TestValidator_Shutdown_Succeeds_When_Called tests the Shutdown method.
// Renamed function to follow ADR-008 convention.
func TestValidator_Shutdown_Succeeds_When_Called(t *testing.T) {
	t.Log("Testing Shutdown: Ensures validator is marked uninitialized and resources cleared.")
	validatorInterface := setupTestValidator(t) // Use helper.
	require.True(t, validatorInterface.IsInitialized(), "Validator should be initialized before shutdown.")

	// Call Shutdown the first time.
	err := validatorInterface.Shutdown()
	assert.NoError(t, err, "First call to Shutdown should not return an error.")
	assert.False(t, validatorInterface.IsInitialized(), "Validator should be marked as uninitialized after shutdown.")

	// --- Check internal state (cannot easily check internal state now) ---.

	// Call Shutdown again (should be idempotent).
	err = validatorInterface.Shutdown()
	assert.NoError(t, err, "Calling Shutdown again should not return an error.")
	assert.False(t, validatorInterface.IsInitialized(), "Validator should remain uninitialized after second shutdown call.")
}

// --- NEW TEST: Verify schema mappings ---.
// TestValidator_AllSchemaDefinitionsHaveMappings verifies that all Request/Result
// definitions found in the compiled schema have corresponding entries in the
// schemaMappings variable used by addGenericMappings.
func TestValidator_AllSchemaDefinitionsHaveMappings(t *testing.T) {
	t.Log("Testing Mapping Completeness: Ensures all *Request/*Result schemas are mapped.")
	// Setup test validator (which calls Initialize and thus VerifyMappingsAgainstSchema internally).
	validator := setupTestValidator(t) // This helper initializes the validator.

	// Call VerifyMappingsAgainstSchema via the interface.
	unmapped := validator.VerifyMappingsAgainstSchema()

	// Assert that the list of unmapped schemas (that match *Request/*Result pattern) is empty.
	assert.Empty(t, unmapped, "Found compiled schema definitions ending in Request/Result without corresponding entries in the validator's schemaMappings variable: %v. Update schemaMappings in internal/schema/validator.go.", unmapped)
}

// --- END NEW TEST ---.
