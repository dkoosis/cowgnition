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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Assuming this path is correct
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// SchemaSource defines where to load the schema from.
// nolint:revive // Keep semantic naming consistent with package, will refactor in future.
type SchemaSource struct {
	// URL is the remote location of the schema, if applicable.
	URL string
	// FilePath is the local file path of the schema, if applicable (also used for caching URL fetches).
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
		// *** Fix for gosimple (S1009) ***
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

// loadSchemaData orchestrates loading schema data from the configured source.
// It requires the main lock (`v.mu`) to be held by the caller (e.g., Initialize)
// because it modifies cache headers (`v.schemaETag`, `v.schemaLastModified`).
func (v *SchemaValidator) loadSchemaData(ctx context.Context) ([]byte, error) {
	// 1. Try embedded schema first.
	if data, err := v.loadSchemaFromEmbedded(); err == nil {
		return data, nil
	} // Error is not possible currently, but good practice

	// 2. Try local file second.
	// We load it here but might discard if URL provides newer data or indicates no change.
	var initialFileData []byte
	var initialFileErr error
	if v.source.FilePath != "" {
		initialFileData, initialFileErr = v.loadSchemaFromFile(v.source.FilePath)
		if initialFileErr != nil {
			v.logger.Warn("Failed to load schema from file initially, will try URL if configured.",
				"path", v.source.FilePath,
				"error", initialFileErr)
			// Don't return error yet, URL might succeed.
		} else {
			v.logger.Debug("Successfully loaded schema from file initially.",
				"path", v.source.FilePath,
				"size", len(initialFileData))
			// If no URL is configured, this file data is definitive.
			if v.source.URL == "" {
				// v.extractSchemaVersion(initialFileData) // Version extracted in Initialize now
				return initialFileData, nil
			}
		}
	}

	// 3. Try URL third (if configured).
	if v.source.URL != "" {
		// Pass lock-acquired context to HTTP fetch
		data, status, err := v.fetchSchemaFromURL(ctx)
		if err != nil {
			// If URL fetch fails completely, return the error.
			// We don't fall back to potentially stale file data in this case.
			return nil, err // fetchSchemaFromURL already wraps the error
		}

		switch status {
		case http.StatusOK:
			// Successfully fetched new data from URL.
			v.logger.Debug("Successfully loaded schema from URL.", "url", v.source.URL, "size", len(data))
			// v.extractSchemaVersion(data) // Version extracted in Initialize now
			v.tryCacheSchemaToFile(data) // Attempt to cache the freshly downloaded data
			return data, nil
		case http.StatusNotModified:
			// Schema hasn't changed on the server.
			v.logger.Info("Schema not modified since last fetch (HTTP 304)", "url", v.source.URL)
			// Use the data we loaded from the file earlier (if successful).
			if initialFileErr == nil {
				v.logger.Debug("Using initially loaded file data as schema is unchanged.", "path", v.source.FilePath)
				// Ensure version is extracted from file data if we haven't done it yet.
				// if v.schemaVersion == "" {
				// 	v.extractSchemaVersion(initialFileData) // Done in Initialize now
				// }
				return initialFileData, nil
			}
			// If file load failed BUT we got 304, it implies a previous successful load must have happened.
			// Signal to the caller (Initialize) to use the already compiled schemas.
			v.logger.Warn("Schema unchanged (304) but failed to load local file cache. Signaling to use previously compiled schemas if available.",
				"path", v.source.FilePath,
				"fileError", initialFileErr)
			return nil, nil // Signal "use existing compiled"
		default:
			// Should not happen if fetchSchemaFromURL is correct, but handle defensively.
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				fmt.Sprintf("unexpected HTTP status %d from URL fetch", status),
				nil,
			).WithContext("url", v.source.URL).WithContext("statusCode", status)
		}
	}

	// 4. If only FilePath was specified and it failed initially.
	if v.source.FilePath != "" && v.source.URL == "" && initialFileErr != nil {
		// Wrap the initial file error properly if it hasn't been wrapped yet.
		var validationErr *ValidationError
		if errors.As(initialFileErr, &validationErr) {
			return nil, initialFileErr // Already a validation error
		}
		// Wrap the raw file error
		return nil, NewValidationError(ErrSchemaLoadFailed, "Failed to load schema from file", initialFileErr).
			WithContext("path", v.source.FilePath)
	}

	// 5. If we get here, no source was configured or loadable.
	return nil, NewValidationError(
		ErrSchemaNotFound,
		"No valid schema source configured or loadable",
		nil,
	).WithContext("sourcesChecked", map[string]interface{}{
		"embedded": len(v.source.Embedded) > 0,
		"filePath": v.source.FilePath,
		"url":      v.source.URL,
	})
}

// loadSchemaFromEmbedded loads schema from embedded bytes.
// Does not require lock.
func (v *SchemaValidator) loadSchemaFromEmbedded() ([]byte, error) {
	if len(v.source.Embedded) > 0 {
		v.logger.Debug("Loading schema from embedded data.", "size", len(v.source.Embedded))
		// Version extraction happens later in Initialize.
		return v.source.Embedded, nil
	}
	return nil, errors.New("no embedded schema provided") // Internal signal, not user-facing error
}

// loadSchemaFromFile loads schema from a local file path.
// Does not require lock.
func (v *SchemaValidator) loadSchemaFromFile(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Wrap error for context, but don't classify as ValidationError yet.
		return nil, errors.Wrapf(err, "failed to read schema file: %s", filePath)
	}
	// Version extraction happens later in Initialize.
	return data, nil
}

// fetchSchemaFromURL fetches the schema from a URL, handling caching headers.
// Returns data, HTTP status code, and error.
// It requires the main lock (`v.mu`) to be held by the caller
// because it reads and potentially modifies cache headers (`v.schemaETag`, `v.schemaLastModified`).
func (v *SchemaValidator) fetchSchemaFromURL(ctx context.Context) ([]byte, int, error) {
	v.logger.Debug("Attempting to load schema from URL.", "url", v.source.URL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.source.URL, nil)
	if err != nil {
		return nil, 0, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to create HTTP request for schema URL",
			errors.Wrap(err, "http.NewRequestWithContext failed"),
		).WithContext("url", v.source.URL)
	}

	// Add caching headers IF THEY EXIST from previous successful requests.
	// Lock is held by caller (Initialize).
	if v.schemaETag != "" {
		req.Header.Set("If-None-Match", v.schemaETag)
		v.logger.Debug("Using cached ETag for conditional request", "etag", v.schemaETag)
	}
	if v.schemaLastModified != "" {
		req.Header.Set("If-Modified-Since", v.schemaLastModified)
		v.logger.Debug("Using cached Last-Modified for conditional request", "lastModified", v.schemaLastModified)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		// Clear potentially stale cache headers on network error
		v.schemaETag = ""
		v.schemaLastModified = ""
		return nil, 0, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to fetch schema from URL",
			errors.Wrap(err, "httpClient.Do failed"),
		).WithContext("url", v.source.URL)
	}
	defer resp.Body.Close()

	// Handle non-OK statuses before reading body (except 304)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		bodyBytes, _ := io.ReadAll(resp.Body) // Read body for context, ignore read error
		// Clear potentially stale cache headers on server error status
		v.schemaETag = ""
		v.schemaLastModified = ""
		return nil, resp.StatusCode, NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Failed to fetch schema: HTTP status %d", resp.StatusCode),
			nil, // No underlying Go error, it's an HTTP error status
		).WithContext("url", v.source.URL).
			WithContext("statusCode", resp.StatusCode).
			WithContext("responseBody", string(bodyBytes))
	}

	// If status is 304, return immediately with no data and the status code.
	// Cache headers remain unchanged.
	if resp.StatusCode == http.StatusNotModified {
		return nil, http.StatusNotModified, nil
	}

	// Status must be 200 OK here. Update ETag/Last-Modified from response headers.
	// Lock is held by caller (Initialize).
	newETag := resp.Header.Get("ETag")
	newLastModified := resp.Header.Get("Last-Modified")
	if newETag != v.schemaETag || newLastModified != v.schemaLastModified {
		v.logger.Debug("Received new schema HTTP cache headers", "etag", newETag, "lastModified", newLastModified)
		v.schemaETag = newETag                 // Update validator's state
		v.schemaLastModified = newLastModified // Update validator's state
	}

	// Read the response body.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		// Clear potentially stale cache headers if read fails after OK status
		v.schemaETag = ""
		v.schemaLastModified = ""
		return nil, resp.StatusCode, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to read schema from HTTP response",
			errors.Wrap(err, "io.ReadAll failed"),
		).WithContext("url", v.source.URL)
	}

	return data, http.StatusOK, nil
}

// tryCacheSchemaToFile attempts to write data to the configured cache file path.
// Logs info on success, warning on failure.
// Does not require lock.
func (v *SchemaValidator) tryCacheSchemaToFile(data []byte) {
	if v.source.FilePath == "" {
		return // No cache file path configured
	}

	dir := filepath.Dir(v.source.FilePath)
	// Use 0750 permissions: Owner rwx, Group rx, Other ---
	if err := os.MkdirAll(dir, 0750); err != nil {
		v.logger.Warn("Failed to create directory for schema cache file",
			"path", dir,
			"error", err)
		return // Can't create directory, so can't write file
	}

	// Use 0600 permissions: Owner rw, Group ---, Other --- (Gosec G306 fix)
	if err := os.WriteFile(v.source.FilePath, data, 0600); err != nil {
		v.logger.Warn("Failed to cache fetched schema to local file",
			"path", v.source.FilePath,
			"error", err)
	} else {
		v.logger.Info("Cached fetched schema to local file", "path", v.source.FilePath)
	}
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

	// *** Fix for errcheck: Assign result back to customErr ***
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
		// *** Fix for errcheck: Assign result back to customErr ***
		customErr = customErr.WithContext("validationErrors", causes)
	}

	return customErr
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

// calculatePreview generates a string preview of a byte slice, limited to a max length.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100 // Use a constant for the max length.
	previewLen := len(data)
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
	}
	// Replace non-printable characters for cleaner logging/error messages potentially
	// This is a simple replacement, more robust handling might be needed.
	previewBytes := bytes.Map(func(r rune) rune {
		if r < 32 || r == 127 { // Control characters + DEL
			return '.' // Replace with dot
		}
		return r
	}, data[:previewLen])

	return string(previewBytes)
}

// extractSchemaVersion attempts to extract version information from schema data.
// Assumes lock is held by caller if needed (when modifying v.schemaVersion).
// Refactored for lower cyclomatic complexity.
func (v *SchemaValidator) extractSchemaVersion(data []byte) {
	var schemaDoc map[string]interface{}
	if err := json.Unmarshal(data, &schemaDoc); err != nil {
		v.logger.Warn("Failed to unmarshal schema to extract version.", "error", err)
		return // Silently fail, this is just for metadata
	}

	detectedVersion := v.getVersionFromSchemaField(schemaDoc)

	if detectedVersion == "" {
		detectedVersion = v.getVersionFromTopLevelFields(schemaDoc)
	}

	if detectedVersion == "" {
		detectedVersion = v.getVersionFromInfoBlock(schemaDoc)
	}

	if detectedVersion == "" {
		detectedVersion = v.getVersionFromMCPHeuristics(schemaDoc)
	}

	// Update the validator state if a new, non-empty version was detected
	if detectedVersion != "" && detectedVersion != v.schemaVersion {
		v.logger.Info("Detected schema version.", "version", detectedVersion)
		v.schemaVersion = detectedVersion // Assumes lock is held by caller
	} else if detectedVersion == "" && v.schemaVersion == "" {
		v.logger.Debug("Could not detect specific version information in the schema.")
	}
}

// --- Helper functions for extractSchemaVersion ---

func (v *SchemaValidator) getVersionFromSchemaField(schemaDoc map[string]interface{}) string {
	if schemaField, ok := schemaDoc["$schema"].(string); ok {
		if strings.Contains(schemaField, "draft-2020-12") || strings.Contains(schemaField, "draft/2020-12") {
			return "draft-2020-12"
		}
		if strings.Contains(schemaField, "draft-07") {
			return "draft-07"
		}
		// Add more draft checks if needed
	}
	return ""
}

func (v *SchemaValidator) getVersionFromTopLevelFields(schemaDoc map[string]interface{}) string {
	if versionField, ok := schemaDoc["version"].(string); ok {
		return versionField
	}
	return ""
}

func (v *SchemaValidator) getVersionFromInfoBlock(schemaDoc map[string]interface{}) string {
	if infoBlock, ok := schemaDoc["info"].(map[string]interface{}); ok {
		if versionField, ok := infoBlock["version"].(string); ok {
			return versionField
		}
	}
	return ""
}

func (v *SchemaValidator) getVersionFromMCPHeuristics(schemaDoc map[string]interface{}) string {
	idRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`) // Basic YYYY-MM-DD pattern

	if id, ok := schemaDoc["$id"].(string); ok && strings.Contains(id, "modelcontextprotocol") {
		if matches := idRegex.FindStringSubmatch(id); len(matches) > 1 {
			return matches[1] // Prefer version from $id
		}
	}

	if title, ok := schemaDoc["title"].(string); ok && strings.Contains(strings.ToLower(title), "mcp") {
		if matches := idRegex.FindStringSubmatch(title); len(matches) > 1 {
			return matches[1] // Fallback to version from title
		}
	}

	return ""
}

// GetSchemaVersion returns the detected schema version, if available.
func (v *SchemaValidator) GetSchemaVersion() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.schemaVersion
}
