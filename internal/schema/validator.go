// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// file: internal/schema/validator.go
//
// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
//
// The validator implementation orchestrates the schema handling process:
// 1. Loading: schema content is retrieved either from a configured URI or from embedded content
// 2. Parsing: JSON schema is parsed into an in-memory structure
// 3. Compilation: Schema definitions are compiled for validation use
// 4. Validation: Incoming/outgoing messages are validated against compiled schemas
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
	"strings"
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
	// ETag/LastModified are no longer needed here as network fetch isn't default.
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
// It prioritizes the override URI, otherwise uses the embedded schema.
func (v *Validator) Initialize(ctx context.Context) error {
	initStart := time.Now()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.initialized {
		v.logger.Warn("Schema validator already initialized, skipping.")
		return nil
	}
	v.logger.Info("Initializing schema validator.")

	var schemaData []byte
	var sourceInfo string
	var loadErr error
	loadStart := time.Now()

	// --- Load Schema Data ---.
	overrideURI := v.schemaConfig.SchemaOverrideURI
	if overrideURI != "" {
		v.logger.Info("🔄 Attempting to load schema from override URI.", "uri", overrideURI)
		// Use the simplified loader function from loader.go.
		schemaData, loadErr = loadSchemaFromURI(ctx, overrideURI, v.logger, v.httpClient)
		sourceInfo = fmt.Sprintf("override: %s", overrideURI)
	} else {
		v.logger.Info("📦 Using embedded schema.")
		schemaData = embeddedSchemaContent
		sourceInfo = "embedded"
		loadErr = nil // No error loading embedded content.
	}
	v.lastLoadDuration = time.Since(loadStart)

	if loadErr != nil {
		v.logger.Error("Schema loading failed.", "duration", v.lastLoadDuration, "source", sourceInfo, "error", loadErr)
		// Wrap the error from loadSchemaFromURI if it occurred.
		return errors.Wrapf(loadErr, "failed to load schema from source '%s'", sourceInfo)
	}

	if len(schemaData) == 0 {
		// This should only happen if embedded content is empty or override load failed.
		err := errors.New("loaded schema data is empty")
		v.logger.Error("Schema loading failed.", "duration", v.lastLoadDuration, "source", sourceInfo, "error", err)
		return err
	}
	v.logger.Info("Schema loaded.", "duration", v.lastLoadDuration, "source", sourceInfo, "sizeBytes", len(schemaData))

	// --- Parse Schema JSON ---.
	if err := json.Unmarshal(schemaData, &v.schemaDoc); err != nil {
		v.logger.Error("Failed to parse schema JSON.", "error", err)
		return NewValidationError(ErrSchemaLoadFailed, "Failed to parse schema JSON", errors.Wrap(err, "json.Unmarshal failed"))
	}

	v.extractSchemaVersion(schemaData) // Extract version info - Call is now valid.

	// --- Add Schema Resource ---.
	addStart := time.Now()
	schemaReader := bytes.NewReader(schemaData)
	resourceID := "mcp://schema.json" // Base URI for internal references.
	if err := v.compiler.AddResource(resourceID, schemaReader); err != nil {
		v.logger.Error("Failed to add schema resource to compiler.", "duration", time.Since(addStart), "resourceID", resourceID, "error", err)
		return NewValidationError(ErrSchemaLoadFailed, "Failed to add schema resource", errors.Wrap(err, "compiler.AddResource failed")).WithContext("schemaSize", len(schemaData))
	}
	v.logger.Info("Schema resource added.", "duration", time.Since(addStart), "resourceID", resourceID)

	// --- Compile Schemas ---.
	compileStart := time.Now()
	compiledSchemas, compileErr := v.compileAllDefinitions(resourceID)
	v.lastCompileDuration = time.Since(compileStart)

	if compileErr != nil {
		v.logger.Error("Schema compilation finished with errors.", "duration", v.lastCompileDuration, "firstError", compileErr)
		return compileErr // Return the first critical error encountered.
	}

	v.schemas = compiledSchemas // Assign successfully compiled schemas.
	v.initialized = true
	initDuration := time.Since(initStart)

	v.logger.Info("Schema validator initialized successfully.",
		"totalDuration", initDuration,
		"loadDuration", v.lastLoadDuration,
		"compileDuration", v.lastCompileDuration,
		"schemaVersion", v.schemaVersion,
		"schemasCompiled", len(v.schemas))

	return nil
}

// compileAllDefinitions compiles the base schema and all found definitions.
func (v *Validator) compileAllDefinitions(baseResourceID string) (map[string]*jsonschema.Schema, error) {
	compiled := make(map[string]*jsonschema.Schema)
	var firstCompileError error

	// Compile base first.
	baseSchema, err := v.compiler.Compile(baseResourceID)
	if err != nil {
		v.logger.Error("CRITICAL: Failed to compile base schema resource.", "resourceID", baseResourceID, "error", err)
		return nil, NewValidationError(ErrSchemaCompileFailed, "Failed to compile base schema resource", errors.Wrap(err, "compiler.Compile failed for base schema")).WithContext("pointer", baseResourceID)
	}
	compiled["base"] = baseSchema
	v.logger.Debug("Compiled base schema definition.", "name", "base")

	// Compile definitions.
	if defs, ok := v.schemaDoc["definitions"].(map[string]interface{}); ok {
		for name := range defs {
			pointer := baseResourceID + "#/definitions/" + name
			schemaCompiled, errCompile := v.compiler.Compile(pointer)
			if errCompile != nil {
				v.logger.Warn("Failed to compile schema definition.", "name", name, "pointer", pointer, "error", errCompile)
				if firstCompileError == nil {
					firstCompileError = NewValidationError(
						ErrSchemaCompileFailed,
						fmt.Sprintf("Failed to compile schema definition '%s'", name),
						errors.Wrap(errCompile, "compiler.Compile failed"),
					).WithContext("pointer", pointer)
				}
			} else {
				compiled[name] = schemaCompiled
				v.logger.Debug("Compiled schema definition.", "name", name)
			}
		}
	} else {
		v.logger.Warn("No 'definitions' section found in the schema JSON.")
	}

	// Add generic mappings after compiling definitions.
	v.addGenericMappings(compiled)

	return compiled, firstCompileError
}

// addGenericMappings creates mappings from generic type names to specific schema definitions.
func (v *Validator) addGenericMappings(compiledSchemas map[string]*jsonschema.Schema) {
	mappings := map[string][]string{
		"success_response":        {"JSONRPCResponse", "Response"},
		"error_response":          {"JSONRPCError", "Error"},
		"ping_notification":       {"PingRequest", "PingNotification", "JSONRPCNotification"},
		"notification":            {"JSONRPCNotification", "Notification"},
		"request":                 {"JSONRPCRequest", "Request"},
		"CallToolResult":          {"CallToolResult", "ToolResult"},
		"initialize_response":     {"InitializeResult"},
		"tools/list_response":     {"ListToolsResult"},
		"resources/list_response": {"ListResourcesResult"},
		"prompts/list_response":   {"ListPromptsResult"},
	}
	mapped := make([]string, 0)
	for genericName, potentialTargets := range mappings {
		if _, exists := compiledSchemas[genericName]; exists {
			continue
		}
		for _, targetDef := range potentialTargets {
			if targetSchema, ok := compiledSchemas[targetDef]; ok {
				compiledSchemas[genericName] = targetSchema
				mapped = append(mapped, fmt.Sprintf("%s→%s", genericName, targetDef))
				break
			}
		}
	}
	if len(mapped) > 0 {
		v.logger.Debug("Added generic schema mappings.", "mappings", mapped)
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
	v.logger.Info("Shutting down schema validator.")
	if transport, ok := v.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	} else if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		dt.CloseIdleConnections()
	}
	v.schemas = nil
	v.schemaDoc = nil
	v.initialized = false
	v.logger.Info("Schema validator shut down.")
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
			WithContext("dataPreview", calculatePreview(data))
	}
	schemaToUse, schemaUsedKey, ok := v.getSchemaForMessageType(messageType)
	if !ok {
		v.mu.RLock()
		availableKeys := getSchemaKeys(v.schemas)
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
				"error", valErr.Message)
			return convertValidationError(valErr, messageType, data)
		}
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

// getSchemaForMessageType finds the appropriate compiled schema based on the message type.
func (v *Validator) getSchemaForMessageType(messageType string) (*jsonschema.Schema, string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if schema, ok := v.schemas[messageType]; ok {
		return schema, messageType, true
	}

	var fallbackKey string
	if strings.HasSuffix(messageType, "_notification") || strings.HasPrefix(messageType, "notifications/") {
		fallbackKey = "JSONRPCNotification"
	} else if strings.Contains(messageType, "Response") || strings.Contains(messageType, "Result") || strings.HasSuffix(messageType, "_response") || strings.HasSuffix(messageType, "_result") {
		if _, responseExists := v.schemas["JSONRPCResponse"]; responseExists {
			fallbackKey = "JSONRPCResponse"
		} else {
			fallbackKey = "base"
		}
		if strings.Contains(messageType, "Error") || strings.HasSuffix(messageType, "_error") {
			if _, errorExists := v.schemas["JSONRPCError"]; errorExists {
				fallbackKey = "JSONRPCError"
			}
		}
	} else {
		fallbackKey = "JSONRPCRequest"
	}

	if schema, ok := v.schemas[fallbackKey]; ok {
		v.logger.Debug("Using fallback schema.", "originalType", messageType, "fallbackKey", fallbackKey)
		return schema, fallbackKey, true
	}

	if schema, ok := v.schemas["base"]; ok {
		v.logger.Debug("Using base schema as final fallback.", "originalType", messageType)
		return schema, "base", true
	}

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
