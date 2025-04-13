// file: internal/schema/validator.go
package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Assuming this path is correct
	"github.com/santhosh-tekuri/jsonschema/v5"
)

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
	// mu protects concurrent access to the schemas map and internal state like cache headers.
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
	// schemaVersion stores the detected version of the loaded schema.
	schemaVersion string
	// schemaETag stores the ETag from the last successful HTTP response for caching.
	schemaETag string
	// schemaLastModified stores the Last-Modified time from the last successful HTTP response for caching.
	schemaLastModified string
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
func (v *SchemaValidator) Initialize(ctx context.Context) error {
	initStart := time.Now() // Start timing the whole initialization.
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.initialized {
		v.logger.Warn("Schema validator already initialized, skipping.")
		return nil
	}
	v.logger.Info("Initializing schema validator...")

	// --- Load Schema Data ---
	loadStart := time.Now()
	// Pass the lock-acquired context (ctx here is outer context)
	schemaData, err := v.loadSchemaData(ctx)
	v.lastLoadDuration = time.Since(loadStart) // Store load duration.
	if err != nil {
		// loadSchemaData returns wrapped ValidationError on failure
		v.logger.Error("Schema loading failed.", "duration", v.lastLoadDuration, "error", err)
		return err // Return the wrapped error directly
	}
	// Check if loadSchemaData signaled "use existing compiled" (returned nil data, nil error)
	// This happens on HTTP 304 when local file cache also failed.
	if schemaData == nil {
		// Remove redundant nil check, len() is safe for nil maps.
		if len(v.schemas) == 0 {
			// This case should ideally not happen if 304 occurs, means initial load never happened.
			err := NewValidationError(ErrSchemaLoadFailed, "Schema unchanged (304) but no schema was previously loaded", nil)
			v.logger.Error("Schema loading failed.", "duration", v.lastLoadDuration, "error", err)
			return err
		}
		// If data is nil and error is nil, it means use existing cache (HTTP 304 case).
		// Mark as initialized if schemas exist.
		v.initialized = true
		v.logger.Info("Schema unchanged on source, re-using previously compiled schemas.", "loadDuration", v.lastLoadDuration)
		return nil // Initialization technically successful (using cached)
	}
	v.logger.Info("Schema loaded.", "duration", v.lastLoadDuration, "sizeBytes", len(schemaData))

	// Try to extract version information from the newly loaded data
	v.extractSchemaVersion(schemaData)

	// --- Add Schema Resource ---
	addStart := time.Now()
	schemaReader := bytes.NewReader(schemaData)
	// Use a base URI that can be referenced internally, like "mcp://"
	resourceID := "mcp://schema.json"
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

	// --- Compile Base Schema and Key Definitions ---
	compileStart := time.Now()
	// Define schemas relative to the added resource ID
	schemasToCompile := map[string]string{
		"base":                resourceID, // Compile the root.
		"JSONRPCRequest":      resourceID + "#/definitions/JSONRPCRequest",
		"JSONRPCNotification": resourceID + "#/definitions/JSONRPCNotification",
		"JSONRPCResponse":     resourceID + "#/definitions/JSONRPCResponse",
		"JSONRPCError":        resourceID + "#/definitions/JSONRPCError",
		"InitializeRequest":   resourceID + "#/definitions/InitializeRequest",
		"InitializeResult":    resourceID + "#/definitions/InitializeResult",
		"ListToolsRequest":    resourceID + "#/definitions/ListToolsRequest",
		"ListToolsResult":     resourceID + "#/definitions/ListToolsResult",
		"CallToolRequest":     resourceID + "#/definitions/CallToolRequest",
		"CallToolResult":      resourceID + "#/definitions/CallToolResult",
		"ping":                resourceID + "#/definitions/PingRequest", // Assuming PingRequest definition exists
		"ping_notification":   resourceID + "#/definitions/PingRequest", // Reuse if notification has same params
		// Add other common/required definitions...
		"success_response": resourceID + "#/definitions/JSONRPCResponse", // Map generic to base JSON-RPC
		"error_response":   resourceID + "#/definitions/JSONRPCError",    // Map generic to base JSON-RPC
	}

	compiledSchemas := make(map[string]*jsonschema.Schema)
	var firstCompileError error

	for name, pointer := range schemasToCompile {
		schema, err := v.compiler.Compile(pointer)
		if err != nil {
			v.logger.Warn("Failed to compile schema definition.", "name", name, "pointer", pointer, "error", err)
			// Store the first error encountered, but try to compile others.
			if firstCompileError == nil {
				firstCompileError = NewValidationError(
					ErrSchemaCompileFailed,
					fmt.Sprintf("Failed to compile schema definition '%s'", name),
					errors.Wrap(err, "compiler.Compile failed"),
				).WithContext("pointer", pointer)
			}
		} else {
			compiledSchemas[name] = schema
			v.logger.Debug("Compiled schema definition.", "name", name)
		}
	}

	v.lastCompileDuration = time.Since(compileStart) // Store total compile duration.

	if firstCompileError != nil {
		v.logger.Error("Schema compilation finished with errors.", "duration", v.lastCompileDuration, "firstError", firstCompileError)
		return firstCompileError // Return the first critical error encountered.
	}

	v.schemas = compiledSchemas // Assign successfully compiled schemas.
	v.initialized = true
	initDuration := time.Since(initStart)

	v.logger.Info("Schema validator initialized successfully.",
		"totalDuration", initDuration,
		"loadDuration", v.lastLoadDuration,
		"compileDuration", v.lastCompileDuration,
		"schemaVersion", v.schemaVersion, // Log detected version
		"schemasCompiled", getSchemaKeys(v.schemas))

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

	// Close HTTP client idle connections.
	if transport, ok := v.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	} else {
		// If using default transport, create one to close connections
		// This might not be strictly necessary but ensures cleanup attempt
		if dt, ok := http.DefaultTransport.(*http.Transport); ok {
			dt.CloseIdleConnections()
		}
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

// Validate validates the given JSON data against the schema for the specified message type.
// Refactored to reduce cyclomatic complexity by extracting schema lookup logic.
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
	if err := json.Unmarshal(data, &instance); err != nil {
		return NewValidationError(
			ErrInvalidJSONFormat,
			"Invalid JSON format",
			errors.Wrap(err, "json.Unmarshal failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data)) // Use helper here.
	}

	// Get the specific schema for the message type using the helper method.
	schema, schemaUsedKey, ok := v.getSchemaForMessageType(messageType)
	if !ok {
		// If we still don't have a schema (not even base), initialization likely failed partially.
		v.mu.RLock()
		availableKeys := getSchemaKeys(v.schemas) // RLock is needed here for availableSchemas
		v.mu.RUnlock()
		return NewValidationError(
			ErrSchemaNotFound,
			fmt.Sprintf("Schema definition not found for message type '%s' or standard fallbacks.", messageType),
			nil,
		).WithContext("messageType", messageType).
			WithContext("availableSchemas", availableKeys)
	}

	// Validate the instance against the schema.
	validationStart := time.Now()
	err := schema.Validate(instance)
	validationDuration := time.Since(validationStart)

	if err != nil {
		// Convert jsonschema validation error to our custom error type.
		var valErr *jsonschema.ValidationError
		if errors.As(err, &valErr) {
			v.logger.Debug("Schema validation failed.",
				"duration", validationDuration,
				"messageType", messageType,
				"schemaUsed", schemaUsedKey, // Log which schema was actually used
				"error", valErr.Error()) // Log basic error message
			return convertValidationError(valErr, messageType, data)
		}

		// For other types of errors during validation.
		v.logger.Error("Unexpected error during schema.Validate.",
			"duration", validationDuration,
			"messageType", messageType,
			"schemaUsed", schemaUsedKey,
			"error", err)
		return NewValidationError(
			ErrValidationFailed,
			"Schema validation failed with unexpected error",
			errors.Wrap(err, "schema.Validate failed unexpectedly"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data))
	}

	v.logger.Debug("Schema validation successful.",
		"duration", validationDuration,
		"messageType", messageType,
		"schemaUsed", schemaUsedKey)
	return nil
}

// getSchemaForMessageType finds the appropriate compiled schema based on the message type,
// including fallback logic. Returns the schema, the key used to find it, and success status.
func (v *SchemaValidator) getSchemaForMessageType(messageType string) (*jsonschema.Schema, string, bool) {
	v.mu.RLock() // Read lock needed to access v.schemas
	defer v.mu.RUnlock()

	// Try specific message type first.
	if schema, ok := v.schemas[messageType]; ok {
		return schema, messageType, true
	}

	// Fallback logic based on message type patterns.
	var fallbackKey string
	if strings.HasSuffix(messageType, "_notification") {
		fallbackKey = "JSONRPCNotification"
	} else if strings.Contains(messageType, "Response") || strings.Contains(messageType, "Result") || strings.Contains(messageType, "_response") {
		fallbackKey = "JSONRPCResponse"
		if _, ok := v.schemas[fallbackKey]; !ok {
			// If generic response not found, try generic error.
			if strings.Contains(messageType, "Error") {
				fallbackKey = "JSONRPCError"
			}
		}
	} else { // Assume request if not notification/response/error.
		fallbackKey = "JSONRPCRequest"
	}

	// Check if the fallback schema exists.
	if schema, ok := v.schemas[fallbackKey]; ok {
		v.logger.Debug("Using fallback schema.", "originalType", messageType, "fallbackKey", fallbackKey)
		return schema, fallbackKey, true
	}

	// Final fallback to the base schema.
	if schema, ok := v.schemas["base"]; ok {
		v.logger.Debug("Using base schema as final fallback.", "originalType", messageType)
		return schema, "base", true
	}

	// No suitable schema found.
	return nil, "", false
}

// compileSubSchema compiles a sub-schema from the base schema.
// nolint:unused // Reserved for future dynamic schema compilation features.
func (v *SchemaValidator) compileSubSchema(name, pointer string) error {
	// This method requires the main lock because it modifies v.schemas
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if already compiled
	if _, exists := v.schemas[name]; exists {
		return nil // Already exists
	}

	// Ensure the compiler is ready (should be if initialized)
	if v.compiler == nil {
		return NewValidationError(ErrSchemaCompileFailed, "Compiler not available", nil)
	}

	subSchema, err := v.compiler.Compile(pointer)
	if err != nil {
		return NewValidationError(
			ErrSchemaCompileFailed,
			fmt.Sprintf("Failed to compile %s schema", name),
			errors.Wrap(err, fmt.Sprintf("compiler.Compile failed for %s", name)),
		).WithContext("schemaPointer", pointer)
	}

	// Add to the map
	if v.schemas == nil {
		v.schemas = make(map[string]*jsonschema.Schema)
	}
	v.schemas[name] = subSchema
	v.logger.Info("Dynamically compiled and added sub-schema.", "name", name, "pointer", pointer)
	return nil
}

// HasSchema checks if a schema with the given name exists.
// This is useful for determining if specific method schemas are available
// before attempting validation.
func (v *SchemaValidator) HasSchema(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	// Ensure schemas map is initialized
	if v.schemas == nil {
		return false
	}
	_, ok := v.schemas[name]
	return ok
}

// getSchemaKeys returns the keys of the schemas map for debugging purposes.
// Assumes lock is held by caller if needed.
func getSchemaKeys(schemas map[string]*jsonschema.Schema) []string {
	if schemas == nil {
		return []string{}
	}
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	return keys
}
