// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// file: internal/schema/validator.go
//
// The validator implementation orchestrates the schema handling process:
// 1. Loading: schema content is retrieved either from a configured URI or from embedded content.
// 2. Parsing: JSON schema is parsed into an in-memory structure.
// 3. Compilation: Schema definitions are compiled for validation use.
// 4. Validation: Incoming/outgoing messages are validated against compiled schemas.
//
// The validator maintains compiled schemas in memory with appropriate thread safety,
// supports schema version detection, and provides diagnostic metrics on loading and
// compilation times. It also implements graceful shutdown and resource cleanup.
package schema

import (
	"bytes"
	"context"
	_ "embed" // Required for go:embed.
	"encoding/json"
	"fmt"
	"net/http"
	"strings" // Keep strings import.
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema.json
var embeddedSchemaContent []byte

// ValidatorInterface defines the methods needed for schema validation.
type ValidatorInterface interface {
	Validate(ctx context.Context, messageType string, data []byte) error
	HasSchema(name string) bool
	IsInitialized() bool
	Initialize(ctx context.Context) error
	GetLoadDuration() time.Duration
	GetCompileDuration() time.Duration
	GetSchemaVersion() string
	Shutdown() error
}

// Validator handles loading, compiling, and validating against JSON schemas.
type Validator struct {
	schemaConfig        config.SchemaConfig // Store the config provided at creation.
	compiler            *jsonschema.Compiler
	schemas             map[string]*jsonschema.Schema
	schemaDoc           map[string]interface{}
	mu                  sync.RWMutex
	httpClient          *http.Client
	initialized         bool
	logger              logging.Logger
	lastLoadDuration    time.Duration
	lastCompileDuration time.Duration
	schemaVersion       string
}

// Ensure Validator implements the interface.
var _ ValidatorInterface = (*Validator)(nil)

// NewValidator creates a new Validator instance with the given schema configuration.
func NewValidator(cfg config.SchemaConfig, logger logging.Logger) *Validator {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	compiler.AssertContent = true

	return &Validator{
		schemaConfig: cfg, // Store the passed config.
		compiler:     compiler,
		schemas:      make(map[string]*jsonschema.Schema),
		schemaDoc:    make(map[string]interface{}),
		httpClient:   &http.Client{Timeout: 30 * time.Second}, // Kept for potential override loading.
		logger:       logger.WithField("component", "schema_validator"),
	}
}

// Initialize loads and compiles the MCP schema definitions.
// It prioritizes the embedded schema unless SchemaOverrideURI is set in the config.
// If SchemaOverrideURI is set but fails to load, initialization fails.
func (v *Validator) Initialize(ctx context.Context) error {
	initStart := time.Now()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.initialized {
		v.logger.Warn("Schema validator already initialized, skipping.")
		return nil
	}
	v.logger.Info("Initializing schema validator...") // Added ellipsis for clarity.

	var schemaData []byte
	var sourceInfo string // Variable to store the definitive source.
	var loadErr error

	loadStart := time.Now()

	// Check for explicit override first.
	if v.schemaConfig.SchemaOverrideURI != "" {
		v.logger.Info("SchemaOverrideURI is set, attempting to load external schema.", "uri", v.schemaConfig.SchemaOverrideURI) // Added period.
		// Use the loader function (defined in loader.go).
		schemaData, loadErr = loadSchemaFromURI(ctx, v.schemaConfig.SchemaOverrideURI, v.logger, v.httpClient)
		if loadErr != nil {
			// CRITICAL: Override was requested but failed. Do NOT fall back.
			v.logger.Error("CRITICAL: Failed to load schema from configured SchemaOverrideURI. Initialization aborted.",
				"uri", v.schemaConfig.SchemaOverrideURI, "error", loadErr) // Added period.
			// Wrap the specific loading error.
			return errors.Wrapf(loadErr, "failed to load schema from configured override URI '%s'", v.schemaConfig.SchemaOverrideURI)
		}
		// ** Explicitly set source info **.
		sourceInfo = fmt.Sprintf("override URI: %s", v.schemaConfig.SchemaOverrideURI)
		v.logger.Info("Successfully loaded schema from override URI.") // Added period.
	} else {
		// No override, use embedded schema.
		v.logger.Info("No SchemaOverrideURI configured, using embedded schema.") // Added period.
		if len(embeddedSchemaContent) == 0 {
			// CRITICAL: No override AND embedded schema is empty. Cannot proceed.
			err := errors.New("embedded schema content is empty and no override URI was provided")
			v.logger.Error("CRITICAL: Embedded schema is empty. Initialization aborted.", "error", err) // Added period.
			// Use the custom error type for consistency.
			return NewValidationError(ErrSchemaLoadFailed, "Embedded schema content is empty", err)
		}
		schemaData = embeddedSchemaContent
		// ** Explicitly set source info **.
		sourceInfo = "embedded"
		// *** Removed ineffectual assignment: loadErr = nil ***
		v.logger.Info("Using embedded schema content.") // Added period.
	}

	v.lastLoadDuration = time.Since(loadStart)

	// Check if schema data is empty after potentially loading.
	if len(schemaData) == 0 {
		// This case should technically be caught above, but double-check.
		err := errors.New("loaded schema data is empty")
		v.logger.Error("Schema loading resulted in empty data. Initialization aborted.", "source", sourceInfo, "error", err) // Added period.
		return NewValidationError(ErrSchemaLoadFailed, "Loaded schema data is empty", err)
	}

	v.logger.Info("Schema content loaded.", // Changed log message slightly. Added period.
		"duration", v.lastLoadDuration,
		// "source" field removed here, will be added in the final log message.
		"sizeBytes", len(schemaData))

	// --- Compile the loaded schemaData (rest of the function is similar) ---.

	// Parse Schema JSON.
	// Use a temporary variable for parsing to avoid modifying v.schemaDoc before success.
	var parsedDoc map[string]interface{}
	if err := json.Unmarshal(schemaData, &parsedDoc); err != nil {
		v.logger.Error("Failed to parse loaded schema JSON.", "error", err, "source", sourceInfo) // Added period.
		return NewValidationError(ErrSchemaLoadFailed, "Failed to parse schema JSON", errors.Wrap(err, "json.Unmarshal failed")).
			WithContext("source", sourceInfo)
	}

	// --- Extract Version BEFORE compilation ---.
	// Ensure schemaVersion is set before the final success log.
	// Assumes extractSchemaVersion updates v.schemaVersion.
	v.extractSchemaVersion(schemaData) // This method now belongs to Validator.

	// Create a new compiler instance *for this initialization attempt*.
	// This ensures retries (if implemented) or subsequent initializations don't reuse stale compiler state.
	// NOTE: ADR doesn't specify retries, but this is good practice.
	v.compiler = jsonschema.NewCompiler() // Re-init compiler.
	v.compiler.Draft = jsonschema.Draft2020
	v.compiler.AssertFormat = true
	v.compiler.AssertContent = true

	// Add Schema Resource.
	addStart := time.Now()
	schemaReader := bytes.NewReader(schemaData)
	resourceID := "mcp://schema.json" // Base URI for internal references.
	if err := v.compiler.AddResource(resourceID, schemaReader); err != nil {
		v.logger.Error("Failed to add schema resource to compiler.",
			"duration", time.Since(addStart),
			"resourceID", resourceID,
			"source", sourceInfo,
			"error", err) // Added period.
		return NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to add schema resource",
			errors.Wrap(err, "compiler.AddResource failed"),
		).WithContext("source", sourceInfo).WithContext("schemaSize", len(schemaData))
	}
	v.logger.Info("Schema resource added.", "duration", time.Since(addStart), "resourceID", resourceID) // Added period.

	// Compile Schemas.
	compileStart := time.Now()
	// Pass the parsedDoc to the compile function.
	compiledSchemas, compileErr := v.compileAllDefinitions(resourceID, parsedDoc)
	v.lastCompileDuration = time.Since(compileStart)

	if compileErr != nil {
		v.logger.Error("Schema compilation finished with errors. Initialization aborted.", "duration", v.lastCompileDuration, "firstError", compileErr) // Added period.
		return compileErr                                                                                                                               // Return the first critical error encountered.
	}

	// --- Success: Update Validator State ---.
	v.schemaDoc = parsedDoc     // Store the successfully parsed document.
	v.schemas = compiledSchemas // Store the successfully compiled schemas.
	v.initialized = true
	initDuration := time.Since(initStart)

	// --- Modified Final Success Log ---.
	// Ensure schemaVersion and sourceInfo are included here.
	finalSchemaVersion := v.schemaVersion // Use the potentially updated field.
	if finalSchemaVersion == "" {
		finalSchemaVersion = "[unknown]" // Fallback if extraction failed.
	}
	v.logger.Info("✅ Schema validator initialized successfully.", // Added checkmark and period.
		"totalDuration", initDuration.String(), // Use .String() for readability.
		"loadDuration", v.lastLoadDuration.String(),
		"compileDuration", v.lastCompileDuration.String(),
		"schemaVersion", finalSchemaVersion, // Use the variable.
		"schemasCompiled", len(v.schemas),
		"schemaSource", sourceInfo) // Add the definitive source.

	return nil
}

// compileAllDefinitions compiles the base schema and all found definitions.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) compileAllDefinitions(baseResourceID string, schemaDoc map[string]interface{}) (map[string]*jsonschema.Schema, error) {
	compiled := make(map[string]*jsonschema.Schema)
	var firstCompileError error

	// Compile base first.
	baseSchema, err := v.compiler.Compile(baseResourceID)
	if err != nil {
		v.logger.Error("CRITICAL: Failed to compile base schema resource.", "resourceID", baseResourceID, "error", err) // Added period.
		return nil, NewValidationError(
			ErrSchemaCompileFailed,
			"Failed to compile base schema resource",
			errors.Wrap(err, "compiler.Compile failed for base schema"),
		).WithContext("pointer", baseResourceID)
	}
	compiled["base"] = baseSchema
	v.logger.Debug("Compiled base schema definition.", "name", "base") // Added period.

	// Compile definitions using the passed-in schemaDoc.
	if defs, ok := schemaDoc["definitions"].(map[string]interface{}); ok {
		for name := range defs {
			pointer := baseResourceID + "#/definitions/" + name
			schemaCompiled, errCompile := v.compiler.Compile(pointer)
			if errCompile != nil {
				v.logger.Warn("Failed to compile schema definition.", "name", name, "pointer", pointer, "error", errCompile) // Added period.
				if firstCompileError == nil {
					firstCompileError = NewValidationError(
						ErrSchemaCompileFailed,
						fmt.Sprintf("Failed to compile schema definition '%s'", name),
						errors.Wrap(errCompile, "compiler.Compile failed"),
					).WithContext("pointer", pointer)
				}
			} else {
				compiled[name] = schemaCompiled
				v.logger.Debug("Compiled schema definition.", "name", name) // Added period.
			}
		}
	} else {
		v.logger.Warn("No 'definitions' section found in the schema JSON.") // Added period.
	}

	// Add generic mappings after compiling definitions.
	v.addGenericMappings(compiled) // Assuming this doesn't need schemaDoc.

	return compiled, firstCompileError
}

// addGenericMappings creates mappings from generic type names to specific schema definitions.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) addGenericMappings(compiledSchemas map[string]*jsonschema.Schema) {
	mappings := map[string][]string{
		"success_response":        {"JSONRPCResponse", "Response"},
		"error_response":          {"JSONRPCError", "Error"},
		"ping_notification":       {"PingRequest", "PingNotification", "JSONRPCNotification"}, // Corrected: Ping is often request-like.
		"notification":            {"JSONRPCNotification", "Notification"},
		"request":                 {"JSONRPCRequest", "Request"},
		"CallToolResult":          {"CallToolResult", "ToolResult"}, // Corrected: Added CallToolResult mapping.
		"initialize_response":     {"InitializeResult"},
		"tools/list_response":     {"ListToolsResult"},
		"resources/list_response": {"ListResourcesResult"},
		"prompts/list_response":   {"ListPromptsResult"},
		// Add more mappings as needed based on schema definitions.
	}
	mapped := make([]string, 0)
	for genericName, potentialTargets := range mappings {
		if _, exists := compiledSchemas[genericName]; exists {
			continue // Don't overwrite if already explicitly defined.
		}
		for _, targetDef := range potentialTargets {
			if targetSchema, ok := compiledSchemas[targetDef]; ok {
				compiledSchemas[genericName] = targetSchema
				mapped = append(mapped, fmt.Sprintf("%s->%s", genericName, targetDef))
				break // Use the first found target.
			}
		}
	}
	if len(mapped) > 0 {
		v.logger.Debug("Added generic schema mappings.", "mappings", mapped) // Added period.
	}
}

// GetLoadDuration returns the duration of the last schema load operation.
func (v *Validator) GetLoadDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastLoadDuration
}

// GetCompileDuration returns the duration of the last schema compile operation.
func (v *Validator) GetCompileDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastCompileDuration
}

// Shutdown performs any cleanup needed for the schema validator.
func (v *Validator) Shutdown() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.initialized {
		return nil
	}
	v.logger.Info("Shutting down schema validator.") // Added period.
	// Close idle HTTP client connections if applicable.
	if transport, ok := v.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	} else if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		// Check default transport too, just in case.
		dt.CloseIdleConnections()
	}
	v.schemas = nil   // Release compiled schemas map.
	v.schemaDoc = nil // Release parsed document map.
	v.initialized = false
	v.logger.Info("Schema validator shut down.") // Added period.
	return nil
}

// IsInitialized returns whether the validator has been initialized.
func (v *Validator) IsInitialized() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.initialized
}

// Validate validates the given JSON data against the schema for the specified message type.
func (v *Validator) Validate(_ context.Context, messageType string, data []byte) error {
	if !v.IsInitialized() {
		return NewValidationError(ErrSchemaNotFound, "Schema validator not initialized", nil)
	}
	var instance interface{}
	if err := json.Unmarshal(data, &instance); err != nil {
		return NewValidationError(
			ErrInvalidJSONFormat,
			"Invalid JSON format",
			errors.Wrap(err, "json.Unmarshal failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data)) // Assuming calculatePreview is in helpers.go.
	}
	schemaToUse, schemaUsedKey, ok := v.getSchemaForMessageType(messageType)
	if !ok {
		v.mu.RLock()
		availableKeys := getSchemaKeys(v.schemas) // Assuming getSchemaKeys is in helpers.go or here.
		v.mu.RUnlock()
		return NewValidationError(
			ErrSchemaNotFound,
			fmt.Sprintf("Schema definition not found for message type '%s' or standard fallbacks.", messageType),
			nil,
		).WithContext("messageType", messageType).
			WithContext("availableSchemas", availableKeys)
	}

	validationStart := time.Now()
	err := schemaToUse.Validate(instance)
	validationDuration := time.Since(validationStart)

	if err != nil {
		var valErr *jsonschema.ValidationError
		if errors.As(err, &valErr) {
			v.logger.Debug("Schema validation failed.",
				"duration", validationDuration,
				"messageType", messageType,
				"schemaUsed", schemaUsedKey,
				"error", valErr.Message) // Added period.
			// Pass necessary info to convertValidationError.
			return convertValidationError(valErr, messageType, data) // Assumes convertValidationError is in errors.go.
		}
		// Handle unexpected non-jsonschema errors during validation.
		v.logger.Error("Unexpected error during schema.Validate.",
			"duration", validationDuration,
			"messageType", messageType,
			"schemaUsed", schemaUsedKey,
			"error", err) // Added period.
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
		"schemaUsed", schemaUsedKey) // Added period.
	return nil
}

// getSchemaForMessageType finds the appropriate compiled schema based on the message type.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) getSchemaForMessageType(messageType string) (*jsonschema.Schema, string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// 1. Exact Match.
	if schema, ok := v.schemas[messageType]; ok {
		return schema, messageType, true
	}

	// 2. Fallback Logic.
	var fallbackKey string
	if strings.HasSuffix(messageType, "_notification") || strings.HasPrefix(messageType, "notifications/") {
		fallbackKey = "JSONRPCNotification" // Use standard JSON-RPC definition.
	} else if strings.Contains(messageType, "Response") || strings.Contains(messageType, "Result") || strings.HasSuffix(messageType, "_response") || strings.HasSuffix(messageType, "_result") {
		// Prioritize more specific JSON-RPC types if they exist.
		if strings.Contains(messageType, "Error") || strings.HasSuffix(messageType, "_error") {
			if _, errorExists := v.schemas["JSONRPCError"]; errorExists {
				fallbackKey = "JSONRPCError"
			} else if _, responseExists := v.schemas["JSONRPCResponse"]; responseExists {
				fallbackKey = "JSONRPCResponse" // Fallback for general response structure.
			} else {
				fallbackKey = "base" // Ultimate fallback.
			}
		} else {
			// Likely a success response.
			if _, responseExists := v.schemas["JSONRPCResponse"]; responseExists {
				fallbackKey = "JSONRPCResponse"
			} else {
				fallbackKey = "base"
			}
		}
	} else {
		// Assume it's a request if not notification or response.
		fallbackKey = "JSONRPCRequest"
	}

	// Check if the chosen fallback key exists.
	if schema, ok := v.schemas[fallbackKey]; ok {
		v.logger.Debug("Using fallback schema.", "originalType", messageType, "fallbackKey", fallbackKey) // Added period.
		return schema, fallbackKey, true
	}

	// 3. Ultimate Fallback to "base".
	if schema, ok := v.schemas["base"]; ok {
		v.logger.Warn("Using 'base' schema as final fallback.", "originalType", messageType, "initialFallbackAttempt", fallbackKey) // Added period.
		return schema, "base", true
	}

	// 4. Schema not found even with fallbacks.
	v.logger.Error("Could not find schema definition.", "requestedType", messageType, "fallbackAttempted", fallbackKey) // Added period.
	return nil, "", false
}

// HasSchema checks if a schema with the given name exists.
func (v *Validator) HasSchema(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.schemas == nil {
		return false
	}
	_, ok := v.schemas[name]
	return ok
}

// getSchemaKeys returns the keys of the schemas map for debugging purposes.
// This should likely be in helpers.go or remain private if only used internally.
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

// GetSchemaVersion returns the detected schema version, if available.
func (v *Validator) GetSchemaVersion() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.schemaVersion
}
