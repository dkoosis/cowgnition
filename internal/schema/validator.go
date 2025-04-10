// file: internal/schema/validator.go
package schema

import (
	"bytes"
	"context"
	"crypto/sha256" // Import for checksum calculation (for future use).
	"encoding/hex"  // Import for checksum encoding (for future use).
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// SchemaSource defines where to load the schema from.
// nolint:revive // Keep semantic naming consistent with package, will refactor in future.
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
// nolint:revive // Keep semantic naming consistent with package, will refactor in future.
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
	// initialized indicates whether the validator has been initialized.
	initialized bool
	// logger for validation-related events.
	logger logging.Logger
	// lastLoadDuration stores the time taken for the last schema load.
	lastLoadDuration time.Duration
	// lastCompileDuration stores the time taken for the last schema compile.
	lastCompileDuration time.Duration
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
		Cause:   errors.WithStack(cause), // Preserve stack trace.
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(),
		},
	}
}

// NewSchemaValidator creates a new SchemaValidator with the given schema source.
func NewSchemaValidator(source SchemaSource, logger logging.Logger) *SchemaValidator {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	compiler := jsonschema.NewCompiler()

	// Set up draft-2020-12 dialect.
	compiler.Draft = jsonschema.Draft2020

	// Provide schemas for metaschema (required for draft-2020-12).
	compiler.AssertFormat = true
	compiler.AssertContent = true

	return &SchemaValidator{
		source:     source,
		compiler:   compiler,
		schemas:    make(map[string]*jsonschema.Schema),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger.WithField("component", "schema_validator"),
	}
}

// Initialize loads and compiles the MCP schema definitions.
// This should be called during application startup before any validation occurs.
func (v *SchemaValidator) Initialize(ctx context.Context) error {
	initStart := time.Now() // Start timing the whole initialization.
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if already initialized.
	if v.initialized {
		v.logger.Warn("Schema validator already initialized, skipping.")
		return nil
	}

	v.logger.Info("Initializing schema validator...")

	// --- Load Schema Data ---
	loadStart := time.Now()
	schemaData, err := v.loadSchemaData(ctx)
	v.lastLoadDuration = time.Since(loadStart) // Store load duration.
	if err != nil {
		// Log load duration even on failure.
		v.logger.Error("Schema loading failed.", "duration", v.lastLoadDuration, "error", err)
		return errors.Wrap(err, "failed to load schema data")
	}
	v.logger.Info("Schema loaded.", "duration", v.lastLoadDuration, "sizeBytes", len(schemaData))
	// TODO: Implement checksum verification here in the future.
	_ = sha256.Sum256(schemaData)    // Placeholder for checksum calculation.
	_ = hex.EncodeToString([]byte{}) // Placeholder for encoding.

	// --- Add Schema Resource ---
	addStart := time.Now()
	schemaReader := bytes.NewReader(schemaData)
	resourceID := "mcp-schema.json" // Or derive dynamically if needed.
	if err := v.compiler.AddResource(resourceID, schemaReader); err != nil {
		addDuration := time.Since(addStart)
		v.logger.Error("Failed to add schema resource to compiler.", "duration", addDuration, "resourceID", resourceID, "error", err)
		return NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to add schema resource to compiler",
			errors.Wrap(err, "compiler.AddResource failed"),
		).WithContext("schemaSize", len(schemaData))
	}
	v.logger.Info("Schema resource added.", "duration", time.Since(addStart), "resourceID", resourceID)

	// --- Compile Base Schema ---
	compileStart := time.Now()
	// Assuming the main schema entry point is the resourceID added above.
	baseSchema, err := v.compiler.Compile(resourceID)
	v.lastCompileDuration = time.Since(compileStart) // Store compile duration.
	if err != nil {
		// Log compile duration even on failure.
		v.logger.Error("Failed to compile base schema.", "duration", v.lastCompileDuration, "resourceID", resourceID, "error", err)
		return NewValidationError(
			ErrSchemaCompileFailed,
			"Failed to compile base schema",
			errors.Wrap(err, "compiler.Compile failed"),
		)
	}
	v.logger.Info("Base schema compiled.", "duration", v.lastCompileDuration, "resourceID", resourceID)

	// Store the schema.
	v.schemas["base"] = baseSchema // Assuming "base" is the key for the main schema.
	// TODO: Compile specific sub-schemas (Tool, Resource, etc.) if needed for validation.
	v.initialized = true
	initDuration := time.Since(initStart) // Total initialization time.

	v.logger.Info("Schema validator initialized successfully.",
		"totalDuration", initDuration,
		"loadDuration", v.lastLoadDuration,
		"compileDuration", v.lastCompileDuration,
		"schemas", getSchemaKeys(v.schemas))

	return nil
}

// GetLoadDuration returns the duration of the last schema load operation.
func (v *SchemaValidator) GetLoadDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastLoadDuration
}

// GetCompileDuration returns the duration of the last schema compile operation.
func (v *SchemaValidator) GetCompileDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastCompileDuration
}

// Shutdown performs any cleanup needed for the schema validator.
// Should be called during application shutdown.
func (v *SchemaValidator) Shutdown() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.initialized {
		return nil
	}

	v.logger.Info("Shutting down schema validator...")

	// Close HTTP client if needed.
	if transport, ok := v.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	// Clear cached schemas to free memory.
	v.schemas = nil
	v.initialized = false
	v.logger.Info("Schema validator shut down.")

	return nil
}

// IsInitialized returns whether the validator has been initialized.
func (v *SchemaValidator) IsInitialized() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.initialized
}

// compileSubSchema compiles a sub-schema from the base schema.
// nolint:unused // Reserved for future schema compilation features.
func (v *SchemaValidator) compileSubSchema(name, pointer string) error {
	// In the santhosh-tekuri/jsonschema/v5 library, we use Compile with a pointer.
	// instead of CompileWithID which doesn't exist.
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
	// Try to load from each source in order of preference.

	// 1. Try embedded schema if provided.
	if len(v.source.Embedded) > 0 {
		v.logger.Debug("Loading schema from embedded data.")
		return v.source.Embedded, nil
	}

	// 2. Try local file if path is provided.
	if v.source.FilePath != "" {
		v.logger.Debug("Attempting to load schema from file.", "path", v.source.FilePath)
		data, err := os.ReadFile(v.source.FilePath)
		if err == nil {
			v.logger.Debug("Successfully loaded schema from file.",
				"path", v.source.FilePath,
				"size", len(data))
			return data, nil
		}
		v.logger.Warn("Failed to load schema from file, will try URL next.",
			"path", v.source.FilePath,
			"error", err)
		// If file read failed, log and continue to next source.
		// We don't return error yet, we'll try URL next.
	}

	// 3. Try URL if provided.
	if v.source.URL != "" {
		v.logger.Debug("Attempting to load schema from URL.", "url", v.source.URL)
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
			bodyBytes, _ := io.ReadAll(resp.Body) // Read body for context.
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				fmt.Sprintf("Failed to fetch schema: HTTP status %d", resp.StatusCode),
				nil,
			).WithContext("url", v.source.URL).
				WithContext("statusCode", resp.StatusCode).
				WithContext("responseBody", string(bodyBytes)) // Add response body to error context.
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to read schema from HTTP response",
				errors.Wrap(err, "io.ReadAll failed"),
			).WithContext("url", v.source.URL)
		}

		v.logger.Debug("Successfully loaded schema from URL.",
			"url", v.source.URL,
			"size", len(data))
		return data, nil
	}

	// 4. If we get here, all sources failed.
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
	// Check if initialized.
	if !v.IsInitialized() {
		return NewValidationError(
			ErrSchemaNotFound,
			"Schema validator not initialized",
			nil,
		)
	}

	// First, ensure the data is valid JSON.
	var instance interface{}
	// Use json.Unmarshal into interface{} for validation as required by the library.
	if err := json.Unmarshal(data, &instance); err != nil {
		return NewValidationError(
			ErrInvalidJSONFormat,
			"Invalid JSON format",
			errors.Wrap(err, "json.Unmarshal failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data)) // Use helper here.
	}

	// Get the schema for the message type.
	v.mu.RLock()
	// Use "base" schema for now, until specific type compilation is added.
	// TODO: Select schema based on messageType if sub-schemas are compiled.
	schema, ok := v.schemas["base"]
	v.mu.RUnlock()

	if !ok {
		// If we don't have a specific schema for this message type, use the base schema.
		// This case should not happen if Initialize succeeded for "base".
		return NewValidationError(
			ErrSchemaNotFound,
			fmt.Sprintf("Base schema not found, though validator reported initialized. Type attempted: %s", messageType),
			nil,
		).WithContext("messageType", messageType).
			WithContext("availableSchemas", getSchemaKeys(v.schemas))
	}

	// Validate the instance against the schema.
	validationStart := time.Now() // Time individual validation call.
	err := schema.Validate(instance)
	validationDuration := time.Since(validationStart)

	if err != nil {
		// Convert jsonschema validation error to our custom error type.
		var valErr *jsonschema.ValidationError
		if errors.As(err, &valErr) {
			// Log validation duration on error.
			v.logger.Debug("Schema validation failed.", "duration", validationDuration, "messageType", messageType)
			return convertValidationError(valErr, messageType, data)
		}

		// For other types of errors during validation.
		v.logger.Error("Unexpected error during schema.Validate.", "duration", validationDuration, "messageType", messageType, "error", err)
		return NewValidationError(
			ErrValidationFailed,
			"Schema validation failed with unexpected error",
			errors.Wrap(err, "schema.Validate failed unexpectedly"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data)) // Use helper here.
	}
	// Log validation duration on success too, if needed for performance analysis.
	// v.logger.Debug("Schema validation successful.", "duration", validationDuration, "messageType", messageType)

	return nil
}

// convertValidationError converts a jsonschema.ValidationError to our custom ValidationError.
func convertValidationError(valErr *jsonschema.ValidationError, messageType string, data []byte) *ValidationError {
	// Extract error details.
	// In this library, the error details are in the Basic Output format described in JSON Schema spec.

	// Extract schema path and instance path from the error.
	// Note: The library's internal structure might make precise path extraction tricky directly.
	// We rely on the error message and potentially BasicOutput() for paths.
	var schemaPath string
	var instancePath string

	// Try to extract paths using BasicOutput().
	basicOutput := valErr.BasicOutput()
	if len(basicOutput.Errors) > 0 {
		// Find the most specific error detail (often the last one?).
		// This is heuristic as the library might nest errors differently.
		lastError := basicOutput.Errors[len(basicOutput.Errors)-1]
		instancePath = lastError.InstanceLocation
		schemaPath = lastError.KeywordLocation // KeywordLocation often points to the relevant schema part.
	} else {
		// Fallback if BasicOutput is empty - try parsing from message (less reliable).
		errorMsg := valErr.Error()
		// Simple parsing attempt - might need refinement based on actual error formats.
		if strings.Contains(errorMsg, "schema path:") {
			parts := strings.SplitN(errorMsg, "schema path:", 2)
			if len(parts) > 1 {
				schemaPath = strings.Split(strings.TrimSpace(parts[1]), " ")[0]
			}
		}
		if strings.Contains(errorMsg, "instance path:") {
			parts := strings.SplitN(errorMsg, "instance path:", 2)
			if len(parts) > 1 {
				instancePath = strings.Split(strings.TrimSpace(parts[1]), " ")[0]
			}
		}
	}

	// Create our custom error with the extracted paths.
	customErr := NewValidationError(
		ErrValidationFailed,
		valErr.Message, // Use the primary message from the validation error.
		valErr,         // Include the original error as cause.
	).WithContext("messageType", messageType).
		WithContext("dataPreview", calculatePreview(data)) // Use helper here.

	// Assign paths if found.
	if schemaPath != "" {
		customErr.SchemaPath = schemaPath
	}
	if instancePath != "" {
		customErr.InstancePath = instancePath
	}

	// Add basic info about the validation error causes.
	if len(valErr.Causes) > 0 {
		causes := make([]string, 0, len(valErr.Causes))
		for _, cause := range valErr.Causes {
			causes = append(causes, cause.Error())
		}
		// Use _ to explicitly ignore the error return value.
		_ = customErr.WithContext("causes", causes)
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

// HasSchema checks if a schema with the given name exists.
// This is useful for determining if specific method schemas are available.
// before attempting validation.
func (v *SchemaValidator) HasSchema(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, ok := v.schemas[name]
	return ok
}

// calculatePreview generates a string preview of a byte slice, limited to a max length.
// maxLength parameter was removed as it was always 100.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100 // Use a constant for the max length.
	previewLen := len(data)
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
	}
	return string(data[:previewLen])
}
